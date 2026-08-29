// Package client provides a thin HTTP client for the OneBusAway REST API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"oba-mcp/cachedb"
)

const (
	staticTTL   = 60 * time.Minute
	realtimeTTL = 30 * time.Second

	// Circuit breaker: open after cbThreshold consecutive transport failures,
	// stay open for cbCooldown, then allow one probe through (half-open).
	cbThreshold = 3
	cbCooldown  = 30 * time.Second
)

// realtimePrefixes are paths whose responses change frequently.
// Everything else is treated as static and cached for staticTTL.
var realtimePrefixes = []string{
	"/api/where/arrivals-and-departures-for-stop/",
	"/api/where/arrival-and-departure-for-stop/",
	"/api/where/arrivals-and-departures-for-location.json",
	"/api/where/trip-details/",
	"/api/where/trip-for-vehicle/",
	"/api/where/vehicles-for-agency/",
	"/api/where/trips-for-location.json",
	"/api/where/current-time.json",
}

type cacheEntry struct {
	data      map[string]any
	expiresAt time.Time
}

// OBAClient makes requests to an OneBusAway-compatible REST API.
type OBAClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	cache      map[string]cacheEntry
	cacheMu    sync.Mutex
	logger     *log.Logger
	db         *cachedb.Queries // optional persistent cache; nil = memory-only

	cbFailures  int
	cbOpenUntil time.Time
	cbMu        sync.Mutex
}

// New creates an OBAClient targeting baseURL and authenticating with apiKey.
// db is optional: pass a *cachedb.Queries to enable cross-session SQLite caching,
// or nil for in-memory only.
func New(baseURL, apiKey string, logger *log.Logger, db *cachedb.Queries) *OBAClient {
	return &OBAClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		cache:  make(map[string]cacheEntry),
		logger: logger,
		db:     db,
	}
}

// opFromPath extracts a short readable name from an OBA API path.
// "/api/where/arrivals-and-departures-for-stop/id.json" → "arrivals-and-departures-for-stop"
// "/api/where/search/stop.json" → "search/stop"
func opFromPath(path string) string {
	s := strings.TrimPrefix(path, "/api/where/")
	s = strings.TrimSuffix(s, ".json")
	parts := strings.SplitN(s, "/", 3)
	if len(parts) >= 2 && parts[0] == "search" {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func ttlForPath(path string) time.Duration {
	for _, prefix := range realtimePrefixes {
		if path == prefix || strings.HasPrefix(path, prefix) {
			return realtimeTTL
		}
	}
	return staticTTL
}

// Get makes a GET request to path with the given query params (plus the API key).
// Responses are cached: static data for 60 minutes, real-time data for 30 seconds.
// Each call emits one JSON log line to the logger (if set): op, cache, ms, bytes, tokens.
func (c *OBAClient) Get(path string, params url.Values) (map[string]any, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("key", c.apiKey)

	cacheKey := path + "?" + params.Encode()
	ttl := ttlForPath(path)
	op := opFromPath(path)

	c.cacheMu.Lock()
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		c.cacheMu.Unlock()
		if c.logger != nil {
			c.logger.Printf(`{"event":"req","op":%q,"cache":"hit","ms":0,"bytes":0,"tokens":0}`, op)
		}
		return entry.data, nil
	}
	c.cacheMu.Unlock()

	// L2: persistent SQLite cache (cross-session, static data survives restart).
	if c.db != nil {
		if row, err := c.db.GetEntry(context.Background(), cacheKey); err == nil {
			if time.Now().Unix() < row.ExpiresAt {
				var result map[string]any
				if json.Unmarshal(row.Data, &result) == nil {
					expiresAt := time.Unix(row.ExpiresAt, 0)
					c.cacheMu.Lock()
					c.cache[cacheKey] = cacheEntry{data: result, expiresAt: expiresAt}
					c.cacheMu.Unlock()
					if c.logger != nil {
						c.logger.Printf(`{"event":"req","op":%q,"cache":"l2-hit","ms":0,"bytes":%d,"tokens":%d}`,
							op, len(row.Data), len(row.Data)/4)
					}
					return result, nil
				}
			}
		}
	}

	// Circuit breaker: fail fast when maglev is known to be down.
	c.cbMu.Lock()
	if !c.cbOpenUntil.IsZero() && time.Now().Before(c.cbOpenUntil) {
		retryIn := time.Until(c.cbOpenUntil).Round(time.Second)
		c.cbMu.Unlock()
		if c.logger != nil {
			c.logger.Printf(`{"event":"req","op":%q,"circuit":"open","retry_in_sec":%d}`, op, int(retryIn.Seconds()))
		}
		return nil, fmt.Errorf("maglev is unreachable (circuit open, retrying in %s)", retryIn)
	}
	c.cbMu.Unlock()

	start := time.Now()
	resp, err := c.httpClient.Get(fmt.Sprintf("%s%s?%s", c.baseURL, path, params.Encode()))
	if err != nil {
		ms := time.Since(start).Milliseconds()
		c.cbMu.Lock()
		c.cbFailures++
		if c.cbFailures >= cbThreshold {
			c.cbOpenUntil = time.Now().Add(cbCooldown)
			if c.logger != nil {
				c.logger.Printf(`{"event":"circuit","state":"open","failures":%d}`, c.cbFailures)
			}
		}
		c.cbMu.Unlock()
		if c.logger != nil {
			c.logger.Printf(`{"event":"req","op":%q,"cache":"miss","ms":%d,"error":%q}`,
				op, ms, err.Error())
		}
		return nil, fmt.Errorf("request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Successful response: reset circuit breaker.
	c.cbMu.Lock()
	if c.cbFailures > 0 {
		if c.logger != nil {
			c.logger.Printf(`{"event":"circuit","state":"closed","after_failures":%d}`, c.cbFailures)
		}
		c.cbFailures = 0
		c.cbOpenUntil = time.Time{}
	}
	c.cbMu.Unlock()

	if c.logger != nil {
		c.logger.Printf(`{"event":"req","op":%q,"cache":"miss","ms":%d,"bytes":%d,"tokens":%d}`,
			op, ms, len(body), len(body)/4)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	c.cacheMu.Lock()
	c.cache[cacheKey] = cacheEntry{data: result, expiresAt: expiresAt}
	c.cacheMu.Unlock()

	// Write through to SQLite (only static data — real-time TTL is too short to be useful).
	if c.db != nil && ttl >= staticTTL {
		if encoded, err := json.Marshal(result); err == nil {
			_ = c.db.SetEntry(context.Background(), cachedb.SetEntryParams{
				Key:       cacheKey,
				Data:      encoded,
				ExpiresAt: expiresAt.Unix(),
			})
		}
	}

	return result, nil
}

// Data validates the OBA response envelope { "code":200, "data":{...} }
// and returns the inner data map.
func Data(response map[string]any) (map[string]any, error) {
	code, _ := response["code"].(float64)
	if code != 200 {
		text, _ := response["text"].(string)
		return nil, fmt.Errorf("OBA error (code %d): %s", int(code), text)
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing data field in response")
	}
	return data, nil
}

// StrVal converts any value to a string, returning "" for nil.
func StrVal(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// FloatVal extracts a float64, returning 0 if not present or wrong type.
func FloatVal(v any) float64 {
	f, _ := v.(float64)
	return f
}

// AsSlice type-asserts v to []any, returning nil on failure.
func AsSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// FormatRelativeTime formats a Unix millisecond timestamp relative to now in loc.
// Pass nil to use UTC. Example: "3:42 PM (in 8 min)" or "3:42 PM (2 min ago)".
func FormatRelativeTime(ms float64, loc *time.Location) string {
	if ms == 0 {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	t := time.UnixMilli(int64(ms)).In(loc)
	diff := time.Until(t).Round(time.Minute)
	switch {
	case diff < 0:
		return fmt.Sprintf("%s (%d min ago)", t.Format("3:04 PM"), int(-diff.Minutes()))
	case diff < time.Minute:
		return fmt.Sprintf("%s (arriving now)", t.Format("3:04 PM"))
	default:
		return fmt.Sprintf("%s (in %d min)", t.Format("3:04 PM"), int(diff.Minutes()))
	}
}

// TimezoneFor returns the *time.Location for an agency from the OBA API.
// The agency response is static data (60-min TTL), so this is free after the first call.
// Falls back to UTC on any error.
func (c *OBAClient) TimezoneFor(agencyID string) *time.Location {
	if agencyID == "" {
		return time.UTC
	}
	resp, err := c.Get("/api/where/agency/"+agencyID+".json", nil)
	if err != nil {
		return time.UTC
	}
	data, err := Data(resp)
	if err != nil {
		return time.UTC
	}
	entry, _ := data["entry"].(map[string]any)
	tzName := StrVal(entry["timezone"])
	if tzName == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.UTC
	}
	return loc
}

// AgencyIDFromEntityID extracts the agency portion from a qualified OBA ID.
// "1_1013" → "1", "unitrans_22274" → "unitrans".
func AgencyIDFromEntityID(id string) string {
	if before, _, found := strings.Cut(id, "_"); found {
		return before
	}
	return id
}

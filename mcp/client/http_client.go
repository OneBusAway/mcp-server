// Package client provides reliable HTTP access to OneBusAway-compatible APIs.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"oba-mcp/cachedb"
	"oba-mcp/internal/requestmeta"
)

const (
	staticTTL = 60 * time.Minute
	// Real-time responses are intentionally uncached. Concurrent identical calls
	// are still coalesced through inflight, but every subsequent call reaches OBA.
	realtimeTTL = 0 * time.Second

	maxResponseBytes   = 2 << 20 // 2 MiB maximum accepted payload.
	maxMemoryCacheSize = 512
	maxConcurrentCalls = 10
	maxAttempts        = 3
	maxRetryAfter      = 5 * time.Second

	// Circuit breaker: open after cbThreshold consecutive upstream-health
	// failures, stay open for cbCooldown, then permit one half-open probe.
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

// ErrorCode identifies an upstream failure without exposing upstream details.
type ErrorCode string

const (
	ErrorCircuitOpen      ErrorCode = "UPSTREAM_CIRCUIT_OPEN"
	ErrorCancelled        ErrorCode = "UPSTREAM_CANCELLED"
	ErrorTimeout          ErrorCode = "UPSTREAM_TIMEOUT"
	ErrorRateLimited      ErrorCode = "UPSTREAM_RATE_LIMITED"
	ErrorUnavailable      ErrorCode = "UPSTREAM_UNAVAILABLE"
	ErrorBadResponse      ErrorCode = "UPSTREAM_BAD_RESPONSE"
	ErrorResponseTooLarge ErrorCode = "UPSTREAM_RESPONSE_TOO_LARGE"
)

// CacheState is the safe cache provenance exposed to tool results.
type CacheState string

const (
	CacheHit  CacheState = "hit"
	CacheMiss CacheState = "miss"
)

// UpstreamObservation is the bounded, privacy-safe data emitted for an OBA
// request attempt. Query parameters are available to logs, but credentials are excluded.
type UpstreamObservation struct {
	Operation  string
	Cache      string
	ErrorCode  ErrorCode
	StatusCode int
	Duration   time.Duration
	Bytes      int
}

// Observer receives operational events without coupling the OBA client to a
// particular metrics implementation.
type Observer interface {
	ObserveUpstream(UpstreamObservation)
	ObserveCircuit(state string)
	ObserveConcurrencyLimit(outcome string)
}

// UpstreamError classifies failures without including upstream internals.
type UpstreamError struct {
	Code          ErrorCode
	Retryable     bool
	RetryAfter    time.Duration
	StatusCode    int
	internalCause error
}

func (e *UpstreamError) Error() string {
	details := ""
	if e.Retryable {
		details = "retryable"
	}
	if e.RetryAfter > 0 {
		if details != "" {
			details += "; "
		}
		details += fmt.Sprintf("retry_after_ms=%d", e.RetryAfter.Milliseconds())
	}
	if details != "" {
		return string(e.Code) + " (" + details + ")"
	}
	return string(e.Code)
}

func (e *UpstreamError) Unwrap() error { return e.internalCause }

func upstreamError(code ErrorCode, retryable bool, cause error) *UpstreamError {
	return &UpstreamError{Code: code, Retryable: retryable, internalCause: cause}
}

type cacheEntry struct {
	data      json.RawMessage
	expiresAt time.Time
	lastUsed  time.Time
}

type inFlightRequest struct {
	done chan struct{}
	data json.RawMessage
	err  error
}

// OBAClient makes requests to an OneBusAway-compatible REST API.
type OBAClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *log.Logger
	observer   Observer
	db         *cachedb.Queries // optional persistent cache; nil = memory-only

	// clock controls cache-TTL comparisons. It defaults to time.Now and is
	// overridable in tests so TTL expiry can be exercised without real waits.
	// Elapsed measurements (fetch latency logging) intentionally keep the
	// real time.Now to preserve monotonic-clock behavior.
	clock func() time.Time

	// waiterEntered is called once by each coalesced waiter immediately before
	// it blocks in the select inside waitForInFlight. It is nil in production
	// and only used by tests to sequence "both waiters have actually joined
	// the leader" without a sleep-based race.
	waiterEntered func()

	cache    map[string]cacheEntry
	inflight map[string]*inFlightRequest
	cacheMu  sync.Mutex

	upstreamSem chan struct{}

	cbFailures      int
	cbOpenUntil     time.Time
	cbProbeInFlight bool
	cbMu            sync.Mutex
}

// SetObserver installs the process-level operational metrics sink.
func (c *OBAClient) SetObserver(observer Observer) {
	c.observer = observer
}

// New creates an OBAClient targeting baseURL and authenticating with apiKey.
// db is optional: pass a *cachedb.Queries to enable cross-session SQLite caching,
// or nil for in-memory only.
func New(baseURL, apiKey string, logger *log.Logger, db *cachedb.Queries) *OBAClient {
	return &OBAClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		logger:      logger,
		db:          db,
		clock:       time.Now,
		cache:       make(map[string]cacheEntry),
		inflight:    make(map[string]*inFlightRequest),
		upstreamSem: make(chan struct{}, maxConcurrentCalls),
	}
}

// opFromPath extracts a short readable name from an OBA API path.
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

func cacheKey(path string, params url.Values) string {
	return "oba:v1:" + path + "?" + params.Encode()
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	return json.RawMessage(append([]byte(nil), data...))
}

func cloneValues(values url.Values) url.Values {
	if values == nil {
		return url.Values{}
	}
	clone := make(url.Values, len(values))
	for key, values := range values {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func (c *OBAClient) memoryCacheGet(key string, now time.Time) (json.RawMessage, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := c.cache[key]
	if !ok || !now.Before(entry.expiresAt) {
		if ok {
			delete(c.cache, key)
		}
		return nil, false
	}
	entry.lastUsed = now
	c.cache[key] = entry
	return cloneRawMessage(entry.data), true
}

func (c *OBAClient) memoryCacheSet(key string, data json.RawMessage, expiresAt, now time.Time) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	for existingKey, entry := range c.cache {
		if !now.Before(entry.expiresAt) {
			delete(c.cache, existingKey)
		}
	}
	if _, exists := c.cache[key]; !exists && len(c.cache) >= maxMemoryCacheSize {
		var oldestKey string
		var oldest time.Time
		for existingKey, entry := range c.cache {
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey, oldest = existingKey, entry.lastUsed
			}
		}
		delete(c.cache, oldestKey)
	}
	c.cache[key] = cacheEntry{data: cloneRawMessage(data), expiresAt: expiresAt, lastUsed: now}
}

func (c *OBAClient) loadPersistentCache(ctx context.Context, key string, now time.Time) (json.RawMessage, bool) {
	if c.db == nil {
		return nil, false
	}
	row, err := c.db.GetEntry(ctx, key)
	if err != nil || now.Unix() >= row.ExpiresAt || !json.Valid(row.Data) {
		return nil, false
	}
	result := cloneRawMessage(row.Data)
	c.memoryCacheSet(key, result, time.Unix(row.ExpiresAt, 0), now)
	return result, true
}

func (c *OBAClient) beginInFlight(key string) (*inFlightRequest, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if call, ok := c.inflight[key]; ok {
		return call, false
	}
	call := &inFlightRequest{done: make(chan struct{})}
	c.inflight[key] = call
	return call, true
}

func (c *OBAClient) finishInFlight(key string, call *inFlightRequest, data json.RawMessage, err error) {
	c.cacheMu.Lock()
	call.data = cloneRawMessage(data)
	call.err = err
	delete(c.inflight, key)
	close(call.done)
	c.cacheMu.Unlock()
}

func (c *OBAClient) waitForInFlight(ctx context.Context, call *inFlightRequest) (json.RawMessage, error) {
	if c.waiterEntered != nil {
		c.waiterEntered()
	}
	select {
	case <-ctx.Done():
		return nil, upstreamError(ErrorCancelled, false, ctx.Err())
	case <-call.done:
		return cloneRawMessage(call.data), call.err
	}
}

// Get makes a context-aware GET request to path with the given query params.
// The cache key excludes the API key so credentials never enter memory or SQLite.
func (c *OBAClient) Get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	data, _, err := c.GetWithCacheState(ctx, path, params)
	return data, err
}

// GetWithCacheState makes a context-aware GET request and reports only whether
// the response came from a cache, without disclosing cache implementation details.
func (c *OBAClient) GetWithCacheState(ctx context.Context, path string, params url.Values) (json.RawMessage, CacheState, error) {
	if err := ctx.Err(); err != nil {
		return nil, CacheMiss, upstreamError(ErrorCancelled, false, err)
	}
	query := cloneValues(params)
	key := cacheKey(path, query)
	ttl := ttlForPath(path)
	op := opFromPath(path)
	now := c.clock()
	if ttl > 0 {
		if result, ok := c.memoryCacheGet(key, now); ok {
			c.logRequest(ctx, op, "hit", query, 0, len(result), nil)
			return result, CacheHit, nil
		}
		if result, ok := c.loadPersistentCache(ctx, key, now); ok {
			c.logRequest(ctx, op, "l2-hit", query, 0, len(result), nil)
			return result, CacheHit, nil
		}
	}

	call, leader := c.beginInFlight(key)
	if !leader {
		result, err := c.waitForInFlight(ctx, call)
		return result, CacheMiss, err
	}
	result, err := c.fetch(ctx, path, query, op)
	if ttl > 0 && err == nil && cacheableResponse(result) {
		writeNow := c.clock()
		expiresAt := writeNow.Add(ttl)
		c.memoryCacheSet(key, result, expiresAt, writeNow)
		if c.db != nil && ttl >= staticTTL {
			_ = c.db.PruneExpired(ctx, writeNow.Unix())
			_ = c.db.SetEntry(ctx, cachedb.SetEntryParams{Key: key, Data: result, ExpiresAt: expiresAt.Unix()})
		}
	}
	c.finishInFlight(key, call, result, err)
	return result, CacheMiss, err
}

func cacheableResponse(data json.RawMessage) bool {
	var envelope struct {
		Code *int `json:"code"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return false
	}
	return envelope.Code == nil || *envelope.Code == http.StatusOK
}

func (c *OBAClient) fetch(ctx context.Context, path string, query url.Values, op string) (json.RawMessage, error) {
	if err := c.allowCircuitRequest(); err != nil {
		return nil, err
	}
	select {
	case c.upstreamSem <- struct{}{}:
		defer func() { <-c.upstreamSem }()
	case <-ctx.Done():
		c.finishCircuitProbe(false)
		if c.observer != nil {
			c.observer.ObserveConcurrencyLimit("wait_cancelled")
		}
		return nil, upstreamError(ErrorCancelled, false, ctx.Err())
	}

	query.Set("key", c.apiKey)
	requestURL, err := c.requestURL(path, query)
	if err != nil {
		c.finishCircuitProbe(false)
		return nil, upstreamError(ErrorBadResponse, false, err)
	}
	var lastErr *UpstreamError
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, retryDelay(attempt, lastErr.RetryAfter)); err != nil {
				c.finishCircuitProbe(false)
				return nil, upstreamError(ErrorCancelled, false, err)
			}
		}
		start := time.Now()
		result, failure := c.doRequest(ctx, requestURL)
		c.logRequest(ctx, op, "miss", query, time.Since(start).Milliseconds(), len(result), failure)
		if failure == nil {
			c.recordCircuitSuccess()
			return result, nil
		}
		lastErr = failure
		if !failure.Retryable || attempt == maxAttempts-1 {
			break
		}
	}
	c.recordCircuitFailure(lastErr)
	return nil, lastErr
}

func (c *OBAClient) requestURL(path string, query url.Values) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid configured OBA base URL")
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *OBAClient) doRequest(ctx context.Context, requestURL string) (json.RawMessage, *UpstreamError) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, upstreamError(ErrorBadResponse, false, err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, upstreamError(ErrorCancelled, false, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, upstreamError(ErrorTimeout, true, err)
		}
		var networkErr net.Error
		if errors.As(err, &networkErr) {
			return nil, upstreamError(ErrorUnavailable, true, err)
		}
		return nil, upstreamError(ErrorUnavailable, true, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, statusError(response.StatusCode, response.Header.Get("Retry-After"))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		// Mid-body errors caused by ctx cancellation or Client.Timeout must
		// be reported as cancellation/timeout so callers retry (timeout) or
		// bail (cancel) appropriately. Otherwise a slow-body upstream would
		// masquerade as a hard UPSTREAM_BAD_RESPONSE and never retry.
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, upstreamError(ErrorCancelled, false, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, upstreamError(ErrorTimeout, true, err)
		}
		return nil, upstreamError(ErrorBadResponse, false, err)
	}
	if len(body) > maxResponseBytes {
		return nil, upstreamError(ErrorResponseTooLarge, false, nil)
	}
	if !json.Valid(body) {
		return nil, upstreamError(ErrorBadResponse, false, nil)
	}
	return cloneRawMessage(body), nil
}

func statusError(status int, retryAfterHeader string) *UpstreamError {
	if status == http.StatusTooManyRequests {
		err := upstreamError(ErrorRateLimited, true, nil)
		err.StatusCode = status
		err.RetryAfter = parseRetryAfter(retryAfterHeader)
		return err
	}
	if status >= http.StatusInternalServerError {
		err := upstreamError(ErrorUnavailable, true, nil)
		err.StatusCode = status
		err.RetryAfter = parseRetryAfter(retryAfterHeader)
		return err
	}
	err := upstreamError(ErrorBadResponse, false, nil)
	err.StatusCode = status
	return err
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, maxRetryAfter)
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		return min(max(time.Until(retryAt), 0), maxRetryAfter)
	}
	return 0
}

func retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	backoff := 100 * time.Millisecond * time.Duration(1<<(attempt-1))
	jitter := time.Duration(rand.Int64N(int64(backoff/2) + 1))
	return min(backoff/2+jitter, maxRetryAfter)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *OBAClient) allowCircuitRequest() error {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	if c.cbOpenUntil.IsZero() {
		return nil
	}
	if time.Now().Before(c.cbOpenUntil) {
		return &UpstreamError{Code: ErrorCircuitOpen, Retryable: true, RetryAfter: time.Until(c.cbOpenUntil).Round(time.Second)}
	}
	if c.cbProbeInFlight {
		return &UpstreamError{Code: ErrorCircuitOpen, Retryable: true, RetryAfter: time.Second}
	}
	c.cbProbeInFlight = true
	return nil
}

func (c *OBAClient) finishCircuitProbe(success bool) {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	if c.cbProbeInFlight {
		c.cbProbeInFlight = false
		if !success {
			c.cbOpenUntil = time.Now().Add(cbCooldown)
		}
	}
}

func (c *OBAClient) recordCircuitSuccess() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	wasOpen := !c.cbOpenUntil.IsZero()
	c.cbFailures = 0
	c.cbOpenUntil = time.Time{}
	c.cbProbeInFlight = false
	if wasOpen && c.logger != nil {
		c.logger.Printf(`{"event":"circuit","state":"closed"}`)
	}
	if wasOpen && c.observer != nil {
		c.observer.ObserveCircuit("closed")
	}
}

func (c *OBAClient) recordCircuitFailure(err *UpstreamError) {
	if err == nil || !err.Retryable || err.Code == ErrorCancelled {
		c.finishCircuitProbe(false)
		return
	}
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.cbProbeInFlight = false
	c.cbFailures++
	if c.cbFailures >= cbThreshold {
		c.cbOpenUntil = time.Now().Add(cbCooldown)
		if c.logger != nil {
			c.logger.Printf(`{"event":"circuit","state":"open","failures":%d}`, c.cbFailures)
		}
		if c.observer != nil {
			c.observer.ObserveCircuit("open")
		}
	}
}

func (c *OBAClient) logRequest(ctx context.Context, op, cache string, params url.Values, ms int64, bytes int, err *UpstreamError) {
	observation := UpstreamObservation{
		Operation: op,
		Cache:     cache,
		Duration:  time.Duration(ms) * time.Millisecond,
		Bytes:     bytes,
	}
	if err != nil {
		observation.ErrorCode = err.Code
		observation.StatusCode = err.StatusCode
	} else if cache == "miss" {
		observation.StatusCode = http.StatusOK
	}
	if c.observer != nil {
		c.observer.ObserveUpstream(observation)
	}
	if c.logger == nil {
		return
	}
	encodedParams, marshalErr := json.Marshal(sanitizedParams(params))
	if marshalErr != nil {
		encodedParams = []byte(`{}`)
	}
	if err != nil {
		c.logger.Printf(`{"event":"upstream","request_id":%q,"tool":%q,"op":%q,"cache":%q,"params":%s,"ms":%d,"status":%d,"error_code":%q}`, requestmeta.RequestID(ctx), requestmeta.ToolName(ctx), op, cache, encodedParams, ms, err.StatusCode, err.Code)
		return
	}
	c.logger.Printf(`{"event":"upstream","request_id":%q,"tool":%q,"op":%q,"cache":%q,"params":%s,"ms":%d,"status":%d,"bytes":%d}`, requestmeta.RequestID(ctx), requestmeta.ToolName(ctx), op, cache, encodedParams, ms, observation.StatusCode, bytes)
}

func sanitizedParams(params url.Values) url.Values {
	safe := make(url.Values, len(params))
	for key, values := range params {
		if isCredentialParam(key) {
			continue
		}
		safe[key] = append([]string(nil), values...)
	}
	return safe
}

func isCredentialParam(key string) bool {
	switch strings.ToLower(key) {
	case "key", "api_key", "apikey", "access_token", "token", "authorization", "password", "secret":
		return true
	default:
		return false
	}
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
func (c *OBAClient) TimezoneFor(ctx context.Context, agencyID string) *time.Location {
	if agencyID == "" {
		return time.UTC
	}
	resp, err := c.GetAgency(ctx, agencyID)
	if err != nil {
		return time.UTC
	}
	tzName := resp.Data.Entry.Timezone
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

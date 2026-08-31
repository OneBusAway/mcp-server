// Tests for the shared upstream HTTP client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oba-mcp/cachedb"
)

func clientWithTransport(transport http.RoundTripper) *OBAClient {
	c := New("https://oba.test", "secret-api-key", nil, nil)
	c.httpClient = &http.Client{Transport: transport}
	return c
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestGetPropagatesCallerContext(t *testing.T) {
	markerKey := struct{}{}
	marker := "request-context"
	c := clientWithTransport(roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Context().Value(markerKey); got != marker {
			t.Fatalf("request context marker = %v, want %q", got, marker)
		}
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))

	ctx := context.WithValue(context.Background(), markerKey, marker)
	if _, err := c.Get(ctx, "/api/where/current-time.json", nil); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
}

func TestGetRejectsCancelledContextBeforeUpstreamCall(t *testing.T) {
	called := false
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Get(ctx, "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorCancelled)
	if called {
		t.Fatal("cancelled request reached upstream")
	}
}

func TestGetRejectsOversizedResponses(t *testing.T) {
	body := `"` + strings.Repeat("x", maxResponseBytes) + `"`
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}))

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorResponseTooLarge)
}

func TestGetClassifiesNonSuccessStatusWithoutParsing(t *testing.T) {
	var calls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResponse(http.StatusBadRequest, "not JSON"), nil
	}))

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorBadResponse)
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 for non-retryable status", got)
	}
}

func TestGetClassifiesUpstreamTimeout(t *testing.T) {
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorTimeout)
}

// TestGetClassifiesRealHTTPClientTimeout exercises Client.Timeout firing
// against a real httptest server: headers never arrive because the upstream
// blocks past the client's timeout budget. This covers the production path
// (dial + read) that the RoundTripper-based test above cannot reach.
func TestGetClassifiesRealHTTPClientTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	c := New(upstream.URL, "secret-api-key", nil, nil)
	c.httpClient = &http.Client{Timeout: 100 * time.Millisecond}

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorTimeout)
}

// TestGetClassifiesTimeoutMidBody covers the case where the server flushes
// headers and a partial body, then blocks past Client.Timeout. The body-read
// error path must classify this as UPSTREAM_TIMEOUT (retryable), not
// UPSTREAM_BAD_RESPONSE, so callers retry a slow upstream rather than
// treating it as a permanent failure.
func TestGetClassifiesTimeoutMidBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(`{"code":200,`))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	c := New(upstream.URL, "secret-api-key", nil, nil)
	c.httpClient = &http.Client{Timeout: 100 * time.Millisecond}

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorTimeout)
}

// TestGetClassifiesMidBodyContextCancel pins the mirror case: mid-body
// cancellation via context.Cancel must classify as UPSTREAM_CANCELLED
// (non-retryable) rather than UPSTREAM_BAD_RESPONSE.
func TestGetClassifiesMidBodyContextCancel(t *testing.T) {
	bodyStarted := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(`{"code":200,`))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		bodyStarted <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	c := New(upstream.URL, "secret-api-key", nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := c.Get(ctx, "/api/where/current-time.json", nil)
		errCh <- err
	}()

	select {
	case <-bodyStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not begin reading the partial response body within 3s")
	}
	cancel()

	select {
	case err := <-errCh:
		assertUpstreamCode(t, err, ErrorCancelled)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled mid-body request did not return within 3s")
	}
}

func TestGetDoesNotCacheOBAErrorEnvelopes(t *testing.T) {
	var calls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResponse(http.StatusOK, `{"code":404,"text":"not found"}`), nil
	}))

	for range 2 {
		if _, err := c.Get(context.Background(), "/api/where/stop/missing.json", nil); err != nil {
			t.Fatalf("Get returned transport error: %v", err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 because OBA errors must not be cached", got)
	}
}

func TestGetWithCacheStateReportsHitAndMiss(t *testing.T) {
	var calls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))

	for _, wantState := range []CacheState{CacheMiss, CacheHit} {
		_, state, err := c.GetWithCacheState(context.Background(), "/api/where/current-time.json", nil)
		if err != nil {
			t.Fatalf("GetWithCacheState returned error: %v", err)
		}
		if state != wantState {
			t.Fatalf("cache state = %q, want %q", state, wantState)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestGetRetriesTransientFailures(t *testing.T) {
	var calls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) < maxAttempts {
			return jsonResponse(http.StatusServiceUnavailable, ""), nil
		}
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))

	if _, err := c.Get(context.Background(), "/api/where/current-time.json", nil); err != nil {
		t.Fatalf("Get returned error after retry: %v", err)
	}
	if got := calls.Load(); got != maxAttempts {
		t.Fatalf("upstream calls = %d, want %d", got, maxAttempts)
	}
}

func TestRetryDelayHonorsRetryAfter(t *testing.T) {
	want := 3 * time.Second
	if got := retryDelay(1, want); got != want {
		t.Fatalf("retry delay = %s, want Retry-After value %s", got, want)
	}
}

func TestGetCoalescesConcurrentCacheMisses(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))

	const callers = 4
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
			errs <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leader request did not reach upstream")
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("coalesced request returned error: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

// TestCoalescedWaiterCancellationDoesNotAffectOthers verifies that when a
// coalesced waiter cancels its context, only that waiter fails with
// UPSTREAM_CANCELLED; the leader completes normally, other waiters receive
// the leader's success payload, and upstream is called exactly once. This
// guards Phase 3's coalescing + cancellation contract at its real boundary
// rather than in a helper in isolation.
//
// Synchronization uses the waiterEntered hook so we can prove both waiters
// have blocked inside waitForInFlight before we cancel — a sleep-based
// join would let the cancelled goroutine race past the ctx.Err() pre-check
// in GetWithCacheState and short-circuit before ever reaching the wait
// path, which would silently defeat the property under test.
func TestCoalescedWaiterCancellationDoesNotAffectOthers(t *testing.T) {
	var upstreamCalls atomic.Int32
	leaderInFlight := make(chan struct{})
	releaseLeader := make(chan struct{})
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		close(leaderInFlight)
		<-releaseLeader
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))

	const wantWaiters = 2
	waitersEntered := make(chan struct{}, wantWaiters)
	c.waiterEntered = func() { waitersEntered <- struct{}{} }

	const path = "/api/where/current-time.json"

	leaderErr := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), path, nil)
		leaderErr <- err
	}()

	select {
	case <-leaderInFlight:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not reach upstream within 2s")
	}

	otherWaiterErr := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), path, nil)
		otherWaiterErr <- err
	}()

	cancelCtx, cancelWaiter := context.WithCancel(context.Background())
	cancelledWaiterErr := make(chan error, 1)
	go func() {
		_, err := c.Get(cancelCtx, path, nil)
		cancelledWaiterErr <- err
	}()

	// Wait deterministically for both waiters to have entered waitForInFlight.
	// Only then can we guarantee that the cancellation exercises the wait's
	// ctx.Done branch, not the caller-side pre-check.
	for received := 0; received < wantWaiters; received++ {
		select {
		case <-waitersEntered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d/%d waiters entered waitForInFlight within 2s", received, wantWaiters)
		}
	}

	cancelWaiter()

	select {
	case err := <-cancelledWaiterErr:
		assertUpstreamCode(t, err, ErrorCancelled)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return within 2s")
	}

	// Leader and the other waiter must still be blocked on the upstream
	// response — cancellation of one waiter must not release them.
	select {
	case err := <-leaderErr:
		t.Fatalf("leader returned before release: %v", err)
	case err := <-otherWaiterErr:
		t.Fatalf("uncancelled waiter returned before release: %v", err)
	default:
	}

	close(releaseLeader)

	if err := <-leaderErr; err != nil {
		t.Fatalf("leader returned error: %v", err)
	}
	if err := <-otherWaiterErr; err != nil {
		t.Fatalf("uncancelled waiter returned error: %v", err)
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (coalescing broken)", got)
	}
}

func TestTTLForPathSeparatesRealtimeAndStaticData(t *testing.T) {
	if got := ttlForPath("/api/where/arrivals-and-departures-for-stop/test_1013.json"); got != realtimeTTL {
		t.Fatalf("arrivals TTL = %s, want realtime TTL %s", got, realtimeTTL)
	}
	if got := ttlForPath("/api/where/stop/test_1013.json"); got != staticTTL {
		t.Fatalf("stop TTL = %s, want static TTL %s", got, staticTTL)
	}
}

// TestMemoryCacheEntryExpiresAfterTTL uses the injectable clock to advance
// time past the entry's TTL without a real sleep, then verifies the next Get
// is a miss (not a stale hit) and re-fetches upstream. This closes the gap
// between TTL classification (tested above) and TTL enforcement.
func TestMemoryCacheEntryExpiresAfterTTL(t *testing.T) {
	var upstreamCalls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))
	baseTime := time.Now()
	c.clock = func() time.Time { return baseTime }

	// Populate cache.
	if _, _, err := c.GetWithCacheState(context.Background(), "/api/where/stop/test_1013.json", nil); err != nil {
		t.Fatalf("initial Get returned error: %v", err)
	}
	// Within TTL: hit, no upstream call.
	if _, state, err := c.GetWithCacheState(context.Background(), "/api/where/stop/test_1013.json", nil); err != nil {
		t.Fatalf("within-TTL Get returned error: %v", err)
	} else if state != CacheHit {
		t.Fatalf("within-TTL cache state = %q, want %q", state, CacheHit)
	}

	// Advance the clock past the static TTL. The stored entry's expiresAt was
	// baseTime + staticTTL, so any read after that must miss and refetch.
	c.clock = func() time.Time { return baseTime.Add(staticTTL + time.Second) }
	_, state, err := c.GetWithCacheState(context.Background(), "/api/where/stop/test_1013.json", nil)
	if err != nil {
		t.Fatalf("post-TTL Get returned error: %v", err)
	}
	if state != CacheMiss {
		t.Fatalf("post-TTL cache state = %q, want %q (stale entry served)", state, CacheMiss)
	}
	if got := upstreamCalls.Load(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (populate + refetch after expiry)", got)
	}
}

// TestGetWithCacheStateReportsL2Hit hits the SQLite persistent cache path
// bypassing the in-memory cache. The client must report the response as a
// cache hit (not miss) so callers can attribute it correctly, and no upstream
// call should be made.
func TestGetWithCacheStateReportsL2Hit(t *testing.T) {
	q, sqlDB, err := cachedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory cachedb: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("upstream must not be called when L2 cache serves the response")
		return nil, nil
	}))
	c.db = q

	const path = "/api/where/stop/test_1013.json"
	key := cacheKey(path, cloneValues(nil))
	body := json.RawMessage(`{"code":200,"data":{"entry":{"id":"test_1013"}}}`)
	if err := q.SetEntry(context.Background(), cachedb.SetEntryParams{
		Key:       key,
		Data:      body,
		ExpiresAt: time.Now().Add(staticTTL).Unix(),
	}); err != nil {
		t.Fatalf("preload L2 entry: %v", err)
	}

	got, state, err := c.GetWithCacheState(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("L2 Get returned error: %v", err)
	}
	if state != CacheHit {
		t.Fatalf("L2 cache state = %q, want %q", state, CacheHit)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("L2 payload = %s, want %s", got, body)
	}
}

// TestPersistentCacheFallsBackWhenEntryCorrupt covers the L2 error path:
// a row that decodes as invalid JSON must be treated as a miss (not returned
// to the caller), and the client must fetch from upstream instead. A silent
// hang or a garbage response back to the model is the failure mode this
// guards against.
func TestPersistentCacheFallsBackWhenEntryCorrupt(t *testing.T) {
	q, sqlDB, err := cachedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory cachedb: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var upstreamCalls atomic.Int32
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		return jsonResponse(http.StatusOK, `{"code":200,"data":{"entry":{"id":"test_1013"}}}`), nil
	}))
	c.db = q

	const path = "/api/where/stop/test_1013.json"
	key := cacheKey(path, cloneValues(nil))
	if err := q.SetEntry(context.Background(), cachedb.SetEntryParams{
		Key:       key,
		Data:      []byte("not json"),
		ExpiresAt: time.Now().Add(staticTTL).Unix(),
	}); err != nil {
		t.Fatalf("preload corrupt L2 entry: %v", err)
	}

	_, state, err := c.GetWithCacheState(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Get with corrupt L2 returned error: %v", err)
	}
	if state != CacheMiss {
		t.Fatalf("cache state = %q, want %q (corrupt entry must not be served)", state, CacheMiss)
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (corrupt L2 should force refetch)", got)
	}
}

func TestMemoryCacheIsBoundedAndCredentialsAreExcluded(t *testing.T) {
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))
	params := url.Values{"input": {"Memorial"}}
	if _, err := c.Get(context.Background(), "/api/where/search/stop.json", params); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if params.Has("key") {
		t.Fatal("Get mutated caller query parameters with API key")
	}
	for key := range c.cache {
		if strings.Contains(key, "secret-api-key") {
			t.Fatal("cache key contains API key")
		}
	}
	for i := 0; i < maxMemoryCacheSize+1; i++ {
		c.memoryCacheSet(string(rune(i)), bytes.Repeat([]byte("x"), 1), time.Now().Add(time.Minute), time.Now())
	}
	if got := len(c.cache); got != maxMemoryCacheSize {
		t.Fatalf("memory cache size = %d, want %d", got, maxMemoryCacheSize)
	}
}

func TestCircuitOpensAfterConsecutiveUpstreamFailures(t *testing.T) {
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("open circuit should fail before upstream call")
		return nil, nil
	}))
	failure := &UpstreamError{Code: ErrorUnavailable, Retryable: true}
	for range cbThreshold {
		c.recordCircuitFailure(failure)
	}

	_, err := c.Get(context.Background(), "/api/where/current-time.json", nil)
	assertUpstreamCode(t, err, ErrorCircuitOpen)
}

func TestSuccessfulHalfOpenProbeClosesCircuit(t *testing.T) {
	c := clientWithTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"code":200}`), nil
	}))
	c.cbFailures = cbThreshold
	c.cbOpenUntil = time.Now().Add(-time.Second)

	if _, err := c.Get(context.Background(), "/api/where/current-time.json", nil); err != nil {
		t.Fatalf("half-open probe returned error: %v", err)
	}
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	if !c.cbOpenUntil.IsZero() || c.cbFailures != 0 || c.cbProbeInFlight {
		t.Fatalf("circuit was not reset after successful probe: open_until=%s failures=%d probe=%t", c.cbOpenUntil, c.cbFailures, c.cbProbeInFlight)
	}
}

func assertUpstreamCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("error %v is not an UpstreamError", err)
	}
	if upstream.Code != want {
		t.Fatalf("error code = %q, want %q", upstream.Code, want)
	}
}

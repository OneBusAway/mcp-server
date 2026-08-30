// Tests for the shared upstream HTTP client.
package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

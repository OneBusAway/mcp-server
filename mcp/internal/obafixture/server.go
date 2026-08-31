// Package obafixture provides a deterministic HTTP server for OBA client and
// MCP transport tests. It deliberately has no live-feed dependency.
package obafixture

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
)

// Response is a fixed response for one OBA API path.
type Response struct {
	Status int
	Header http.Header
	Body   string
}

// Request records the observable parts of a request received by the fixture.
type Request struct {
	Path  string
	Query url.Values
}

// Server serves fixed OBA responses and records requests for assertions.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	requests []Request
}

// New starts a deterministic OBA fixture. Paths must be absolute, such as
// /api/where/stop/test_1013.json. Unregistered paths return an OBA-style 404.
func New(responses map[string]Response) *Server {
	fixture := &Server{}
	fixture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		fixture.requests = append(fixture.requests, Request{
			Path:  r.URL.Path,
			Query: cloneValues(r.URL.Query()),
		})
		fixture.mu.Unlock()

		response, ok := responses[r.URL.Path]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"text":"fixture endpoint not found"}`))
			return
		}
		for key, values := range response.Header {
			w.Header()[key] = append([]string(nil), values...)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		status := response.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response.Body))
	}))
	return fixture
}

// Requests returns a snapshot of received fixture requests.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := make([]Request, len(s.requests))
	for index, request := range s.requests {
		requests[index] = Request{Path: request.Path, Query: cloneValues(request.Query)}
	}
	return requests
}

func cloneValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, entries := range values {
		clone[key] = append([]string(nil), entries...)
	}
	return clone
}

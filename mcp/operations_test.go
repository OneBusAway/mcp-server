package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndReadinessHandlers(t *testing.T) {
	state := &operationalState{}

	health := httptest.NewRecorder()
	state.healthHandler(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assertOperationalStatus(t, health, http.StatusOK, "ok")

	notReady := httptest.NewRecorder()
	state.readinessHandler(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assertOperationalStatus(t, notReady, http.StatusServiceUnavailable, "not_ready")

	state.setReady(true)
	ready := httptest.NewRecorder()
	state.readinessHandler(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assertOperationalStatus(t, ready, http.StatusOK, "ready")
}

func TestHealthRejectsUnsupportedMethods(t *testing.T) {
	response := httptest.NewRecorder()
	(&operationalState{}).healthHandler(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q, want GET, HEAD", got)
	}
}

func TestOperationalEndpointsAuthenticationBoundary(t *testing.T) {
	state := &operationalState{}
	state.setReady(true)
	mux := newHTTPMux(http.NotFoundHandler(), state, newOperationalMetrics(), "metrics-token")

	health := httptest.NewRecorder()
	mux.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("unauthenticated health status = %d, want %d", health.Code, http.StatusOK)
	}

	unauthorizedMetrics := httptest.NewRecorder()
	mux.ServeHTTP(unauthorizedMetrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorizedMetrics.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated metrics status = %d, want %d", unauthorizedMetrics.Code, http.StatusUnauthorized)
	}

	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRequest.Header.Set("Authorization", "Bearer metrics-token")
	authorizedMetrics := httptest.NewRecorder()
	mux.ServeHTTP(authorizedMetrics, metricsRequest)
	if authorizedMetrics.Code != http.StatusOK {
		t.Fatalf("authenticated metrics status = %d, want %d", authorizedMetrics.Code, http.StatusOK)
	}
}

func TestAnnounceReadyWritesStructuredLogAndStderrStatus(t *testing.T) {
	var logOutput bytes.Buffer
	var statusOutput bytes.Buffer
	announceReady(log.New(&logOutput, "", 0), &statusOutput, "streamable-http", "http://127.0.0.1:8080/mcp")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logOutput.Bytes()), &entry); err != nil {
		t.Fatalf("decode ready log: %v", err)
	}
	if entry["event"] != "ready" || entry["transport"] != "streamable-http" {
		t.Fatalf("ready log = %#v", entry)
	}
	if !strings.Contains(statusOutput.String(), "http://127.0.0.1:8080/mcp") {
		t.Fatalf("status output = %q", statusOutput.String())
	}
}

func assertOperationalStatus(t *testing.T, response *httptest.ResponseRecorder, statusCode int, status string) {
	t.Helper()
	if response.Code != statusCode {
		t.Fatalf("status code = %d, want %d", response.Code, statusCode)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != status {
		t.Fatalf("status body = %q, want %q", body.Status, status)
	}
}

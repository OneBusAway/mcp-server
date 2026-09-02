package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync/atomic"
)

type operationalState struct {
	ready atomic.Bool
}

func (s *operationalState) setReady(ready bool) {
	s.ready.Store(ready)
}

func (s *operationalState) healthHandler(w http.ResponseWriter, r *http.Request) {
	if !healthMethodAllowed(w, r) {
		return
	}
	writeOperationalStatus(w, http.StatusOK, "ok")
}

func (s *operationalState) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if !healthMethodAllowed(w, r) {
		return
	}
	if !s.ready.Load() {
		writeOperationalStatus(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	writeOperationalStatus(w, http.StatusOK, "ready")
}

func healthMethodAllowed(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func writeOperationalStatus(w http.ResponseWriter, statusCode int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{Status: status})
}

func announceReady(appLogger *log.Logger, statusWriter io.Writer, transport, endpoint string) {
	if endpoint == "" {
		appLogger.Printf(`{"event":"ready","transport":%q}`, transport)
		_, _ = io.WriteString(statusWriter, "oba-mcp ready: "+transport+" transport; waiting for MCP client messages\n")
		return
	}
	appLogger.Printf(`{"event":"ready","transport":%q,"endpoint":%q}`, transport, endpoint)
	_, _ = io.WriteString(statusWriter, "oba-mcp ready: listening on "+endpoint+"\n")
}

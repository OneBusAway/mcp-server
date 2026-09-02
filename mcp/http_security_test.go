package main

import (
	"net/http"
	"net/http/httptest"
	"oba-mcp/internal/requestmeta"
	"testing"
)

func TestProtectedHTTPHandler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := protectedHTTPHandler(next, []string{"https://app.example"}, "gateway-token")
	tests := []struct {
		name, method, origin, authorization string
		want                                int
	}{
		{name: "rejects unknown origin", method: http.MethodPost, origin: "https://evil.example", authorization: "Bearer gateway-token", want: http.StatusForbidden},
		{name: "rejects no token", method: http.MethodPost, origin: "https://app.example", want: http.StatusUnauthorized},
		{name: "rejects invalid token", method: http.MethodPost, authorization: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "allows authorized originless client", method: http.MethodPost, authorization: "Bearer gateway-token", want: http.StatusNoContent},
		{name: "allows authorized origin", method: http.MethodPost, origin: "https://app.example", authorization: "Bearer gateway-token", want: http.StatusNoContent},
		{name: "allows approved preflight", method: http.MethodOptions, origin: "https://app.example", want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost/mcp", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Authorization", tt.authorization)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tt.want {
				t.Fatalf("status = %d, want %d", res.Code, tt.want)
			}
		})
	}
}

func TestProtectedHTTPHandlerPropagatesSafeRequestMetadata(t *testing.T) {
	var requestID string
	var callerHash string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID = requestmeta.RequestID(r.Context())
		callerHash = requestmeta.CallerHash(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	handler := protectedHTTPHandler(next, nil, "gateway-token")
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil)
	request.Header.Set("Authorization", "Bearer gateway-token")
	request.Header.Set("X-Request-ID", "gateway.request-123")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if requestID != "gateway.request-123" {
		t.Fatalf("request ID = %q, want gateway.request-123", requestID)
	}
	if response.Header().Get("X-Request-ID") != requestID {
		t.Fatalf("response request ID = %q, want %q", response.Header().Get("X-Request-ID"), requestID)
	}
	if callerHash == "" || callerHash == "gateway-token" {
		t.Fatalf("caller hash = %q", callerHash)
	}
}

func TestProtectedHTTPHandlerReplacesUnsafeRequestID(t *testing.T) {
	var requestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID = requestmeta.RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	handler := protectedHTTPHandler(next, nil, "gateway-token")
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil)
	request.Header.Set("Authorization", "Bearer gateway-token")
	request.Header.Set("X-Request-ID", "unsafe request id")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if requestID == "" || requestID == "unsafe request id" {
		t.Fatalf("unsafe request ID was not replaced: %q", requestID)
	}
}

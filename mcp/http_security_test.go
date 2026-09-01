package main

import (
	"net/http"
	"net/http/httptest"
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

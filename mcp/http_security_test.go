package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name, raw string
		want      []string
		ok        bool
	}{
		{name: "empty disables browser access", raw: "", ok: true},
		{name: "deduplicates valid origins", raw: "https://app.example, http://localhost:3000, https://app.example", want: []string{"https://app.example", "http://localhost:3000"}, ok: true},
		{name: "rejects wildcard", raw: "*"},
		{name: "rejects path", raw: "https://app.example/mcp"},
		{name: "rejects query", raw: "https://app.example?x=1"},
		{name: "rejects unsupported scheme", raw: "ftp://app.example"},
		{name: "rejects empty member", raw: "https://app.example,"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAllowedOrigins(tt.raw)
			if (err == nil) != tt.ok {
				t.Fatalf("error = %v, want success %t", err, tt.ok)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

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

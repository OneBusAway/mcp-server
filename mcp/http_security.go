package main

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"oba-mcp/internal/requestmeta"
)

func protectedHTTPHandler(next http.Handler, allowedOrigins []string, token string) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			if _, ok := allowed[origin]; !ok {
				http.Error(w, "Forbidden origin", http.StatusForbidden)
				return
			}
		}
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !validBearerToken(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="oba-mcp"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		requestID := requestmeta.NormalizeRequestID(r.Header.Get("X-Request-ID"))
		credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		ctx := requestmeta.WithRequestID(r.Context(), requestID)
		ctx = requestmeta.WithCallerHash(ctx, requestmeta.CredentialHash(credential))
		r.Header.Set("X-Request-ID", requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validBearerToken(authorization, expected string) bool {
	if expected == "" || !strings.HasPrefix(authorization, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, "Bearer ")), []byte(expected)) == 1
}

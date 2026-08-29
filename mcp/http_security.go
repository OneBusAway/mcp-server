package main

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func parseAllowedOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	origins := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(value)
		if origin == "" {
			return nil, fmt.Errorf("OBA_ALLOWED_ORIGINS contains an empty origin")
		}
		if origin == "*" {
			return nil, fmt.Errorf("OBA_ALLOWED_ORIGINS must not contain a wildcard")
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.String() != origin {
			return nil, fmt.Errorf("invalid origin %q in OBA_ALLOWED_ORIGINS", origin)
		}
		if _, ok := seen[origin]; !ok {
			seen[origin] = struct{}{}
			origins = append(origins, origin)
		}
	}
	return origins, nil
}

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
		next.ServeHTTP(w, r)
	})
}

func validBearerToken(authorization, expected string) bool {
	if expected == "" || !strings.HasPrefix(authorization, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, "Bearer ")), []byte(expected)) == 1
}

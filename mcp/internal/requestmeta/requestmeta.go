// Package requestmeta carries safe request correlation metadata through the
// MCP handler and upstream client layers.
package requestmeta

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"
)

const maxRequestIDLength = 64

var fallbackID atomic.Uint64

type contextKey uint8

const (
	requestIDKey contextKey = iota
	callerHashKey
)

// NewRequestID returns a random, log-safe request identifier.
func NewRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "fallback-" + strconv.FormatUint(fallbackID.Add(1), 10)
	}
	return hex.EncodeToString(value[:])
}

// NormalizeRequestID accepts only short identifiers that are safe to place in
// logs and response headers. Invalid caller values are replaced.
func NormalizeRequestID(value string) string {
	if value == "" || len(value) > maxRequestIDLength {
		return NewRequestID()
	}
	for _, char := range value {
		if !isRequestIDCharacter(char) {
			return NewRequestID()
		}
	}
	return value
}

func isRequestIDCharacter(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		strings.ContainsRune("._-", char)
}

// CredentialHash produces a short, non-reversible identifier for correlating
// requests made with the same HTTP credential. The credential itself is never
// stored in the context or logs.
func CredentialHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:6])
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

func WithCallerHash(ctx context.Context, callerHash string) context.Context {
	return context.WithValue(ctx, callerHashKey, callerHash)
}

func CallerHash(ctx context.Context) string {
	callerHash, _ := ctx.Value(callerHashKey).(string)
	return callerHash
}

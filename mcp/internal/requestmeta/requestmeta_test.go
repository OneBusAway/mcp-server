package requestmeta

import (
	"context"
	"testing"
)

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		preserved bool
	}{
		{name: "safe", value: "gateway.request-123", preserved: true},
		{name: "empty", value: ""},
		{name: "control character", value: "request\nforged"},
		{name: "space", value: "request forged"},
		{name: "too long", value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeRequestID(test.value)
			if got == "" {
				t.Fatal("normalized request ID is empty")
			}
			if test.preserved && got != test.value {
				t.Fatalf("NormalizeRequestID(%q) = %q, want original value", test.value, got)
			}
			if !test.preserved && got == test.value {
				t.Fatalf("NormalizeRequestID(%q) preserved an unsafe value", test.value)
			}
		})
	}
}

func TestRequestMetadataRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "request-1")
	ctx = WithCallerHash(ctx, "caller-1")
	if got := RequestID(ctx); got != "request-1" {
		t.Fatalf("RequestID = %q, want request-1", got)
	}
	if got := CallerHash(ctx); got != "caller-1" {
		t.Fatalf("CallerHash = %q, want caller-1", got)
	}
}

func TestCredentialHashIsStableAndDoesNotExposeCredential(t *testing.T) {
	first := CredentialHash("secret-token")
	second := CredentialHash("secret-token")
	if first != second {
		t.Fatalf("CredentialHash is not stable: %q != %q", first, second)
	}
	if first == "" || first == "secret-token" {
		t.Fatalf("CredentialHash returned unsafe value %q", first)
	}
	if first == CredentialHash("other-token") {
		t.Fatal("different credentials produced the same test hash")
	}
}

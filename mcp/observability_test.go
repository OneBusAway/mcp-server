package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"oba-mcp/internal/requestmeta"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestToolObservabilityAddsRequestIDAndSafeLogFields(t *testing.T) {
	var logs bytes.Buffer
	var observedToolName string
	middleware := toolObservabilityMiddleware(log.New(&logs, "", 0), nil)
	handler := middleware(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		observedToolName = requestmeta.ToolName(ctx)
		return mcp.NewToolResultStructured(
			tools.SuccessEnvelope[any]{Data: map[string]string{"id": "test_1"}},
			"fixture result",
		), nil
	})
	ctx := requestmeta.WithRequestID(context.Background(), "request-123")
	ctx = requestmeta.WithCallerHash(ctx, "caller-abc")
	result, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "get_stop"}})
	if err != nil {
		t.Fatal(err)
	}
	envelope, ok := result.StructuredContent.(tools.SuccessEnvelope[any])
	if !ok {
		t.Fatalf("structured content = %T, want SuccessEnvelope[any]", result.StructuredContent)
	}
	if envelope.Meta.RequestID != "request-123" {
		t.Fatalf("response request ID = %q, want request-123", envelope.Meta.RequestID)
	}
	if observedToolName != "get_stop" {
		t.Fatalf("handler tool name = %q, want get_stop", observedToolName)
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry["request_id"] != "request-123" || entry["caller_hash"] != "caller-abc" {
		t.Fatalf("correlation log fields = %#v", entry)
	}
	if entry["tool"] != "get_stop" || entry["outcome"] != "success" {
		t.Fatalf("tool log fields = %#v", entry)
	}
}

func TestToolObservabilityDoesNotLogInternalErrorText(t *testing.T) {
	var logs bytes.Buffer
	middleware := toolObservabilityMiddleware(log.New(&logs, "", 0), nil)
	handler := middleware(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, context.DeadlineExceeded
	})
	_, _ = handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "get_stop"}})
	if strings.Contains(logs.String(), context.DeadlineExceeded.Error()) {
		t.Fatalf("log exposed internal error: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"error_code":"SDK_ERROR"`) {
		t.Fatalf("log omitted stable SDK error classification: %s", logs.String())
	}
}

func TestToolObservabilityAddsRequestIDToPublicError(t *testing.T) {
	middleware := toolObservabilityMiddleware(nil, newOperationalMetrics())
	handler := middleware(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := mcp.NewToolResultStructured(
			tools.ErrorEnvelope{Code: "INVALID_ARGUMENT", Message: "Invalid input."},
			"Invalid input.",
		)
		result.IsError = true
		return result, nil
	})
	ctx := requestmeta.WithRequestID(context.Background(), "request-error")
	result, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "get_stop"}})
	if err != nil {
		t.Fatal(err)
	}
	response, ok := result.StructuredContent.(tools.ErrorEnvelope)
	if !ok {
		t.Fatalf("structured content = %T, want ErrorEnvelope", result.StructuredContent)
	}
	if response.RequestID != "request-error" {
		t.Fatalf("error request ID = %q, want request-error", response.RequestID)
	}
}

func TestSafeRecoveryDoesNotExposePanicValue(t *testing.T) {
	var logs bytes.Buffer
	middleware := safeRecoveryMiddleware(log.New(&logs, "", 0))
	handler := middleware(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("database password=secret")
	})
	ctx := requestmeta.WithRequestID(context.Background(), "request-panic")
	result, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "get_stop"}})
	if err != nil {
		t.Fatalf("recovered handler returned SDK error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("recovered result = %#v, want public tool error", result)
	}
	response, ok := result.StructuredContent.(tools.ErrorEnvelope)
	if !ok || response.Code != "INTERNAL_ERROR" || response.RequestID != "request-panic" {
		t.Fatalf("recovered response = %#v", result.StructuredContent)
	}
	if strings.Contains(logs.String(), "database") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("panic log exposed recovered value: %s", logs.String())
	}
}

var _ server.ToolHandlerMiddleware = toolObservabilityMiddleware(nil, nil)

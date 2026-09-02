package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"oba-mcp/internal/requestmeta"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func toolObservabilityMiddleware(appLogger *log.Logger, metrics *operationalMetrics) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx = requestmeta.WithToolName(ctx, request.Params.Name)
			requestID := requestmeta.RequestID(ctx)
			if requestID == "" {
				requestID = requestmeta.NormalizeRequestID(request.Header.Get("X-Request-ID"))
				ctx = requestmeta.WithRequestID(ctx, requestID)
			}
			if requestmeta.CallerHash(ctx) == "" {
				credential := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
				if credential != "" {
					ctx = requestmeta.WithCallerHash(ctx, requestmeta.CredentialHash(credential))
				}
			}

			if metrics != nil {
				metrics.beginTool()
				defer metrics.endTool()
			}
			started := time.Now()
			result, err := next(ctx, request)
			tools.AttachRequestID(result, requestID)
			elapsed := time.Since(started)
			outcome, errorCode, cache, responseBytes := toolCallOutcome(result, err)
			logToolCall(appLogger, ctx, request.Params.Name, outcome, errorCode, cache, elapsed, responseBytes)
			if metrics != nil {
				metrics.observeTool(request.Params.Name, outcome, errorCode, elapsed, responseBytes)
			}
			return result, err
		}
	}
}

func safeRecoveryMiddleware(appLogger *log.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
			defer func() {
				if recover() == nil {
					return
				}
				if appLogger != nil {
					appLogger.Printf(`{"event":"panic","request_id":%q,"tool":%q,"error_code":"INTERNAL_ERROR"}`, requestmeta.RequestID(ctx), request.Params.Name)
				}
				response := tools.ErrorEnvelope{
					Code:      "INTERNAL_ERROR",
					Message:   "The tool could not complete the request.",
					Retryable: false,
					RequestID: requestmeta.RequestID(ctx),
				}
				result = mcp.NewToolResultStructured(response, response.Message)
				result.IsError = true
				err = nil
			}()
			return next(ctx, request)
		}
	}
}

func toolCallOutcome(result *mcp.CallToolResult, err error) (string, string, string, int) {
	outcome := "success"
	errorCode := ""
	cache := ""
	if err != nil {
		outcome = "sdk_error"
		errorCode = "SDK_ERROR"
	} else if result != nil && result.IsError {
		outcome = "tool_error"
		if response, ok := result.StructuredContent.(tools.ErrorEnvelope); ok {
			errorCode = response.Code
		}
	} else if result != nil {
		if response, ok := result.StructuredContent.(tools.SuccessEnvelope[any]); ok {
			cache = response.Meta.Cache
		}
	}
	responseBytes := 0
	if result != nil && result.StructuredContent != nil {
		if encoded, marshalErr := json.Marshal(result.StructuredContent); marshalErr == nil {
			responseBytes = len(encoded)
		}
	}
	return outcome, errorCode, cache, responseBytes
}

func logToolCall(appLogger *log.Logger, ctx context.Context, toolName, outcome, errorCode, cache string, elapsed time.Duration, responseBytes int) {
	if appLogger == nil {
		return
	}
	appLogger.Printf(
		`{"event":"tool","request_id":%q,"caller_hash":%q,"tool":%q,"outcome":%q,"error_code":%q,"cache":%q,"ms":%d,"bytes":%d}`,
		requestmeta.RequestID(ctx), requestmeta.CallerHash(ctx), toolName, outcome, errorCode, cache, elapsed.Milliseconds(), responseBytes,
	)
}

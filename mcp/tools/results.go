package tools

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type toolResultKind uint8

const maxStructuredResultBytes = 128 << 10

const (
	toolResultText toolResultKind = iota
	toolResultData
	toolResultError
)

type toolResult struct {
	kind      toolResultKind
	message   string
	data      any
	suffix    string
	truncated bool
	cache     string
}

// ResponseMeta describes the server-side characteristics of a successful tool response.
type ResponseMeta struct {
	GeneratedAtMS int64  `json:"generated_at_ms"`
	Truncated     bool   `json:"truncated"`
	Cache         string `json:"cache,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

// SuccessEnvelope is the canonical machine-readable shape of a successful tool result.
type SuccessEnvelope[T any] struct {
	Data     T            `json:"data,omitempty"`
	Meta     ResponseMeta `json:"meta"`
	Warnings []string     `json:"warnings,omitempty"`
}

// ErrorEnvelope is the canonical machine-readable shape of an expected tool failure.
type ErrorEnvelope struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	RetryAfterMS *int64 `json:"retry_after_ms,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
}

func textResult(message string) toolResult {
	return toolResult{kind: toolResultText, message: message}
}

func dataResult(message string, data any) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data}
}

func truncatedDataResult(message string, data any) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data, truncated: true}
}

func dataResultWithTruncation(message string, data any, truncated bool) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data, truncated: truncated}
}

func dataResultWithSuffix(message string, data any, suffix string) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data, suffix: suffix}
}

func withCache(result toolResult, cache string) toolResult {
	result.cache = cache
	return result
}

func errorResult(message string) toolResult {
	return toolResult{kind: toolResultError, message: message}
}

// toResult is the sole adapter from typed handler output to MCP result content.
// Text content is a concise display fallback; structured content is canonical.
func toResult(result toolResult) *mcp.CallToolResult {
	switch result.kind {
	case toolResultError:
		errorResponse := publicError(result.message)
		return structuredErrorResult(errorResponse)
	case toolResultText:
		return structuredSuccessResult(result.message, nil, false, result.cache)
	case toolResultData:
		return structuredSuccessResult(resultSummary(result.message, result.suffix), result.data, result.truncated, result.cache)
	default:
		return structuredErrorResult(ErrorEnvelope{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to prepare the tool response.",
		})
	}
}

func resultSummary(message, suffix string) string {
	summary := strings.TrimSpace(message)
	if suffix == "" {
		return summary
	}
	return summary + "\n" + strings.TrimSpace(suffix)
}

func structuredSuccessResult(summary string, data any, truncated bool, cache string) *mcp.CallToolResult {
	response := SuccessEnvelope[any]{
		Data: data,
		Meta: ResponseMeta{GeneratedAtMS: time.Now().UnixMilli(), Truncated: truncated, Cache: cache},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return structuredErrorResult(ErrorEnvelope{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to prepare the tool response.",
		})
	}
	if len(encoded) > maxStructuredResultBytes {
		return structuredErrorResult(ErrorEnvelope{
			Code:    "OUTPUT_TOO_LARGE",
			Message: "The response exceeds the output limit. Narrow the request or use pagination.",
		})
	}
	return mcp.NewToolResultStructured(response, strings.TrimSpace(summary))
}

func structuredErrorResult(response ErrorEnvelope) *mcp.CallToolResult {
	result := mcp.NewToolResultStructured(response, response.Message)
	result.IsError = true
	return result
}

// AttachRequestID adds transport correlation metadata without changing the
// typed data contract returned by an individual tool.
func AttachRequestID(result *mcp.CallToolResult, requestID string) {
	if result == nil || requestID == "" {
		return
	}
	switch response := result.StructuredContent.(type) {
	case SuccessEnvelope[any]:
		response.Meta.RequestID = requestID
		result.StructuredContent = response
	case ErrorEnvelope:
		response.RequestID = requestID
		result.StructuredContent = response
	}
}

func publicError(message string) ErrorEnvelope {
	code, retryable := classifyError(message)
	response := ErrorEnvelope{
		Code:      code,
		Message:   publicErrorMessage(code),
		Retryable: retryable,
	}
	if retryable {
		response.RetryAfterMS = retryAfterFromMessage(message)
	}
	return response
}

func retryAfterFromMessage(message string) *int64 {
	const marker = "retry_after_ms="
	index := strings.Index(message, marker)
	if index < 0 {
		return nil
	}
	value := message[index+len(marker):]
	if end := strings.IndexAny(value, ";)"); end >= 0 {
		value = value[:end]
	}
	milliseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || milliseconds <= 0 {
		return nil
	}
	return &milliseconds
}

func classifyError(message string) (string, bool) {
	switch {
	case strings.HasPrefix(message, "UPSTREAM_CIRCUIT_OPEN"):
		return "UPSTREAM_CIRCUIT_OPEN", true
	case strings.HasPrefix(message, "UPSTREAM_CANCELLED"):
		return "UPSTREAM_CANCELLED", false
	case strings.HasPrefix(message, "UPSTREAM_TIMEOUT"):
		return "UPSTREAM_TIMEOUT", true
	case strings.HasPrefix(message, "UPSTREAM_RATE_LIMITED"):
		return "UPSTREAM_RATE_LIMITED", true
	case strings.HasPrefix(message, "UPSTREAM_UNAVAILABLE"):
		return "UPSTREAM_UNAVAILABLE", true
	case strings.HasPrefix(message, "UPSTREAM_BAD_RESPONSE"):
		return "UPSTREAM_BAD_RESPONSE", false
	case strings.HasPrefix(message, "UPSTREAM_RESPONSE_TOO_LARGE"):
		return "UPSTREAM_RESPONSE_TOO_LARGE", false
	case strings.HasPrefix(message, "OUTPUT_TOO_LARGE"):
		return "OUTPUT_TOO_LARGE", false
	}
	if strings.HasPrefix(message, "parsing ") || strings.HasPrefix(message, "OBA error ") {
		return "UPSTREAM_BAD_RESPONSE", false
	}
	return "INVALID_ARGUMENT", false
}

func publicErrorMessage(code string) string {
	switch code {
	case "INVALID_ARGUMENT":
		return "One or more tool arguments are invalid."
	case "UPSTREAM_CANCELLED":
		return "The request was cancelled."
	case "UPSTREAM_TIMEOUT":
		return "The transit service timed out."
	case "UPSTREAM_RATE_LIMITED":
		return "The transit service is rate limiting requests."
	case "UPSTREAM_UNAVAILABLE", "UPSTREAM_CIRCUIT_OPEN":
		return "The transit service is temporarily unavailable."
	case "UPSTREAM_BAD_RESPONSE", "UPSTREAM_RESPONSE_TOO_LARGE":
		return "The transit service returned an unusable response."
	case "OUTPUT_TOO_LARGE":
		return "The response exceeds the output limit. Narrow the request or use pagination."
	default:
		return "Unable to prepare the tool response."
	}
}

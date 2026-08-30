package tools

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

type toolResultKind uint8

const (
	toolResultText toolResultKind = iota
	toolResultData
	toolResultError
)

type toolResult struct {
	kind    toolResultKind
	message string
	data    any
	suffix  string
}

func textResult(message string) toolResult {
	return toolResult{kind: toolResultText, message: message}
}

func dataResult(message string, data any) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data}
}

func dataResultWithSuffix(message string, data any, suffix string) toolResult {
	return toolResult{kind: toolResultData, message: message, data: data, suffix: suffix}
}

func errorResult(message string) toolResult {
	return toolResult{kind: toolResultError, message: message}
}

// toResult is the sole adapter from typed handler output to MCP result content.
// Phase 4 can replace it with structured MCP content without changing handlers.
func toResult(result toolResult) *mcp.CallToolResult {
	switch result.kind {
	case toolResultError:
		return mcp.NewToolResultError(result.message)
	case toolResultText:
		return mcp.NewToolResultText(result.message)
	case toolResultData:
		data, err := json.MarshalIndent(result.data, "", "  ")
		if err != nil {
			return mcp.NewToolResultError("Unable to prepare the tool response.")
		}
		return mcp.NewToolResultText(result.message + string(data) + result.suffix)
	default:
		return mcp.NewToolResultError("Unable to prepare the tool response.")
	}
}

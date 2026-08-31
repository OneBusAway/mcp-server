package evals

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"oba-mcp/client"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestSuiteExecutesAgainstDeterministicFixtures runs every eval case through
// the actual MCP tool handlers using a deterministic fixture. It closes the
// structural-only gap between `make eval-test` and reality: a stale expected
// argument, a regressed error code, or a broken success path fails CI here
// rather than at release time.
func TestSuiteExecutesAgainstDeterministicFixtures(t *testing.T) {
	for _, filename := range []string{"all-tools-v1.json", "scenarios-v1.json"} {
		suite, err := Load(filepath.Join(filename))
		if err != nil {
			t.Fatalf("load %s: %v", filename, err)
		}
		t.Run(suite.Version, func(t *testing.T) {
			for _, testCase := range suite.Cases {
				t.Run(testCase.ID, func(t *testing.T) {
					executeEvalCase(t, testCase)
				})
			}
		})
	}
}

func executeEvalCase(t *testing.T, testCase Case) {
	t.Helper()

	fixture, err := StartFixture(testCase)
	if err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	t.Cleanup(fixture.Close)

	obaClient := client.New(fixture.URL, "fixture-api-key", nil, nil)
	mcpServer := server.NewMCPServer("eval-runner", "1.0.0", server.WithToolCapabilities(true))
	tools.RegisterAll(mcpServer, obaClient)

	for index, call := range testCase.ExpectedToolCalls {
		arguments := decodeArguments(t, call.Arguments)
		entry, ok := mcpServer.ListTools()[call.Name]
		if !ok {
			t.Fatalf("tool %q is not registered", call.Name)
		}

		result, err := entry.Handler(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: call.Name, Arguments: arguments},
		})
		if err != nil {
			t.Fatalf("call %d (%s) returned protocol error: %v", index+1, call.Name, err)
		}
		if result == nil {
			t.Fatalf("call %d (%s) returned nil result", index+1, call.Name)
		}

		switch testCase.Type {
		case "success":
			assertSuccessResult(t, result)
		case "invalid_argument", "upstream_failure":
			assertErrorResult(t, result, testCase.ExpectedErrorCode)
		}
	}
}

func assertSuccessResult(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result.IsError {
		t.Fatalf("success case returned tool error: code=%q message=%q", errorCodeOf(result), errorMessageOf(result))
	}
	if _, ok := result.StructuredContent.(tools.SuccessEnvelope[any]); !ok {
		t.Fatalf("structured content = %T, want tools.SuccessEnvelope[any]", result.StructuredContent)
	}
}

func assertErrorResult(t *testing.T, result *mcp.CallToolResult, wantCode string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("expected tool error, got success: structured=%#v", result.StructuredContent)
	}
	envelope, ok := result.StructuredContent.(tools.ErrorEnvelope)
	if !ok {
		t.Fatalf("error structured content = %T, want tools.ErrorEnvelope", result.StructuredContent)
	}
	if envelope.Code != wantCode {
		t.Fatalf("error code = %q, want %q (message=%q)", envelope.Code, wantCode, envelope.Message)
	}
	if envelope.Message == "" {
		t.Fatal("error message is empty; public error contract requires a safe message")
	}
}

func decodeArguments(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		return map[string]any{}
	}
	arguments := make(map[string]any)
	if err := json.Unmarshal(raw, &arguments); err != nil {
		t.Fatalf("decode arguments: %v", err)
	}
	return arguments
}

func errorCodeOf(result *mcp.CallToolResult) string {
	if envelope, ok := result.StructuredContent.(tools.ErrorEnvelope); ok {
		return envelope.Code
	}
	return "<unknown>"
}

func errorMessageOf(result *mcp.CallToolResult) string {
	if envelope, ok := result.StructuredContent.(tools.ErrorEnvelope); ok {
		return envelope.Message
	}
	return ""
}

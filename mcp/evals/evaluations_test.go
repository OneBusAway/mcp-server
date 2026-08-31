package evals

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"oba-mcp/client"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/server"
)

func TestAllToolsV1SuiteCoversEveryRegisteredTool(t *testing.T) {
	suite, err := Load(filepath.Join("all-tools-v1.json"))
	if err != nil {
		t.Fatalf("load all-tools suite: %v", err)
	}
	if suite.Version != "all-tools-v1" {
		t.Fatalf("suite version = %q, want all-tools-v1", suite.Version)
	}
	registered := registeredToolNames()
	validateSuiteCases(t, suite, registered)
	if len(suite.Cases) < len(registered)*2 {
		t.Fatalf("evaluation cases = %d, want at least one success and one failure case for each of %d tools", len(suite.Cases), len(registered))
	}

	coverage := make(map[string]map[string]int, len(registered))
	for _, testCase := range suite.Cases {
		for _, call := range testCase.ExpectedToolCalls {
			if coverage[call.Name] == nil {
				coverage[call.Name] = make(map[string]int, 4)
			}
			coverage[call.Name][testCase.Type]++
		}
	}
	for toolName := range registered {
		cases := coverage[toolName]
		if cases["success"] == 0 {
			t.Errorf("tool %q is missing a success case", toolName)
		}
		if cases["invalid_argument"]+cases["upstream_failure"] == 0 {
			t.Errorf("tool %q is missing a failure case", toolName)
		}
	}
}

func TestScenariosV1SuiteIsValid(t *testing.T) {
	suite, err := Load(filepath.Join("scenarios-v1.json"))
	if err != nil {
		t.Fatalf("load scenarios suite: %v", err)
	}
	if suite.Version != "scenarios-v1" {
		t.Fatalf("suite version = %q, want scenarios-v1", suite.Version)
	}
	validateSuiteCases(t, suite, registeredToolNames())
	if len(suite.Cases) < 8 {
		t.Fatalf("scenario cases = %d, want at least 8 adversarial/workflow cases", len(suite.Cases))
	}
}

func validateSuiteCases(t *testing.T, suite Suite, registered map[string]bool) {
	t.Helper()
	seen := make(map[string]struct{}, len(suite.Cases))
	for _, testCase := range suite.Cases {
		if testCase.ID == "" || testCase.Prompt == "" || testCase.ExpectedOutcome == "" {
			t.Fatalf("incomplete evaluation case: %#v", testCase)
		}
		if testCase.Type != "success" && testCase.Type != "invalid_argument" && testCase.Type != "upstream_failure" {
			t.Fatalf("case %q has unsupported type %q", testCase.ID, testCase.Type)
		}
		if _, duplicate := seen[testCase.ID]; duplicate {
			t.Fatalf("duplicate evaluation case ID %q", testCase.ID)
		}
		seen[testCase.ID] = struct{}{}
		if len(testCase.ExpectedToolCalls) == 0 {
			t.Fatalf("case %q has no expected tool calls", testCase.ID)
		}
		if testCase.MaxToolCalls < len(testCase.ExpectedToolCalls) {
			t.Fatalf("case %q max tool calls %d is less than required calls %d", testCase.ID, testCase.MaxToolCalls, len(testCase.ExpectedToolCalls))
		}
		if testCase.Type != "success" && len(testCase.ExpectedToolCalls) != 1 {
			t.Fatalf("failure case %q must contain exactly one expected tool call", testCase.ID)
		}
		for _, call := range testCase.ExpectedToolCalls {
			if call.Name == "" || !json.Valid(call.Arguments) {
				t.Fatalf("case %q has invalid expected tool call %#v", testCase.ID, call)
			}
			if !registered[call.Name] {
				t.Fatalf("case %q references unregistered tool %q", testCase.ID, call.Name)
			}
		}
		if testCase.Type == "success" && testCase.ExpectedErrorCode != "" {
			t.Fatalf("success case %q must not expect an error code", testCase.ID)
		}
		if testCase.Type != "success" && testCase.ExpectedErrorCode == "" {
			t.Fatalf("failure case %q must declare a stable public error code", testCase.ID)
		}
		if testCase.AllowNoToolCall && testCase.Type != "invalid_argument" {
			t.Fatalf("case %q may allow no tool call only for invalid arguments", testCase.ID)
		}
		for _, term := range append(append([]string(nil), testCase.RequiredResponseTerms...), testCase.ForbiddenResponseTerms...) {
			if strings.TrimSpace(term) == "" {
				t.Fatalf("case %q contains an empty response term", testCase.ID)
			}
		}
		validatePromptArguments(t, testCase)
	}
}

func registeredToolNames() map[string]bool {
	obaClient := client.New("http://example.invalid", "test-key", nil, nil)
	mcpServer := server.NewMCPServer("test", "test")
	tools.RegisterAll(mcpServer, obaClient)
	registered := make(map[string]bool)
	for name := range mcpServer.ListTools() {
		registered[name] = true
	}
	return registered
}

func validatePromptArguments(t *testing.T, testCase Case) {
	t.Helper()
	if len(testCase.PromptArgumentKeys) == 0 {
		return
	}
	arguments := make(map[string]json.RawMessage)
	for _, call := range testCase.ExpectedToolCalls {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(call.Arguments, &values); err != nil {
			t.Fatalf("case %q decode expected arguments: %v", testCase.ID, err)
		}
		for key, value := range values {
			arguments[key] = value
		}
	}
	for _, key := range testCase.PromptArgumentKeys {
		rawValue, ok := arguments[key]
		if !ok {
			t.Fatalf("case %q prompt argument key %q is not a non-empty string expected argument", testCase.ID, key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil || value == "" {
			t.Fatalf("case %q prompt argument key %q is not a non-empty string expected argument", testCase.ID, key)
		}
		if !containsExactArgumentValue(testCase.Prompt, value) {
			t.Fatalf("case %q prompt does not contain expected %s value %q", testCase.ID, key, value)
		}
	}
}

func containsExactArgumentValue(prompt, value string) bool {
	for start := 0; ; {
		index := strings.Index(prompt[start:], value)
		if index < 0 {
			return false
		}
		index += start
		end := index + len(value)
		beforeIsID := index > 0 && isEntityIDCharacter(prompt[index-1])
		afterIsID := end < len(prompt) && isEntityIDCharacter(prompt[end])
		if !beforeIsID && !afterIsID {
			return true
		}
		start = end
	}
}

// isEntityIDCharacter mirrors the entity-ID grammar accepted by validation.EntityID
// so a partial match inside a larger raw ID (e.g. expected "agency" appearing
// inside "agency.other") is rejected as a boundary. `.` is included because
// OBA IDs allow dots; matcher callers must delimit raw IDs from surrounding
// prose (e.g. via backticks) so sentence punctuation does not break the check.
func isEntityIDCharacter(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '_' || value == '-' || value == ':' || value == '.'
}

// Package evals defines the versioned natural-language MCP evaluation format.
// Suites are model-agnostic: a runner supplies the model/client and scores the
// transcript against the deterministic expectations in the suite.
package evals

import (
	"encoding/json"
	"os"
)

// Suite is a versioned set of MCP catalog evaluations.
type Suite struct {
	Version string `json:"version"`
	Cases   []Case `json:"cases"`
}

// Case describes one success or controlled failure path for an MCP tool.
type Case struct {
	ID                     string         `json:"id"`
	Type                   string         `json:"type"`
	Prompt                 string         `json:"prompt"`
	Fixture                string         `json:"fixture,omitempty"`
	ExpectedToolCalls      []ExpectedCall `json:"expected_tool_calls"`
	AllowNoToolCall        bool           `json:"allow_no_tool_call,omitempty"`
	PromptArgumentKeys     []string       `json:"prompt_argument_keys,omitempty"`
	MaxToolCalls           int            `json:"max_tool_calls"`
	ExpectedErrorCode      string         `json:"expected_error_code,omitempty"`
	ExpectedOutcome        string         `json:"expected_outcome"`
	RequiredResponseTerms  []string       `json:"required_response_terms,omitempty"`
	ForbiddenResponseTerms []string       `json:"forbidden_response_terms,omitempty"`
}

// ExpectedCall defines one required tool call. Arguments remain raw JSON at
// this model/client boundary so exact values and numeric precision are kept.
type ExpectedCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Load reads and decodes an evaluation suite.
func Load(path string) (Suite, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, err
	}
	var suite Suite
	if err := json.Unmarshal(contents, &suite); err != nil {
		return Suite{}, err
	}
	return suite, nil
}

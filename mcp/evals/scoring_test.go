package evals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScenarioSuiteScoresExpectedTranscript(t *testing.T) {
	suite, err := Load(filepath.Join("scenarios-v1.json"))
	if err != nil {
		t.Fatalf("load scenarios suite: %v", err)
	}
	transcript := Transcript{
		SuiteVersion: suite.Version,
		Profile:      TranscriptProfile{ID: "fixture", Client: "direct-mcp", Provider: "fixture", Model: "deterministic"},
		Cases:        make([]TranscriptCase, 0, len(suite.Cases)),
	}
	for _, testCase := range suite.Cases {
		calls := make([]ObservedCall, len(testCase.ExpectedToolCalls))
		for index, call := range testCase.ExpectedToolCalls {
			calls[index] = ObservedCall{Name: call.Name, Arguments: call.Arguments, ErrorCode: testCase.ExpectedErrorCode}
		}
		transcript.Cases = append(transcript.Cases, TranscriptCase{ID: testCase.ID, ToolCalls: calls})
	}

	report := ScoreTranscript(suite, transcript)
	if !report.DeterministicPass || report.Passed != len(suite.Cases) {
		t.Fatalf("scenario score report = %#v, want every declared call to pass", report)
	}
}

func TestScoreTranscriptAcceptsExactCallsAndSemanticJSON(t *testing.T) {
	suite := Suite{Version: "test-v1", Cases: []Case{{
		ID: "workflow", Type: "success", Prompt: "test", MaxToolCalls: 2,
		ExpectedOutcome: "uses both tools",
		ExpectedToolCalls: []ExpectedCall{
			{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)},
			{Name: "get_arrivals_for_stop", Arguments: json.RawMessage(`{"minutes_after":60,"stop_id":"test_1"}`)},
		},
		RequiredResponseTerms:  []string{"no arrivals"},
		ForbiddenResponseTerms: []string{"the ui"},
	}}}
	transcript := Transcript{
		SuiteVersion: "test-v1",
		Profile:      TranscriptProfile{ID: "local", Client: "direct-mcp", Provider: "ollama", Model: "test-model"},
		Cases: []TranscriptCase{{
			ID: "workflow",
			ToolCalls: []ObservedCall{
				{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)},
				{Name: "get_arrivals_for_stop", Arguments: json.RawMessage(`{"stop_id":"test_1","minutes_after":60.0}`)},
			},
			Response: "There are no arrivals right now.",
		}},
	}

	report := ScoreTranscript(suite, transcript)
	if !report.DeterministicPass || report.Passed != 1 || report.Failed != 0 {
		t.Fatalf("score report = %#v, want deterministic pass", report)
	}
	if !report.OutcomeReviewRequired || !report.Cases[0].OutcomeReviewRequired {
		t.Fatal("semantic expected_outcome must remain explicitly marked for review")
	}
}

func TestScoreTranscriptReportsCallBudgetArgumentsAndResponseFailures(t *testing.T) {
	suite := Suite{Version: "test-v1", Cases: []Case{{
		ID: "single", Type: "success", Prompt: "test", MaxToolCalls: 1,
		ExpectedOutcome:        "one exact call",
		ExpectedToolCalls:      []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)}},
		RequiredResponseTerms:  []string{"stop"},
		ForbiddenResponseTerms: []string{"automatic"},
	}}}
	transcript := Transcript{SuiteVersion: "test-v1", Profile: TranscriptProfile{ID: "test", Client: "test", Provider: "test", Model: "test"}, Cases: []TranscriptCase{{
		ID: "single",
		ToolCalls: []ObservedCall{
			{Name: "search_stops", Arguments: json.RawMessage(`{"query":"test_1"}`)},
			{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)},
		},
		Response: "The map updates automatically.",
	}}}

	report := ScoreTranscript(suite, transcript)
	if report.DeterministicPass || report.Failed != 1 || len(report.Cases[0].Failures) < 4 {
		t.Fatalf("score report = %#v, want call-budget, tool, argument, and response failures", report)
	}
}

func TestScoreTranscriptReportsMissingUnexpectedDuplicateAndVersionCases(t *testing.T) {
	suite := Suite{Version: "suite-v1", Cases: []Case{
		{ID: "missing", Type: "success", Prompt: "test", MaxToolCalls: 1, ExpectedOutcome: "test", ExpectedToolCalls: []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{}`)}}},
	}}
	transcript := Transcript{SuiteVersion: "wrong-v1", Profile: TranscriptProfile{ID: "test", Client: "test", Provider: "test", Model: "test"}, Cases: []TranscriptCase{
		{ID: "unexpected"},
		{ID: "unexpected"},
	}}

	report := ScoreTranscript(suite, transcript)
	if report.DeterministicPass || report.Failed != 1 || len(report.Failures) != 2 {
		t.Fatalf("score report = %#v, want one failed suite case and two transcript-level failures", report)
	}
}

func TestScoreTranscriptChecksPublicErrorCode(t *testing.T) {
	suite := Suite{Version: "test-v1", Cases: []Case{{
		ID: "rate-limit", Type: "upstream_failure", Prompt: "test", MaxToolCalls: 1,
		ExpectedOutcome: "stable rate-limit error", ExpectedErrorCode: "UPSTREAM_RATE_LIMITED",
		ExpectedToolCalls: []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)}},
	}}}
	transcript := Transcript{
		SuiteVersion: "test-v1",
		Profile:      TranscriptProfile{ID: "test", Client: "test", Provider: "test", Model: "test"},
		Cases: []TranscriptCase{{ID: "rate-limit", ToolCalls: []ObservedCall{{
			Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`), ErrorCode: "UPSTREAM_UNAVAILABLE",
		}}}},
	}

	report := ScoreTranscript(suite, transcript)
	if report.DeterministicPass || len(report.Cases[0].Failures) != 1 {
		t.Fatalf("score report = %#v, want public error-code mismatch", report)
	}
}

func TestScoreTranscriptAcceptsSafeInvalidArgumentRefusal(t *testing.T) {
	suite := Suite{Version: "test-v1", Cases: []Case{{
		ID: "invalid-id", Type: "invalid_argument", Prompt: "test", MaxToolCalls: 1,
		ExpectedOutcome: "rejects invalid input", ExpectedErrorCode: "INVALID_ARGUMENT", AllowNoToolCall: true,
		ExpectedToolCalls: []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"../../metadata"}`)}},
	}}}
	transcript := Transcript{
		SuiteVersion: "test-v1",
		Profile:      TranscriptProfile{ID: "test", Client: "test", Provider: "test", Model: "test"},
		Cases:        []TranscriptCase{{ID: "invalid-id", Response: "That stop ID is invalid."}},
	}

	report := ScoreTranscript(suite, transcript)
	if !report.DeterministicPass || report.Passed != 1 {
		t.Fatalf("score report = %#v, want safe refusal to pass", report)
	}
}

func TestScoreTranscriptFailsRunnerError(t *testing.T) {
	suite := Suite{Version: "test-v1", Cases: []Case{{
		ID: "runner-failure", Type: "success", Prompt: "test", MaxToolCalls: 1,
		ExpectedOutcome:   "one call",
		ExpectedToolCalls: []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)}},
	}}}
	transcript := Transcript{
		SuiteVersion: "test-v1",
		Profile:      TranscriptProfile{ID: "test", Client: "test", Provider: "test", Model: "test"},
		Cases: []TranscriptCase{{
			ID:        "runner-failure",
			ToolCalls: []ObservedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1"}`)}},
			RunnerError: &RunnerError{
				Code:    runnerErrorToolRoundLimit,
				Message: "The model exceeded the configured tool-round limit.",
			},
		}},
	}

	report := ScoreTranscript(suite, transcript)
	if report.DeterministicPass || report.Failed != 1 {
		t.Fatalf("score report = %#v, want runner error to fail", report)
	}
}

func TestWriteTranscriptUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "transcript.json")
	transcript := Transcript{SuiteVersion: "test-v1"}
	if err := WriteTranscript(path, transcript); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat transcript: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("transcript permissions = %o, want 600", permissions)
	}
	loaded, err := LoadTranscript(path)
	if err != nil {
		t.Fatalf("load transcript: %v", err)
	}
	if loaded.SuiteVersion != transcript.SuiteVersion {
		t.Fatalf("suite version = %q, want %q", loaded.SuiteVersion, transcript.SuiteVersion)
	}
}

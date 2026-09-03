package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Transcript is a credential-free record produced by a model/client eval run.
// It contains model-selected calls, public MCP results, and final responses;
// raw provider request/response payloads must not be committed.
type Transcript struct {
	SuiteVersion string            `json:"suite_version"`
	Profile      TranscriptProfile `json:"profile"`
	Cases        []TranscriptCase  `json:"cases"`
}

// TranscriptProfile identifies the client/model combination under evaluation.
type TranscriptProfile struct {
	ID       string `json:"id"`
	Client   string `json:"client"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// TranscriptCase records the observable result for one suite case.
type TranscriptCase struct {
	ID          string         `json:"id"`
	ToolCalls   []ObservedCall `json:"tool_calls"`
	Response    string         `json:"response"`
	RunnerError *RunnerError   `json:"runner_error,omitempty"`
}

// ObservedCall is one model-selected tool invocation.
type ObservedCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	ErrorCode string          `json:"error_code,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

// RunnerError describes an evaluation-runner failure rather than a public MCP
// tool error. AttemptedCall is populated when a safety limit blocks execution.
type RunnerError struct {
	Code          string        `json:"code"`
	Message       string        `json:"message"`
	AttemptedCall *ObservedCall `json:"attempted_call,omitempty"`
}

// ScoreReport contains deterministic call/response-term scoring. The semantic
// expected_outcome still requires a human or separately configured judge.
type ScoreReport struct {
	SuiteVersion          string            `json:"suite_version"`
	Profile               TranscriptProfile `json:"profile"`
	Total                 int               `json:"total"`
	Passed                int               `json:"passed"`
	Failed                int               `json:"failed"`
	DeterministicPass     bool              `json:"deterministic_pass"`
	OutcomeReviewRequired bool              `json:"outcome_review_required"`
	Failures              []string          `json:"failures,omitempty"`
	Cases                 []CaseScore       `json:"cases"`
}

// CaseScore explains any deterministic mismatch for one case.
type CaseScore struct {
	ID                    string   `json:"id"`
	Pass                  bool     `json:"pass"`
	Failures              []string `json:"failures,omitempty"`
	ExpectedOutcome       string   `json:"expected_outcome"`
	OutcomeReviewRequired bool     `json:"outcome_review_required"`
}

// LoadTranscript reads a model/client transcript from disk.
func LoadTranscript(path string) (Transcript, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Transcript{}, err
	}
	var transcript Transcript
	if err := json.Unmarshal(contents, &transcript); err != nil {
		return Transcript{}, err
	}
	return transcript, nil
}

// WriteTranscript atomically writes a credential-free transcript with
// owner-only permissions. Existing output is replaced only after encoding
// succeeds.
func WriteTranscript(path string, transcript Transcript) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create transcript directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".transcript-*")
	if err != nil {
		return fmt.Errorf("create transcript: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect transcript: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(transcript); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode transcript: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close transcript: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish transcript: %w", err)
	}
	removeTemporary = false
	return nil
}

// ScoreTranscript checks exact ordered tool calls, semantic JSON arguments,
// call budgets, and optional required/forbidden response terms.
func ScoreTranscript(suite Suite, transcript Transcript) ScoreReport {
	report := ScoreReport{
		SuiteVersion:          suite.Version,
		Profile:               transcript.Profile,
		Total:                 len(suite.Cases),
		OutcomeReviewRequired: true,
		Cases:                 make([]CaseScore, 0, len(suite.Cases)),
	}
	if transcript.Profile.ID == "" || transcript.Profile.Client == "" || transcript.Profile.Provider == "" || transcript.Profile.Model == "" {
		report.Failures = append(report.Failures, "transcript profile must include id, client, provider, and model")
	}
	observed := make(map[string]TranscriptCase, len(transcript.Cases))
	duplicates := make(map[string]bool)
	for _, result := range transcript.Cases {
		if _, exists := observed[result.ID]; exists {
			duplicates[result.ID] = true
		}
		observed[result.ID] = result
	}

	for _, testCase := range suite.Cases {
		score := CaseScore{
			ID:                    testCase.ID,
			ExpectedOutcome:       testCase.ExpectedOutcome,
			OutcomeReviewRequired: true,
		}
		result, ok := observed[testCase.ID]
		if !ok {
			score.Failures = append(score.Failures, "transcript case is missing")
		} else {
			if duplicates[testCase.ID] {
				score.Failures = append(score.Failures, "transcript contains the case more than once")
			}
			score.Failures = append(score.Failures, scoreCalls(testCase, result.ToolCalls)...)
			if result.RunnerError != nil {
				score.Failures = append(score.Failures, fmt.Sprintf("runner error %s: %s", result.RunnerError.Code, result.RunnerError.Message))
			} else {
				score.Failures = append(score.Failures, scoreResponseTerms(testCase, result.Response)...)
			}
		}
		score.Pass = len(score.Failures) == 0
		if score.Pass {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Cases = append(report.Cases, score)
	}
	if transcript.SuiteVersion != suite.Version {
		report.Failures = append(report.Failures, fmt.Sprintf("transcript suite version %q does not match %q", transcript.SuiteVersion, suite.Version))
	}
	for id := range observed {
		if !suiteHasCase(suite, id) {
			report.Failures = append(report.Failures, fmt.Sprintf("transcript case %q is not present in the suite", id))
		}
	}
	report.DeterministicPass = report.Failed == 0 && len(report.Failures) == 0
	return report
}

func scoreCalls(testCase Case, observed []ObservedCall) []string {
	var failures []string
	if len(observed) == 0 && testCase.AllowNoToolCall {
		return failures
	}
	if len(observed) > testCase.MaxToolCalls {
		failures = append(failures, fmt.Sprintf("tool calls = %d, exceeds max_tool_calls %d", len(observed), testCase.MaxToolCalls))
	}
	if len(observed) != len(testCase.ExpectedToolCalls) {
		failures = append(failures, fmt.Sprintf("tool calls = %d, want %d", len(observed), len(testCase.ExpectedToolCalls)))
	}
	for index := 0; index < min(len(observed), len(testCase.ExpectedToolCalls)); index++ {
		want := testCase.ExpectedToolCalls[index]
		got := observed[index]
		if got.Name != want.Name {
			failures = append(failures, fmt.Sprintf("call %d tool = %q, want %q", index+1, got.Name, want.Name))
		}
		if !jsonContainsExpected(got.Arguments, want.Arguments) {
			failures = append(failures, fmt.Sprintf("call %d arguments = %s, want %s", index+1, printableJSON(got.Arguments), printableJSON(want.Arguments)))
		}
		if testCase.ExpectedErrorCode != "" && got.ErrorCode != testCase.ExpectedErrorCode {
			failures = append(failures, fmt.Sprintf("call %d error_code = %q, want %q", index+1, got.ErrorCode, testCase.ExpectedErrorCode))
		}
		if testCase.ExpectedErrorCode == "" && got.ErrorCode != "" {
			failures = append(failures, fmt.Sprintf("call %d returned unexpected error_code %q", index+1, got.ErrorCode))
		}
	}
	return failures
}

func scoreResponseTerms(testCase Case, response string) []string {
	var failures []string
	normalized := strings.ToLower(response)
	for _, term := range testCase.RequiredResponseTerms {
		if !strings.Contains(normalized, strings.ToLower(term)) {
			failures = append(failures, fmt.Sprintf("response is missing required term %q", term))
		}
	}
	for _, term := range testCase.ForbiddenResponseTerms {
		if containsStandaloneTerm(normalized, strings.ToLower(term)) {
			failures = append(failures, fmt.Sprintf("response contains forbidden term %q", term))
		}
	}
	return failures
}

// containsStandaloneTerm avoids treating a forbidden phrase as present when it
// is only a prefix or suffix of a larger word (for example, "feed is current"
// inside "feed is currently stale").
func containsStandaloneTerm(text, term string) bool {
	if term == "" {
		return false
	}
	for start := 0; ; {
		offset := strings.Index(text[start:], term)
		if offset < 0 {
			return false
		}
		start += offset
		end := start + len(term)
		if isTermBoundaryBefore(text, start) && isTermBoundaryAfter(text, end) {
			return true
		}
		start = end
	}
}

func isTermBoundaryBefore(text string, offset int) bool {
	if offset == 0 {
		return true
	}
	runeValue, _ := utf8.DecodeLastRuneInString(text[:offset])
	return !unicode.IsLetter(runeValue) && !unicode.IsNumber(runeValue)
}

func isTermBoundaryAfter(text string, offset int) bool {
	if offset == len(text) {
		return true
	}
	runeValue, _ := utf8.DecodeRuneInString(text[offset:])
	return !unicode.IsLetter(runeValue) && !unicode.IsNumber(runeValue)
}

// jsonContainsExpected checks that every key-value pair in expected exists in
// actual with an equal value. Extra keys in actual are ignored, so a model
// that passes optional parameters with valid values does not fail the check.
func jsonContainsExpected(actual, expected json.RawMessage) bool {
	var actualVal, expectedVal any
	if json.Unmarshal(actual, &actualVal) != nil || json.Unmarshal(expected, &expectedVal) != nil {
		return false
	}
	expectedMap, isExpectedMap := expectedVal.(map[string]any)
	actualMap, isActualMap := actualVal.(map[string]any)
	if !isExpectedMap || !isActualMap {
		return reflect.DeepEqual(actualVal, expectedVal)
	}
	for key, want := range expectedMap {
		got, exists := actualMap[key]
		if !exists || !reflect.DeepEqual(got, want) {
			return false
		}
	}
	return true
}

func printableJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "<empty>"
	}
	return string(raw)
}

func suiteHasCase(suite Suite, id string) bool {
	for _, testCase := range suite.Cases {
		if testCase.ID == id {
			return true
		}
	}
	return false
}

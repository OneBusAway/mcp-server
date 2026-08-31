package evals

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"oba-mcp/client"
	"oba-mcp/tools"

	"github.com/mark3labs/mcp-go/server"
)

func TestRunOpenAICompatibleExecutesModelSelectedTool(t *testing.T) {
	var mu sync.Mutex
	var requests []chatCompletionRequest
	var authorization []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, request)
		authorization = append(authorization, r.Header.Get("Authorization"))
		requestNumber := len(requests)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if requestNumber == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_stop","arguments":"{\"stop_id\":\"test_1013\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Fixture Stop is served by route 10."}}]}`))
	}))
	t.Cleanup(provider.Close)

	suite := Suite{Version: "live-test-v1", Cases: []Case{{
		ID:     "known-stop",
		Type:   "success",
		Prompt: "Where is stop `test_1013`?",
	}}}
	transcript, err := RunOpenAICompatible(t.Context(), suite, LiveConfig{
		Profile: TranscriptProfile{
			ID:       "fake-model",
			Client:   "direct-mcp",
			Provider: "fake",
			Model:    "fixture-model",
		},
		BaseURL:     provider.URL + "/v1",
		APIKey:      "secret-token",
		ToolProfile: tools.ToolProfileAll,
	})
	if err != nil {
		t.Fatalf("run live evaluation: %v", err)
	}
	if len(transcript.Cases) != 1 {
		t.Fatalf("transcript cases = %d, want 1", len(transcript.Cases))
	}
	result := transcript.Cases[0]
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "get_stop" {
		t.Fatalf("tool calls = %#v, want one get_stop call", result.ToolCalls)
	}
	if string(result.ToolCalls[0].Arguments) != `{"stop_id":"test_1013"}` {
		t.Fatalf("arguments = %s", result.ToolCalls[0].Arguments)
	}
	if result.Response != "Fixture Stop is served by route 10." {
		t.Fatalf("response = %q", result.Response)
	}
	if !strings.Contains(string(result.ToolCalls[0].Result), "Fixture Stop") {
		t.Fatalf("recorded tool result = %s, want public structured stop data", result.ToolCalls[0].Result)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("model requests = %d, want 2", len(requests))
	}
	if len(requests[0].Tools) != 29 {
		t.Fatalf("advertised tools = %d, want 29", len(requests[0].Tools))
	}
	if len(requests[1].Messages) == 0 || requests[1].Messages[len(requests[1].Messages)-1].Role != "tool" {
		t.Fatal("second model request does not include the tool result")
	}
	if authorization[0] != "Bearer secret-token" || authorization[1] != "Bearer secret-token" {
		t.Fatalf("authorization headers = %#v", authorization)
	}
	encoded, err := json.Marshal(transcript)
	if err != nil {
		t.Fatalf("encode transcript: %v", err)
	}
	if strings.Contains(string(encoded), "secret-token") {
		t.Fatal("transcript contains provider credential")
	}
}

func TestChatCompletionsEndpointValidation(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
		wantErr bool
	}{
		{name: "base", baseURL: "http://localhost:11434/v1", want: "http://localhost:11434/v1/chat/completions"},
		{name: "complete endpoint", baseURL: "https://example.com/v1/chat/completions", want: "https://example.com/v1/chat/completions"},
		{name: "relative", baseURL: "/v1", wantErr: true},
		{name: "credentials", baseURL: "https://user:pass@example.com/v1", wantErr: true},
		{name: "query", baseURL: "https://example.com/v1?token=secret", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := chatCompletionsEndpoint(test.baseURL)
			if test.wantErr && err == nil {
				t.Fatalf("chatCompletionsEndpoint(%q) succeeded, want error", test.baseURL)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("chatCompletionsEndpoint(%q): %v", test.baseURL, err)
			}
			if got != test.want {
				t.Fatalf("endpoint = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRunOpenAICompatibleRecordsPublicToolError(t *testing.T) {
	var requestNumber atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		currentRequest := requestNumber.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if currentRequest == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_arrivals_for_stop","arguments":"{\"stop_id\":\"test_1013\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"The transit service is rate limited; try again later."}}]}`))
	}))
	t.Cleanup(provider.Close)

	suite := Suite{Version: "live-error-v1", Cases: []Case{{
		ID:                "rate-limited",
		Type:              "upstream_failure",
		Prompt:            "Get arrivals while the service is rate limited.",
		ExpectedErrorCode: "UPSTREAM_RATE_LIMITED",
	}}}
	transcript, err := RunOpenAICompatible(t.Context(), suite, LiveConfig{
		Profile: TranscriptProfile{ID: "fake", Client: "direct-mcp", Provider: "fake", Model: "fixture-model"},
		BaseURL: provider.URL,
	})
	if err != nil {
		t.Fatalf("run live evaluation: %v", err)
	}
	call := transcript.Cases[0].ToolCalls[0]
	if call.ErrorCode != "UPSTREAM_RATE_LIMITED" {
		t.Fatalf("public error code = %q, want UPSTREAM_RATE_LIMITED", call.ErrorCode)
	}
	if !strings.Contains(string(call.Result), "UPSTREAM_RATE_LIMITED") {
		t.Fatalf("recorded error result = %s, want public error envelope", call.Result)
	}
}

func TestExecuteModelToolNormalizesEmptyArguments(t *testing.T) {
	fixture, err := StartFixture(Case{Type: "success"})
	if err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	t.Cleanup(fixture.Close)

	mcpServer := server.NewMCPServer("test", "test", server.WithToolCapabilities(true))
	tools.RegisterAll(mcpServer, client.New(fixture.URL, "fixture-key", nil, nil))

	observed, content, runnerError := executeModelTool(t.Context(), mcpServer, observedModelCall(chatToolCall{
		Function: chatToolCallFunction{Name: "get_metadata", Arguments: ""},
	}))
	if runnerError != nil {
		t.Fatalf("execute no-argument tool: %#v", runnerError)
	}
	if observed.ErrorCode != "" {
		t.Fatalf("public error code = %q, want success", observed.ErrorCode)
	}
	if string(observed.Arguments) != `{}` {
		t.Fatalf("recorded arguments = %s, want {}", observed.Arguments)
	}
	if !json.Valid([]byte(content)) {
		t.Fatalf("tool content is not JSON: %q", content)
	}

	observed, _, runnerError = executeModelTool(t.Context(), mcpServer, observedModelCall(chatToolCall{
		Function: chatToolCallFunction{Name: "get_stop", Arguments: ""},
	}))
	if runnerError != nil {
		t.Fatalf("execute required-argument tool: %#v", runnerError)
	}
	if observed.ErrorCode != "INVALID_ARGUMENT" {
		t.Fatalf("public error code = %q, want INVALID_ARGUMENT", observed.ErrorCode)
	}
}

func TestRunOpenAICompatibleMarksToolCallLimitAsFailure(t *testing.T) {
	var requestNumber atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		currentRequest := requestNumber.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if currentRequest == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_stop","arguments":"{\"stop_id\":\"test_1013\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-2","type":"function","function":{"name":"search_stops","arguments":"{\"query\":\"extra\"}"}}]}}]}`))
	}))
	t.Cleanup(provider.Close)

	suite := Suite{Version: "limit-v1", Cases: []Case{{
		ID: "limit", Type: "success", Prompt: "test", MaxToolCalls: 1, ExpectedOutcome: "one call",
		ExpectedToolCalls: []ExpectedCall{{Name: "get_stop", Arguments: json.RawMessage(`{"stop_id":"test_1013"}`)}},
	}}}
	transcript, err := RunOpenAICompatible(t.Context(), suite, LiveConfig{
		Profile:      TranscriptProfile{ID: "fake", Client: "direct-mcp", Provider: "fake", Model: "fixture-model"},
		BaseURL:      provider.URL,
		MaxToolCalls: 1,
	})
	if err != nil {
		t.Fatalf("run live evaluation: %v", err)
	}
	result := transcript.Cases[0]
	if result.RunnerError == nil || result.RunnerError.Code != runnerErrorToolCallLimit {
		t.Fatalf("runner error = %#v, want %s", result.RunnerError, runnerErrorToolCallLimit)
	}
	if result.RunnerError.AttemptedCall == nil || result.RunnerError.AttemptedCall.Name != "search_stops" {
		t.Fatalf("attempted call = %#v, want search_stops", result.RunnerError.AttemptedCall)
	}
	if report := ScoreTranscript(suite, transcript); report.DeterministicPass {
		t.Fatalf("score report = %#v, want runner safety-limit failure", report)
	}
}

func TestRunOpenAICompatiblePreservesPartialTranscript(t *testing.T) {
	var requestNumber atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch requestNumber.Add(1) {
		case 1:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_stop","arguments":"{\"stop_id\":\"test_1013\"}"}}]}}]}`))
		case 2:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"First case complete."}}]}`))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(provider.Close)

	suite := Suite{Version: "partial-v1", Cases: []Case{
		{ID: "complete", Type: "success", Prompt: "first"},
		{ID: "failed", Type: "success", Prompt: "second"},
	}}
	transcript, err := RunOpenAICompatible(t.Context(), suite, LiveConfig{
		Profile: TranscriptProfile{ID: "fake", Client: "direct-mcp", Provider: "fake", Model: "fixture-model"},
		BaseURL: provider.URL,
	})
	if err == nil {
		t.Fatal("run live evaluation succeeded, want provider failure")
	}
	if len(transcript.Cases) != 2 {
		t.Fatalf("partial transcript cases = %d, want completed and failed cases", len(transcript.Cases))
	}
	if transcript.Cases[0].Response != "First case complete." {
		t.Fatalf("completed response = %q", transcript.Cases[0].Response)
	}
	if runnerError := transcript.Cases[1].RunnerError; runnerError == nil || runnerError.Code != runnerErrorModelRequest {
		t.Fatalf("failed case runner error = %#v, want %s", runnerError, runnerErrorModelRequest)
	}
}

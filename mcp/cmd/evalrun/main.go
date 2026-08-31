package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"oba-mcp/evals"
	"oba-mcp/tools"
)

func main() {
	suitePath := flag.String("suite", "evals/scenarios-v1.json", "path to an evaluation suite")
	outputPath := flag.String("out", "evals/transcripts/live.json", "path for the credential-free transcript")
	baseURL := flag.String("base-url", os.Getenv("EVAL_API_BASE_URL"), "OpenAI-compatible API base URL")
	model := flag.String("model", os.Getenv("EVAL_MODEL"), "model name")
	provider := flag.String("provider", envOrDefault("EVAL_PROVIDER", "openai-compatible"), "provider label recorded in the transcript")
	profileID := flag.String("profile-id", os.Getenv("EVAL_PROFILE_ID"), "model/client profile ID recorded in the transcript")
	toolProfile := flag.String("tool-profile", envOrDefault("EVAL_TOOL_PROFILE", string(tools.ToolProfileAll)), "MCP tool profile: all or rider")
	systemPromptPath := flag.String("system-prompt", "", "optional path to a client-specific system prompt")
	timeout := flag.Duration("timeout", 60*time.Second, "timeout for each model HTTP request")
	maxRounds := flag.Int("max-rounds", 8, "maximum model/tool rounds per case")
	maxToolCalls := flag.Int("max-tool-calls", 8, "hard tool-call safety limit per case")
	maxTokens := flag.Int("max-tokens", 2048, "maximum generated tokens per model response")
	flag.Parse()

	if strings.TrimSpace(*baseURL) == "" || strings.TrimSpace(*model) == "" || strings.TrimSpace(*profileID) == "" {
		fmt.Fprintln(os.Stderr, "-base-url, -model, and -profile-id are required")
		os.Exit(2)
	}
	parsedToolProfile, err := tools.ParseToolProfile(*toolProfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	systemPrompt, err := readOptionalFile(*systemPromptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read system prompt: %v\n", err)
		os.Exit(2)
	}
	suite, err := evals.Load(*suitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load suite: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	transcript, err := evals.RunOpenAICompatible(ctx, suite, evals.LiveConfig{
		Profile: evals.TranscriptProfile{
			ID:       *profileID,
			Client:   "direct-mcp",
			Provider: *provider,
			Model:    *model,
		},
		BaseURL:      *baseURL,
		APIKey:       os.Getenv("EVAL_API_KEY"),
		ToolProfile:  parsedToolProfile,
		SystemPrompt: systemPrompt,
		Timeout:      *timeout,
		MaxRounds:    *maxRounds,
		MaxToolCalls: *maxToolCalls,
		MaxTokens:    *maxTokens,
	})
	if err != nil && len(transcript.Cases) == 0 {
		fmt.Fprintf(os.Stderr, "run evaluations: %v\n", err)
		os.Exit(1)
	}
	if err := evals.WriteTranscript(*outputPath, transcript); err != nil {
		fmt.Fprintf(os.Stderr, "write transcript: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d cases to %s\n", len(transcript.Cases), *outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run evaluations: %v (partial transcript saved)\n", err)
		os.Exit(1)
	}
}

func readOptionalFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

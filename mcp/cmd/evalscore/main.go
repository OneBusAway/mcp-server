package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"oba-mcp/evals"
)

func main() {
	suitePath := flag.String("suite", "evals/scenarios-v1.json", "path to an evaluation suite")
	transcriptPath := flag.String("transcript", "", "path to a model/client transcript")
	flag.Parse()
	if *transcriptPath == "" {
		fmt.Fprintln(os.Stderr, "-transcript is required")
		os.Exit(2)
	}

	suite, err := evals.Load(*suitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load suite: %v\n", err)
		os.Exit(2)
	}
	transcript, err := evals.LoadTranscript(*transcriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load transcript: %v\n", err)
		os.Exit(2)
	}
	report := evals.ScoreTranscript(suite, transcript)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode score report: %v\n", err)
		os.Exit(2)
	}
	if !report.DeterministicPass {
		os.Exit(1)
	}
}

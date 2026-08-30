package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToResult(t *testing.T) {
	testCases := []struct {
		name    string
		result  toolResult
		text    string
		isError bool
	}{
		{
			name:   "text",
			result: textResult("No stops found."),
			text:   "No stops found.",
		},
		{
			name:   "data",
			result: dataResult("Stop:\n", StopResponse{ID: "unitrans_1", Name: "Memorial Union"}),
			text: "Stop:\n{\n  \"id\": \"unitrans_1\",\n  \"name\": \"Memorial Union\",\n" +
				"  \"code\": \"\",\n  \"lat\": 0,\n  \"lon\": 0\n}",
		},
		{
			name:    "error",
			result:  errorResult("UPSTREAM_TIMEOUT"),
			text:    "UPSTREAM_TIMEOUT",
			isError: true,
		},
		{
			name:   "data with suffix",
			result: dataResultWithSuffix("Route:\n", RouteResponse{ID: "unitrans_A"}, "\nUse get_stop_schedule for departures."),
			text: "Route:\n{\n  \"id\": \"unitrans_A\",\n  \"short_name\": \"\",\n" +
				"  \"agency_id\": \"\"\n}\nUse get_stop_schedule for departures.",
		},
		{
			name:    "marshal failure",
			result:  dataResult("Result:\n", func() {}),
			text:    "Unable to prepare the tool response.",
			isError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := toResult(testCase.result)
			if result.IsError != testCase.isError {
				t.Fatalf("IsError = %t, want %t", result.IsError, testCase.isError)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content count = %d, want 1", len(result.Content))
			}
			content, ok := result.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T, want mcp.TextContent", result.Content[0])
			}
			if content.Text != testCase.text {
				t.Fatalf("text = %q, want %q", content.Text, testCase.text)
			}
		})
	}
}

package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToResult(t *testing.T) {
	testCases := []struct {
		name         string
		result       toolResult
		text         string
		isError      bool
		errorCode    string
		expectedData any
		noResultData bool
	}{
		{
			name:         "text",
			result:       textResult("No stops found."),
			text:         "No stops found.",
			noResultData: true,
		},
		{
			name:         "data",
			result:       dataResult("Stop:\n", StopResponse{ID: "unitrans_1", Name: "Memorial Union"}),
			text:         "Stop:",
			expectedData: StopResponse{ID: "unitrans_1", Name: "Memorial Union"},
		},
		{
			name:      "error",
			result:    errorResult("UPSTREAM_TIMEOUT (retryable)"),
			text:      "The transit service timed out.",
			isError:   true,
			errorCode: "UPSTREAM_TIMEOUT",
		},
		{
			name:         "data with suffix",
			result:       dataResultWithSuffix("Route:\n", RouteResponse{ID: "unitrans_A"}, "\nUse get_stop_schedule for departures."),
			text:         "Route:\nUse get_stop_schedule for departures.",
			expectedData: RouteResponse{ID: "unitrans_A"},
		},
		{
			name:      "marshal failure",
			result:    dataResult("Result:\n", func() {}),
			text:      "Unable to prepare the tool response.",
			isError:   true,
			errorCode: "INTERNAL_ERROR",
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
			if testCase.isError {
				envelope, ok := result.StructuredContent.(ErrorEnvelope)
				if !ok {
					t.Fatalf("structured content type = %T, want ErrorEnvelope", result.StructuredContent)
				}
				if envelope.Code != testCase.errorCode {
					t.Fatalf("error code = %q, want %q", envelope.Code, testCase.errorCode)
				}
				return
			}

			envelope, ok := result.StructuredContent.(SuccessEnvelope[any])
			if !ok {
				t.Fatalf("structured content type = %T, want SuccessEnvelope[any]", result.StructuredContent)
			}
			if envelope.Meta.GeneratedAtMS <= 0 {
				t.Fatal("generated_at_ms was not set")
			}
			if testCase.noResultData {
				if envelope.Data != nil {
					t.Fatalf("data = %#v, want nil", envelope.Data)
				}
				return
			}
			if !reflect.DeepEqual(envelope.Data, testCase.expectedData) {
				t.Fatalf("data = %#v, want %#v", envelope.Data, testCase.expectedData)
			}
		})
	}
}

func TestPublicErrorDoesNotExposeInternalMessage(t *testing.T) {
	result := toResult(errorResult("parsing JSON: unexpected token at https://internal.example"))
	content := result.Content[0].(mcp.TextContent)
	if content.Text != "The transit service returned an unusable response." {
		t.Fatalf("text = %q", content.Text)
	}
	envelope := result.StructuredContent.(ErrorEnvelope)
	if envelope.Code != "UPSTREAM_BAD_RESPONSE" {
		t.Fatalf("code = %q", envelope.Code)
	}
}

func TestResultMetadataIncludesCacheAndTruncation(t *testing.T) {
	result := toResult(withCache(truncatedDataResult("Stops:\n", Page[StopResponse]{Items: []StopResponse{{ID: "unitrans_1"}}}), "hit"))
	envelope := result.StructuredContent.(SuccessEnvelope[any])
	if !envelope.Meta.Truncated {
		t.Fatal("truncated = false, want true")
	}
	if envelope.Meta.Cache != "hit" {
		t.Fatalf("cache = %q, want hit", envelope.Meta.Cache)
	}
}

func TestPublicErrorIncludesRetryAfter(t *testing.T) {
	result := toResult(errorResult("UPSTREAM_RATE_LIMITED (retryable; retry_after_ms=5000)"))
	envelope := result.StructuredContent.(ErrorEnvelope)
	if envelope.RetryAfterMS == nil || *envelope.RetryAfterMS != 5_000 {
		t.Fatalf("retry_after_ms = %v, want 5000", envelope.RetryAfterMS)
	}
}

func TestToResultRejectsOversizedStructuredContent(t *testing.T) {
	result := toResult(dataResult("Large response:\n", strings.Repeat("x", maxStructuredResultBytes)))
	if !result.IsError {
		t.Fatal("oversized result did not return an error")
	}
	envelope := result.StructuredContent.(ErrorEnvelope)
	if envelope.Code != "OUTPUT_TOO_LARGE" {
		t.Fatalf("code = %q, want OUTPUT_TOO_LARGE", envelope.Code)
	}
}

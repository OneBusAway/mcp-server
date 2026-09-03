package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrettyWriterFormatsSafeOperationalEvents(t *testing.T) {
	var output bytes.Buffer
	writer := NewPretty(&output)
	entries := []string{
		`{"event":"ready","transport":"streamable-http","endpoint":"http://127.0.0.1:8080/mcp"}`,
		`{"event":"upstream","request_id":"request-1","tool":"get_arrivals_for_stop","op":"stop","object_id":"test_1013","cache":"miss","params":"input=private","ms":12,"status":200,"bytes":512}`,
		`{"event":"tool","request_id":"request-1","tool":"get_arrivals_for_stop","outcome":"success","cache":"miss","ms":13,"bytes":256}`,
		`{"event":"draining","timeout_ms":5000}`,
	}
	for _, entry := range entries {
		if _, err := writer.Write([]byte(entry + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	rendered := output.String()
	for _, expected := range []string{"[READY]", "[MISS]", "stop (get_arrivals_for_stop)", "[DRAIN]", "request_id=request-1", "object_id=test_1013"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("pretty log omitted %q:\n%s", expected, rendered)
		}
	}
	if !strings.Contains(rendered, "private") || !strings.Contains(rendered, "params=") {
		t.Fatalf("pretty log omitted request parameters:\n%s", rendered)
	}
	if strings.Contains(rendered, "[TOOL]") {
		t.Fatalf("pretty log rendered a duplicate successful tool event:\n%s", rendered)
	}
}

func TestPrettyWriterKeepsToolErrorsWithoutAnUpstreamEvent(t *testing.T) {
	var output bytes.Buffer
	writer := NewPretty(&output)
	entry := `{"event":"tool","request_id":"request-2","tool":"get_trip_details","outcome":"tool_error","cache":"","ms":1}`
	if _, err := writer.Write([]byte(entry + "\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "[TOOL]") || !strings.Contains(output.String(), "get_trip_details") {
		t.Fatalf("pretty log omitted tool error:\n%s", output.String())
	}
}

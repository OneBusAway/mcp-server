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
		`{"event":"upstream","request_id":"request-1","op":"stop","cache":"miss","params":"input=private","ms":12,"status":200,"bytes":512}`,
		`{"event":"tool","request_id":"request-1","tool":"get_stop","outcome":"success","cache":"miss","ms":13,"bytes":256}`,
		`{"event":"draining","timeout_ms":5000}`,
	}
	for _, entry := range entries {
		if _, err := writer.Write([]byte(entry + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	rendered := output.String()
	for _, expected := range []string{"[READY]", "[MISS]", "[TOOL]", "[DRAIN]", "request_id=request-1"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("pretty log omitted %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "private") || strings.Contains(rendered, "params=") {
		t.Fatalf("pretty log exposed request parameters:\n%s", rendered)
	}
}

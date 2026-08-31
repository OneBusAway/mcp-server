// Package logger provides a human-readable writer for the structured JSON logs
// emitted by client/http_client.go. Wrap any io.Writer with NewPretty; when OBA_LOG_JSON=1
// is set, bypass it entirely and write raw JSON.
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// PrettyWriter wraps an io.Writer and reformats JSON log lines to readable text.
// Non-JSON lines are passed through unchanged.
type PrettyWriter struct {
	w io.Writer
}

func NewPretty(w io.Writer) *PrettyWriter { return &PrettyWriter{w: w} }

func (pw *PrettyWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		fmt.Fprintln(pw.w, line)
		return len(p), nil
	}

	ts := time.Now().Format("15:04:05")
	event, _ := m["event"].(string)

	switch event {
	case "req":
		op, _ := m["op"].(string)
		cache, _ := m["cache"].(string)
		ms, _ := m["ms"].(float64)
		bytes, _ := m["bytes"].(float64)
		params, _ := m["params"].(string)
		errMsg, _ := m["error"].(string)
		errCode, _ := m["error_code"].(string)
		circuit, _ := m["circuit"].(string)

		switch {
		case errMsg != "":
			fmt.Fprintf(pw.w, "%s [ERR]  %-42s %s%s\n", ts, op, errMsg, fmtParams(params))
		case errCode != "":
			fmt.Fprintf(pw.w, "%s [ERR]  %-42s %s%s\n", ts, op, errCode, fmtParams(params))
		case circuit == "open":
			retry, _ := m["retry_in_sec"].(float64)
			fmt.Fprintf(pw.w, "%s [SKIP] %-42s circuit open (retry in %.0fs)\n", ts, op, retry)
		case cache == "hit":
			fmt.Fprintf(pw.w, "%s [HIT]  %s%s\n", ts, op, fmtParams(params))
		case cache == "l2-hit":
			fmt.Fprintf(pw.w, "%s [L2]   %s%s\n", ts, op, fmtParams(params))
		default:
			extra := ""
			if ms > 0 {
				extra = fmt.Sprintf("ms=%.0f", ms)
			}
			if bytes > 0 {
				if extra != "" {
					extra += "  "
				}
				extra += fmtBytes(bytes)
			}
			fmt.Fprintf(pw.w, "%s [MISS] %-42s %s%s\n", ts, op, extra, fmtParams(params))
		}

	case "circuit":
		state, _ := m["state"].(string)
		if state == "open" {
			f, _ := m["failures"].(float64)
			fmt.Fprintf(pw.w, "%s [OPEN]  circuit breaker  failures=%.0f\n", ts, f)
		} else {
			a, _ := m["after_failures"].(float64)
			fmt.Fprintf(pw.w, "%s [CLOSE] circuit breaker  after %.0f failures\n", ts, a)
		}

	case "start":
		target, _ := m["target"].(string)
		fmt.Fprintf(pw.w, "%s [START] %s\n", ts, target)

	case "stop":
		fmt.Fprintf(pw.w, "%s [STOP]\n", ts)

	case "cache":
		if path, ok := m["path"].(string); ok {
			fmt.Fprintf(pw.w, "%s [CACHE] %s\n", ts, path)
		} else if e, ok := m["error"].(string); ok {
			fmt.Fprintf(pw.w, "%s [CACHE ERR] %s\n", ts, e)
		}

	case "http":
		addr, _ := m["addr"].(string)
		fmt.Fprintf(pw.w, "%s [HTTP] addr=%s\n", ts, addr)

	default:
		fmt.Fprintln(pw.w, line)
	}

	return len(p), nil
}

func fmtParams(params string) string {
	if params == "" {
		return ""
	}
	return "  params=" + params
}

func fmtBytes(b float64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", b/(1<<10))
	default:
		return fmt.Sprintf("%.0fB", b)
	}
}

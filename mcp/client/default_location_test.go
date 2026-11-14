package client

import (
	"context"
	"strings"
	"testing"
	"time"
)

// An unresolvable agency used to format in UTC; it now follows the process zone (TZ).
func TestUnresolvedZoneFallsBackToLocal(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("no tzdata")
	}
	saved := time.Local
	time.Local = la
	defer func() { time.Local = saved }()

	if got := (&OBAClient{}).TimezoneFor(context.Background(), ""); got != la {
		t.Fatalf("TimezoneFor(\"\") = %v, want %v", got, la)
	}
	ms := float64(time.Date(2030, 1, 2, 20, 30, 0, 0, time.UTC).UnixMilli()) // 12:30 PM in Los Angeles
	if got := FormatRelativeTime(ms, nil); !strings.HasPrefix(got, "12:30 PM") {
		t.Fatalf("FormatRelativeTime(nil) = %q, want 12:30 PM local", got)
	}
}

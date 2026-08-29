package validation

import (
	"math"
	"testing"
)

func TestEntityIDRejectsPathCharacters(t *testing.T) {
	for _, value := range []string{"agency_stop", "agency:route.1", ".", "..", "../secret", "id/other", "id?key=x", "id%2Fother", "line\nfeed"} {
		_, err := EntityID("stop_id", value)
		valid := value == "agency_stop" || value == "agency:route.1"
		if (err == nil) != valid {
			t.Fatalf("EntityID(%q) error = %v", value, err)
		}
	}
}

func TestCoordinatesAndBounds(t *testing.T) {
	if _, err := Latitude(90); err != nil {
		t.Fatal(err)
	}
	if _, err := Longitude(-180); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{-91, math.Inf(1), math.NaN()} {
		if _, err := Latitude(value); err == nil {
			t.Fatalf("Latitude(%v) accepted", value)
		}
	}
	if _, err := RadiusMeters(50_001, 500, MaxRadiusMeters); err == nil {
		t.Fatal("oversized radius accepted")
	}
	if _, err := WindowMinutes("minutes_after", 1.5, 30, 120); err == nil {
		t.Fatal("fractional window accepted")
	}
}

func TestNumberAndTimestampBoundaries(t *testing.T) {
	if _, err := WholeNumber("max_count", 5, 5, 1, 20); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{-1, 20.5, 21, math.Inf(-1)} {
		if _, err := WholeNumber("max_count", value, 5, 1, 20); err == nil {
			t.Fatalf("WholeNumber(%v) accepted", value)
		}
	}
	if _, err := Span("lat_span", 0); err == nil {
		t.Fatal("zero span accepted")
	}
	if _, err := TimestampMillis("time", float64(MaxTimestampMillis)+1); err == nil {
		t.Fatal("future timestamp accepted")
	}
	if got, err := TimestampMillis("time", 1_700_000_000_000); err != nil || got != 1_700_000_000_000 {
		t.Fatalf("TimestampMillis() = %d, %v", got, err)
	}
}

func TestSearchAndDate(t *testing.T) {
	if got, err := Search("  Main Street "); err != nil || got != "Main Street" {
		t.Fatalf("Search() = %q, %v", got, err)
	}
	if _, err := Search("   "); err == nil {
		t.Fatal("blank search accepted")
	}
	if _, err := Date("2026-02-29"); err == nil {
		t.Fatal("invalid date accepted")
	}
	if _, err := Date("2026-02-28"); err != nil {
		t.Fatal(err)
	}
}

package tools

import (
	"testing"
	"time"
)

func TestGetCurrentTimeContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/current-time.json": envelopeEntry(`{"time":1770000000000,"readableTime":"2026-01-01T00:00:00Z"}`),
	})

	result := invokeHandler(t, handler.getCurrentTime, map[string]any{})
	current := dataAs[CurrentTimeResponse](t, result)
	if current.TimeMS != 1_770_000_000_000 {
		t.Fatalf("current time_ms = %d, want preserved epoch ms", current.TimeMS)
	}
	if current.TimeDisplay != "2026-01-01T00:00:00Z" {
		t.Fatalf("current time_display = %q, want upstream readableTime passthrough", current.TimeDisplay)
	}
}

func TestGetCurrentTimeSynthesizesDisplayWhenAbsent(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/current-time.json": envelopeEntry(`{"time":1770000000000}`),
	})

	result := invokeHandler(t, handler.getCurrentTime, map[string]any{})
	current := dataAs[CurrentTimeResponse](t, result)
	if current.TimeMS != 1_770_000_000_000 {
		t.Fatalf("current time_ms = %d, want preserved epoch ms", current.TimeMS)
	}
	if current.TimeDisplay == "" {
		t.Fatalf("current time_display is empty; handler must synthesize an RFC 3339 display when upstream omits readableTime")
	}
}

func TestGetMetadataContract(t *testing.T) {
	// Use recent-ish timestamps so freshness classification is deterministic:
	// updatedAt = now - 30s → "fresh"; updatedAt = now - 10min → "stale".
	now := time.Now().UTC()
	fresh := now.Add(-30 * time.Second).Format(time.RFC3339)
	stale := now.Add(-10 * time.Minute).Format(time.RFC3339)
	staticUpdated := now.Add(-1 * time.Hour).Format(time.RFC3339)

	handler, _ := fixtureHandler(t, map[string]string{
		"/api/v2/metadata.json": `{
			"staticGtfsLastUpdated":"` + staticUpdated + `",
			"realtimeFeeds":{
				"trip_updates":"` + fresh + `",
				"vehicle_positions":"` + stale + `"
			}
		}`,
	})

	result := invokeHandler(t, handler.getMetadata, map[string]any{})
	metadata := dataAs[MetadataResponse](t, result)
	if metadata.StaticGTFSLastUpdatedMS == 0 {
		t.Fatalf("static GTFS timestamp missing")
	}
	if len(metadata.RealtimeFeeds) != 2 {
		t.Fatalf("realtime feeds = %d, want 2", len(metadata.RealtimeFeeds))
	}
	if got := metadata.RealtimeFeeds["trip_updates"].Status; got != "fresh" {
		t.Fatalf("trip_updates status = %q, want fresh (age ~30s)", got)
	}
	if got := metadata.RealtimeFeeds["vehicle_positions"].Status; got != "stale" {
		t.Fatalf("vehicle_positions status = %q, want stale (age ~10m)", got)
	}
	if metadata.RealtimeFeeds["trip_updates"].UpdatedAtMS == 0 {
		t.Fatalf("realtime feed updated_at_ms missing; the freshness contract requires machine-readable timestamps alongside the display status")
	}
}

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"oba-mcp/client"
	"oba-mcp/internal/obafixture"
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

func TestGetMetadataReportsUnsupportedWhenServerLacksEndpoint(t *testing.T) {
	// The Java OneBusAway server has no /api/v2/metadata.json; Tomcat answers with an HTML 404.
	upstream := obafixture.New(map[string]obafixture.Response{
		"/api/v2/metadata.json": {
			Status: http.StatusNotFound,
			Header: http.Header{"Content-Type": {"text/html;charset=utf-8"}},
			Body:   "<!doctype html><title>HTTP Status 404 – Not Found</title>",
		},
	})
	t.Cleanup(upstream.Close)
	handler := &Handler{client: client.New(upstream.URL, "fixture-api-key", nil, nil)}

	result, err := handler.getMetadata(context.Background(), toolRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want a tool error")
	}
	envelope := result.StructuredContent.(ErrorEnvelope)
	if envelope.Code != "UPSTREAM_UNSUPPORTED" {
		t.Fatalf("code = %q, want UPSTREAM_UNSUPPORTED", envelope.Code)
	}
	if envelope.Retryable {
		t.Fatal("retryable = true, want false: the endpoint will not appear on retry")
	}
}

const javaServerConfigEntry = `{"id":"d08be155-5150-4fc8-8650-e0410e7fd175","name":"2026091711","serviceDateFrom":"1789628400000","serviceDateTo":"1801296000000","gitProperties":{"git.build.version":"2.0.0-SNAPSHOT","git.commit.id":"4470690f8ac2b983d758d57e4c66274a861dc014","git.remote.origin.url":"https://github.com/OneBusAway/onebusaway-application-modules.git","git.build.user.email":"dev@example.com","git.build.host":"build-box"}}`

func TestGetServerConfigContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/config.json": envelopeEntry(javaServerConfigEntry),
	})

	result := invokeHandler(t, handler.getServerConfig, map[string]any{})
	config := dataAs[ServerConfigResponse](t, result)
	want := ServerConfigResponse{
		ServerType:        "java",
		BundleID:          "d08be155-5150-4fc8-8650-e0410e7fd175",
		BundleName:        "2026091711",
		ServiceDateFromMS: 1_789_628_400_000,
		ServiceDateToMS:   1_801_296_000_000,
		Version:           "2.0.0-SNAPSHOT",
		CommitID:          "4470690f8ac2b983d758d57e4c66274a861dc014",
	}
	if config != want {
		t.Fatalf("config = %+v, want %+v", config, want)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "dev@example.com") || strings.Contains(string(encoded), "build-box") {
		t.Fatalf("structured content exposes build-machine details: %s", encoded)
	}
}

func TestGetServerConfigIdentifiesMaglev(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/config.json": envelopeEntry(`{"id":"oba-maglev","name":"OneBusAway Go","serviceDateFrom":"","serviceDateTo":"","gitProperties":{"git.build.version":"v1.4.0","git.commit.id":"0123456789abcdef","git.remote.origin.url":"https://github.com/OneBusAway/maglev.git"}}`),
	})

	result := invokeHandler(t, handler.getServerConfig, map[string]any{})
	config := dataAs[ServerConfigResponse](t, result)
	want := ServerConfigResponse{ServerType: "maglev", Version: "v1.4.0", CommitID: "0123456789abcdef"}
	if config != want {
		t.Fatalf("config = %+v, want %+v", config, want)
	}
}

func TestGetServerConfigReportsUnknownServer(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/config.json": envelopeEntry(`{"id":"bundle-7","name":"Spring 2026","gitProperties":{}}`),
	})

	result := invokeHandler(t, handler.getServerConfig, map[string]any{})
	config := dataAs[ServerConfigResponse](t, result)
	want := ServerConfigResponse{ServerType: "unknown", BundleID: "bundle-7", BundleName: "Spring 2026"}
	if config != want {
		t.Fatalf("config = %+v, want %+v", config, want)
	}
}

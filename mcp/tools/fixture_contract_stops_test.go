package tools

import "testing"

func TestGetStopContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/stop/test_1013.json": envelopeEntry(`{"id":"test_1013","name":"Fixture Stop","code":"1013","direction":"North","lat":27.9488,"lon":-82.4582,"routeIds":["test_10","test_11"]}`),
	})

	result := invokeHandler(t, handler.getStop, map[string]any{"stop_id": "test_1013"})
	stop := dataAs[StopResponse](t, result)
	if stop.ID != "test_1013" || stop.Name != "Fixture Stop" || stop.Code != "1013" {
		t.Fatalf("stop identity mapping wrong: %#v", stop)
	}
	if stop.Direction != "North" || stop.Lat != 27.9488 || stop.Lon != -82.4582 {
		t.Fatalf("stop location mapping wrong: %#v", stop)
	}
	if len(stop.RouteIDs) != 2 || stop.RouteIDs[0] != "test_10" || stop.RouteIDs[1] != "test_11" {
		t.Fatalf("stop routeIds mapping wrong: %#v", stop.RouteIDs)
	}
	assertRequestPath(t, upstream, "/api/where/stop/test_1013.json", nil)
}

// TestSearchStopsEmptyReturnsStructuredPage pins the empty-list contract for
// search_stops so a zero-hit query yields a typed empty Page[StopResponse].
func TestSearchStopsEmptyReturnsStructuredPage(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/search/stop.json": envelopeList(`[]`),
	})
	result := invokeHandler(t, handler.searchStops, map[string]any{"query": "nothing-matches"})
	page := dataAs[Page[StopResponse]](t, result)
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty search_stops page.Items = %#v, want empty non-nil slice", page.Items)
	}
}

func TestSearchStopsContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/search/stop.json": envelopeList(`[
			{"id":"test_1013","name":"Fixture Stop","code":"1013","lat":27.9488,"lon":-82.4582,"routeIds":["test_10"]},
			{"id":"test_1014","name":"Another Stop","code":"1014","lat":27.95,"lon":-82.46,"routeIds":[]}
		]`),
	})

	result := invokeHandler(t, handler.searchStops, map[string]any{"query": "Fixture"})
	page := dataAs[Page[StopResponse]](t, result)
	if len(page.Items) != 2 {
		t.Fatalf("search stops = %d, want 2", len(page.Items))
	}
	if page.Items[0].ID != "test_1013" || page.Items[1].ID != "test_1014" {
		t.Fatalf("search stops ordering/mapping wrong: %#v", page.Items)
	}
	assertRequestPath(t, upstream, "/api/where/search/stop.json", map[string]string{"input": "Fixture"})
}

func TestFindStopsNearLocationContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/stops-for-location.json": envelopeList(`[
			{"id":"test_1013","name":"Nearby Stop","code":"1013","lat":27.9488,"lon":-82.4582,"routeIds":["test_10"]}
		]`),
	})

	result := invokeHandler(t, handler.findStopsNearLocation, map[string]any{
		"lat":    27.9488,
		"lon":    -82.4582,
		"radius": 750.0,
	})
	page := dataAs[Page[StopResponse]](t, result)
	if len(page.Items) != 1 || page.Items[0].ID != "test_1013" {
		t.Fatalf("nearby stops mapping wrong: %#v", page.Items)
	}
	assertRequestPath(t, upstream, "/api/where/stops-for-location.json", map[string]string{
		"lat":    "27.948800",
		"lon":    "-82.458200",
		"radius": "750",
	})
}

// TestFindStopsNearLocationEmptyReturnsStructuredPage pins the empty-list
// contract: even with zero nearby stops, the tool must return a typed
// Page[StopResponse] with an empty Items slice — not a nil Data with only a
// text summary. This keeps clients on the structured code path regardless of
// upstream size, so agents never have to scrape the human-readable summary.
func TestFindStopsNearLocationEmptyReturnsStructuredPage(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/stops-for-location.json": envelopeList(`[]`),
	})

	result := invokeHandler(t, handler.findStopsNearLocation, map[string]any{"lat": 0.0, "lon": 0.0})
	page := dataAs[Page[StopResponse]](t, result)
	if page.Items == nil {
		t.Fatalf("empty-nearby page.Items = nil, want empty non-nil slice so JSON marshals as []")
	}
	if len(page.Items) != 0 {
		t.Fatalf("empty-nearby page.Items = %d, want 0", len(page.Items))
	}
	if page.NextCursor != "" {
		t.Fatalf("empty-nearby next_cursor = %q, want empty (nothing to page past)", page.NextCursor)
	}
}

func TestGetStopsForAgencyContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/stops-for-agency/test.json": envelopeList(`[
			{"id":"test_1013","name":"Agency Stop","code":"1013","lat":27.9,"lon":-82.4,"routeIds":[]}
		]`),
	})

	result := invokeHandler(t, handler.getStopsForAgency, map[string]any{"agency_id": "test"})
	page := dataAs[Page[StopResponse]](t, result)
	if len(page.Items) != 1 || page.Items[0].Name != "Agency Stop" {
		t.Fatalf("agency stops mapping wrong: %#v", page.Items)
	}
}

func TestGetStopScheduleContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/schedule-for-stop/test_1013.json": envelopeEntry(`{
			"stopId":"test_1013",
			"date":1770000000000,
			"stopRouteSchedules":[{
				"routeId":"test_10",
				"stopRouteDirectionSchedules":[{
					"tripHeadsign":"Downtown",
					"scheduleStopTimes":[
						{"tripId":"test_trip1","departureTime":1770000300000},
						{"tripId":"test_trip2","departureTime":0}
					]
				}]
			}]
		}`),
	})

	result := invokeHandler(t, handler.getStopSchedule, map[string]any{"stop_id": "test_1013"})
	schedule := dataAs[StopScheduleResponse](t, result)
	if schedule.StopID != "test_1013" || schedule.Timezone != "America/Los_Angeles" {
		t.Fatalf("schedule identity/timezone mapping wrong: %#v", schedule)
	}
	if schedule.DateMS != 1_770_000_000_000 {
		t.Fatalf("schedule date_ms = %d, want preserved epoch ms", schedule.DateMS)
	}
	if len(schedule.Routes) != 1 || schedule.Routes[0].RouteID != "test_10" {
		t.Fatalf("schedule routes mapping wrong: %#v", schedule.Routes)
	}
	if len(schedule.Routes[0].Directions) != 1 || schedule.Routes[0].Directions[0].Headsign != "Downtown" {
		t.Fatalf("schedule direction mapping wrong: %#v", schedule.Routes[0].Directions)
	}
	trips := schedule.Routes[0].Directions[0].Trips
	if len(trips) != 1 {
		t.Fatalf("schedule trips = %d, want 1 non-zero departure filtered", len(trips))
	}
	if trips[0].TripID != "test_trip1" || trips[0].DepartureMS != 1_770_000_300_000 {
		t.Fatalf("scheduled trip fields wrong: %#v", trips[0])
	}
	assertRequestPath(t, upstream, "/api/where/schedule-for-stop/test_1013.json", nil)
}

func TestGetStopIDsForAgencyContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/stop-ids-for-agency/test.json": envelopeList(`["test_1013","test_1014","test_1015"]`),
	})

	result := invokeHandler(t, handler.getStopIDsForAgency, map[string]any{"agency_id": "test"})
	page := dataAs[Page[string]](t, result)
	if len(page.Items) != 3 || page.Items[0] != "test_1013" || page.Items[2] != "test_1015" {
		t.Fatalf("stop IDs mapping wrong: %#v", page.Items)
	}
}

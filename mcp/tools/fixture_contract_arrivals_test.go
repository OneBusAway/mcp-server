package tools

import "testing"

func TestGetArrivalAndDepartureForStopContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/arrival-and-departure-for-stop/test_1013.json": envelopeEntry(`{
			"stopId":"test_1013",
			"tripId":"test_trip",
			"routeId":"test_10",
			"routeShortName":"10",
			"tripHeadsign":"Downtown",
			"predicted":true,
			"scheduledArrivalTime":1770000300000,
			"predictedArrivalTime":1770000360000,
			"scheduleDeviation":45,
			"tripStatus":{"activeTripId":"test_active_trip","orientation":90,"position":{"lat":27.9488,"lon":-82.4582},"activeTrip":{"id":"test_active_trip","routeId":"test_20","shapeId":"active_shape","tripHeadsign":"Current Route"}}
		}`),
	})

	result := invokeHandler(t, handler.getArrivalAndDepartureForStop, map[string]any{
		"stop_id":      "test_1013",
		"trip_id":      "test_trip",
		"service_date": float64(1_770_000_000_000),
		"vehicle_id":   "test_bus",
	})
	arrival := dataAs[ArrivalResponse](t, result)
	if arrival.TripID != "test_trip" || arrival.RouteID != "test_10" || arrival.RouteName != "10" {
		t.Fatalf("arrival identity mapping wrong: %#v", arrival)
	}
	if !arrival.Predicted || arrival.PredictedArrivalMS != 1_770_000_360_000 {
		t.Fatalf("predicted-arrival preservation wrong: %#v", arrival)
	}
	if arrival.ScheduledArrivalMS != 1_770_000_300_000 || arrival.Timezone != "America/Los_Angeles" {
		t.Fatalf("scheduled-arrival timezone/preservation wrong: %#v", arrival)
	}
	if arrival.ActiveTripID != "test_active_trip" || arrival.ActiveRouteID != "test_20" || arrival.ActiveShapeID != "active_shape" {
		t.Fatalf("active trip mapping wrong: %#v", arrival)
	}
	assertRequestPath(t, upstream, "/api/where/arrival-and-departure-for-stop/test_1013.json", map[string]string{
		"tripId":      "test_trip",
		"serviceDate": "1770000000000",
		"vehicleId":   "test_bus",
	})
}

func TestGetArrivalsForLocationContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/arrivals-and-departures-for-location.json": envelopeEntry(`{
			"arrivalsAndDepartures":[{
				"tripId":"test_trip",
				"routeId":"test_10",
				"routeShortName":"10",
				"tripHeadsign":"Downtown",
				"predicted":true,
				"scheduledArrivalTime":1770000300000,
				"predictedArrivalTime":1770000360000
			}]
		}`),
	})

	result := invokeHandler(t, handler.getArrivalsForLocation, map[string]any{
		"lat":    27.9488,
		"lon":    -82.4582,
		"radius": 800.0,
	})
	page := dataAs[Page[ArrivalResponse]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("nearby arrivals = %#v, want one arrival", page.Items)
	}
	arrival := page.Items[0]
	if arrival.TripID != "test_trip" || arrival.RouteName != "10" || !arrival.Predicted {
		t.Fatalf("nearby arrival mapping wrong: %#v", arrival)
	}
	if arrival.PredictedArrivalMS != 1_770_000_360_000 || arrival.Timezone != "America/Los_Angeles" {
		t.Fatalf("nearby arrival timestamp/timezone wrong: %#v", arrival)
	}
	assertRequestPath(t, upstream, "/api/where/arrivals-and-departures-for-location.json", map[string]string{
		"lat":    "27.948800",
		"lon":    "-82.458200",
		"radius": "800",
	})
}

// TestGetArrivalsForStopEmptyReturnsStructuredPage pins the empty-list
// contract: a stop with no upcoming arrivals must return a typed empty
// Page[ArrivalResponse], not text-only nil data.
func TestGetArrivalsForStopEmptyReturnsStructuredPage(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json":                                tzAwareAgency,
		"/api/where/arrivals-and-departures-for-stop/test_1013.json": envelopeEntry(`{"stopId":"test_1013","arrivalsAndDepartures":[]}`),
	})
	result := invokeHandler(t, handler.getArrivalsForStop, map[string]any{"stop_id": "test_1013"})
	page := dataAs[Page[ArrivalResponse]](t, result)
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty arrivals page.Items = %#v, want empty non-nil slice", page.Items)
	}
}

// TestGetArrivalsForStopOmitsPredictedFieldsWhenNotPredicted pins the
// invariant that a non-predicted arrival must not surface predicted_*
// timestamps to the client — the DTO→MCP transform is expected to gate on
// the Predicted flag rather than blindly copying whatever OBA returned.
func TestGetArrivalsForStopOmitsPredictedFieldsWhenNotPredicted(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/arrivals-and-departures-for-stop/test_1013.json": envelopeEntry(`{
			"stopId":"test_1013",
			"arrivalsAndDepartures":[{
				"tripId":"test_trip","routeId":"test_10","routeShortName":"10",
				"predicted":false,
				"scheduledArrivalTime":1770000300000,
				"predictedArrivalTime":1770099999999
			}]
		}`),
	})

	result := invokeHandler(t, handler.getArrivalsForStop, map[string]any{"stop_id": "test_1013"})
	page := dataAs[Page[ArrivalResponse]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("arrivals = %#v, want one arrival", page.Items)
	}
	arrival := page.Items[0]
	if arrival.Predicted {
		t.Fatalf("arrival predicted = true, want false (fixture said false)")
	}
	if arrival.PredictedArrivalMS != 0 {
		t.Fatalf("predicted_arrival_ms = %d, want 0 when predicted=false (leaked stale prediction to client)", arrival.PredictedArrivalMS)
	}
	if arrival.ScheduledArrivalMS != 1_770_000_300_000 {
		t.Fatalf("scheduled_arrival_ms = %d, want fixture value even when not predicted", arrival.ScheduledArrivalMS)
	}
}

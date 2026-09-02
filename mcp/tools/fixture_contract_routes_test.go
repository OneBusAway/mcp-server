package tools

import "testing"

func TestGetRouteContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/route/test_10.json": envelopeEntry(`{"id":"test_10","shortName":"10","longName":"Downtown Express","agencyId":"test","color":"FF0000"}`),
	})

	result := invokeHandler(t, handler.getRoute, map[string]any{"route_id": "test_10"})
	route := dataAs[RouteResponse](t, result)
	if route.ID != "test_10" || route.ShortName != "10" || route.LongName != "Downtown Express" {
		t.Fatalf("route identity mapping wrong: %#v", route)
	}
	if route.AgencyID != "test" || route.Color != "FF0000" {
		t.Fatalf("route agency/color mapping wrong: %#v", route)
	}
}

func TestSearchRoutesContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/search/route.json": envelopeList(`[
			{"id":"test_10","shortName":"10","longName":"Downtown","agencyId":"test"},
			{"id":"test_11","shortName":"11","longName":"Uptown","agencyId":"test"}
		]`),
	})

	result := invokeHandler(t, handler.searchRoutes, map[string]any{"query": "1"})
	page := dataAs[Page[RouteResponse]](t, result)
	if len(page.Items) != 2 {
		t.Fatalf("search routes = %d, want 2", len(page.Items))
	}
	assertRequestPath(t, upstream, "/api/where/search/route.json", map[string]string{"input": "1"})
}

func TestGetRoutesForAgencyContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/routes-for-agency/test.json": envelopeList(`[
			{"id":"test_10","shortName":"10","longName":"Downtown","agencyId":"test"}
		]`),
	})

	result := invokeHandler(t, handler.getRoutesForAgency, map[string]any{"agency_id": "test"})
	page := dataAs[Page[RouteResponse]](t, result)
	if len(page.Items) != 1 || page.Items[0].ID != "test_10" {
		t.Fatalf("routes-for-agency mapping wrong: %#v", page.Items)
	}
}

func TestGetRoutesForLocationContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/routes-for-location.json": envelopeList(`[
			{"id":"test_10","shortName":"10","longName":"Downtown","agencyId":"test"}
		]`),
	})

	result := invokeHandler(t, handler.getRoutesForLocation, map[string]any{"lat": 27.9488, "lon": -82.4582})
	page := dataAs[Page[RouteResponse]](t, result)
	if len(page.Items) != 1 || page.Items[0].ID != "test_10" {
		t.Fatalf("routes-for-location mapping wrong: %#v", page.Items)
	}
	assertRequestPath(t, upstream, "/api/where/routes-for-location.json", map[string]string{
		"lat":    "27.948800",
		"lon":    "-82.458200",
		"radius": "500",
	})
}

func TestGetStopsForRouteContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/stops-for-route/test_10.json": `{
			"code":200,
			"data":{
				"entry":{
					"routeId":"test_10",
					"stopIds":["test_1013"],
					"stopGroupings":[{
						"type":"direction",
						"stopGroups":[{
							"id":"0",
							"name":{"type":"destination","name":"Downtown"},
							"stopIds":["test_1013"]
						}]
					}],
					"polylines":[{"points":"abc","length":250}]
				},
				"references":{"stops":[{"id":"test_1013","name":"Fixture Stop","lat":27.9,"lon":-82.4}]}
			}
		}`,
	})

	result := invokeHandler(t, handler.getStopsForRoute, map[string]any{"route_id": "test_10"})
	response := dataAs[RouteStopsResponse](t, result)
	if len(response.Directions) != 1 || response.Directions[0].Direction != "Downtown" {
		t.Fatalf("route direction mapping wrong: %#v", response.Directions)
	}
	stops := response.Directions[0].Stops
	if len(stops) != 1 || stops[0].ID != "test_1013" || stops[0].Name != "Fixture Stop" {
		t.Fatalf("direction stops references not resolved: %#v", stops)
	}
	if len(response.Polylines) != 1 || response.Polylines[0].Points != "abc" || response.Polylines[0].Length != 250 {
		t.Fatalf("route polylines mapping wrong: %#v", response.Polylines)
	}
}

func TestGetTripsForRouteContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/trips-for-route/test_10.json": `{
			"code":200,
			"data":{
				"list":[{
					"tripId":"test_trip",
					"trip":{"id":"test_trip","routeId":"test_10","tripHeadsign":"Downtown"},
					"status":{"activeTripId":"test_trip","phase":"in_progress","vehicleId":"test_bus","position":{"lat":27.9,"lon":-82.4},"activeTrip":{"id":"test_trip","routeId":"test_10","shapeId":"shape_1","tripHeadsign":"Downtown"}}
				},{
					"tripId":"future_interlined_trip",
					"trip":{"id":"future_interlined_trip","routeId":"test_10","tripHeadsign":"Later"},
					"status":{"activeTripId":"current_other_route","phase":"in_progress","vehicleId":"test_bus_2","position":{"lat":28.0,"lon":-82.5},"activeTrip":{"id":"current_other_route","routeId":"test_20","tripHeadsign":"Current Route"}}
				}],
				"references":{"stops":[],"agencies":[]}
			}
		}`,
	})

	result := invokeHandler(t, handler.getTripsForRoute, map[string]any{"route_id": "test_10"})
	page := dataAs[Page[RouteTripResponse]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("trips-for-route = %d, want 1", len(page.Items))
	}
	trip := page.Items[0]
	if trip.TripID != "test_trip" || trip.Headsign != "Downtown" || trip.Phase != "in_progress" {
		t.Fatalf("trips-for-route mapping wrong: %#v", trip)
	}
	if trip.VehicleID != "test_bus" || trip.Lat != 27.9 || trip.Lon != -82.4 {
		t.Fatalf("trips-for-route vehicle mapping wrong: %#v", trip)
	}
	if trip.ActiveTripID != "test_trip" || trip.ActiveRouteID != "test_10" || trip.ActiveShapeID != "shape_1" {
		t.Fatalf("trips-for-route active identity wrong: %#v", trip)
	}
	// include_schedule and include_status default to true and must appear in
	// the outgoing query so the model can distinguish schedule-only vs
	// live-status responses.
	assertRequestPath(t, upstream, "/api/where/trips-for-route/test_10.json", map[string]string{
		"includeSchedule": "true",
		"includeStatus":   "true",
	})
}

func TestGetScheduleForRouteContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/schedule-for-route/test_10.json": envelopeEntry(`{
			"routeId":"test_10",
			"scheduleDate":1770000000000,
			"trips":[],
			"stopTripGroupings":[{
				"directionId":"0",
				"tripHeadsigns":["Downtown"],
				"tripIds":["test_trip1","test_trip2"],
				"stopIds":["test_1013","test_1014"]
			}]
		}`),
	})

	result := invokeHandler(t, handler.getScheduleForRoute, map[string]any{"route_id": "test_10"})
	schedule := dataAs[RouteScheduleStructureResponse](t, result)
	if schedule.RouteID != "test_10" || schedule.Timezone != "America/Los_Angeles" {
		t.Fatalf("route schedule identity/timezone wrong: %#v", schedule)
	}
	if schedule.DateMS != 1_770_000_000_000 {
		t.Fatalf("route schedule date_ms = %d, want preserved epoch ms", schedule.DateMS)
	}
	if len(schedule.Directions) != 1 {
		t.Fatalf("route schedule directions = %d, want 1", len(schedule.Directions))
	}
	dir := schedule.Directions[0]
	if dir.DirectionID != "0" || dir.Headsign != "Downtown" || dir.TripCount != 2 {
		t.Fatalf("route schedule direction fields wrong: %#v", dir)
	}
	if len(dir.TripIDs) != 2 || len(dir.StopIDs) != 2 {
		t.Fatalf("route schedule trip/stop ID mapping wrong: %#v", dir)
	}
}

func TestGetRouteIDsForAgencyContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/route-ids-for-agency/test.json": envelopeList(`["test_10","test_11","test_12"]`),
	})

	result := invokeHandler(t, handler.getRouteIDsForAgency, map[string]any{"agency_id": "test"})
	page := dataAs[Page[string]](t, result)
	if len(page.Items) != 3 || page.Items[0] != "test_10" || page.Items[2] != "test_12" {
		t.Fatalf("route IDs mapping wrong: %#v", page.Items)
	}
}

func TestGetShapeContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/shape/test_shape.json": envelopeEntry(`{"points":"abcdef","length":1500}`),
	})

	result := invokeHandler(t, handler.getShape, map[string]any{"shape_id": "test_shape"})
	shape := dataAs[ShapeResponse](t, result)
	if shape.Points != "abcdef" || shape.Length != 1500 {
		t.Fatalf("shape mapping wrong: %#v", shape)
	}
}

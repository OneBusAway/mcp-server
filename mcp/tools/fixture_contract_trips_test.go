package tools

import "testing"

func TestGetTripContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/trip/test_trip.json": envelopeEntry(`{"id":"test_trip","routeId":"test_10","tripHeadsign":"Downtown","directionId":"0","serviceId":"weekday"}`),
	})

	result := invokeHandler(t, handler.getTrip, map[string]any{"trip_id": "test_trip"})
	trip := dataAs[TripResponse](t, result)
	if trip.ID != "test_trip" || trip.RouteID != "test_10" || trip.Headsign != "Downtown" {
		t.Fatalf("trip identity mapping wrong: %#v", trip)
	}
	if trip.DirectionID != "0" || trip.ServiceID != "weekday" {
		t.Fatalf("trip direction/service mapping wrong: %#v", trip)
	}
}

func TestGetTripDetailsContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/trip-details/test_trip.json": `{
			"code":200,
			"data":{
				"entry":{
					"tripId":"test_trip",
					"serviceDate":1770000000000,
					"schedule":{"stopTimes":[]},
					"status":{"phase":"in_progress","status":"default","vehicleId":"test_bus","orientation":90,"scheduleDeviation":120,"position":{"lat":27.9488,"lon":-82.4582}}
				},
				"references":{"trips":[{"id":"test_trip","routeId":"test_10","shapeId":"shape_1"}]}
			}
		}`,
	})

	result := invokeHandler(t, handler.getTripDetails, map[string]any{"trip_id": "test_trip"})
	details := dataAs[TripDetailsResponse](t, result)
	if details.TripID != "test_trip" || details.RouteID != "test_10" || details.ShapeID != "shape_1" {
		t.Fatalf("trip details identity mapping wrong (references not resolved): %#v", details)
	}
	if details.Phase != "in_progress" || details.VehicleID != "test_bus" {
		t.Fatalf("trip details status mapping wrong: %#v", details)
	}
	if details.Lat != 27.9488 || details.Lon != -82.4582 || details.Bearing != 90 {
		t.Fatalf("trip details position mapping wrong: %#v", details)
	}
	// scheduleDeviation is in seconds upstream; the tool must expose it in
	// minutes so agents don't have to convert.
	if details.DeviationMins != 2 {
		t.Fatalf("trip details deviation_mins = %v, want 2 (120 seconds / 60)", details.DeviationMins)
	}
	assertRequestPath(t, upstream, "/api/where/trip-details/test_trip.json", map[string]string{
		"includeSchedule": "true",
		"includeStatus":   "true",
	})
}

func TestGetTripForVehicleContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/trip-for-vehicle/test_bus.json": `{
			"code":200,
			"data":{
				"entry":{
					"tripId":"test_trip",
					"serviceDate":1770000000000,
					"schedule":{"stopTimes":[]},
					"status":{"phase":"in_progress","vehicleId":"test_bus","position":{"lat":27.9,"lon":-82.4}}
				},
				"references":{"trips":[{"id":"test_trip","routeId":"test_10"}]}
			}
		}`,
	})

	result := invokeHandler(t, handler.getTripForVehicle, map[string]any{"vehicle_id": "test_bus"})
	vehicle := dataAs[VehicleResponse](t, result)
	if vehicle.VehicleID != "test_bus" || vehicle.TripID != "test_trip" || vehicle.RouteID != "test_10" {
		t.Fatalf("trip-for-vehicle mapping wrong: %#v", vehicle)
	}
	if vehicle.Lat != 27.9 || vehicle.Lon != -82.4 {
		t.Fatalf("trip-for-vehicle position wrong: %#v", vehicle)
	}
}

func TestGetVehiclesForAgencyContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/vehicles-for-agency/test.json": envelopeList(`[{
			"vehicleId":"test_bus",
			"tripId":"test_trip",
			"phase":"in_progress",
			"location":{"lat":27.9,"lon":-82.4},
			"tripStatus":{
				"activeTripId":"test_trip",
				"scheduleDeviation":180,
				"activeTrip":{"id":"test_trip","routeId":"test_10","tripHeadsign":"Downtown"}
			}
		}]`),
	})

	result := invokeHandler(t, handler.getVehiclesForAgency, map[string]any{"agency_id": "test"})
	page := dataAs[Page[VehicleResponse]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("vehicles-for-agency = %d, want 1", len(page.Items))
	}
	vehicle := page.Items[0]
	if vehicle.VehicleID != "test_bus" || vehicle.RouteID != "test_10" || vehicle.Headsign != "Downtown" {
		t.Fatalf("vehicle status references not resolved: %#v", vehicle)
	}
	if vehicle.DeviationMins != 3 {
		t.Fatalf("vehicle deviation_mins = %v, want 3 (180 seconds / 60)", vehicle.DeviationMins)
	}
}

func TestGetTripsForLocationContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/trips-for-location.json": envelopeList(`[{
			"tripId":"test_trip",
			"trip":{"id":"test_trip","routeId":"test_10"},
			"status":{"phase":"in_progress","vehicleId":"test_bus","position":{"lat":27.9,"lon":-82.4}}
		}]`),
	})

	result := invokeHandler(t, handler.getTripsForLocation, map[string]any{"lat": 27.9, "lon": -82.4})
	page := dataAs[Page[VehicleResponse]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("trips-for-location = %d, want 1", len(page.Items))
	}
	vehicle := page.Items[0]
	if vehicle.TripID != "test_trip" || vehicle.RouteID != "test_10" || vehicle.VehicleID != "test_bus" {
		t.Fatalf("trips-for-location mapping wrong: %#v", vehicle)
	}
	assertRequestPath(t, upstream, "/api/where/trips-for-location.json", map[string]string{
		"lat":    "27.900000",
		"lon":    "-82.400000",
		"radius": "500",
	})
}

func TestGetBlockContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/block/test_block.json": envelopeEntry(`{
			"id":"test_block",
			"configurations":[{
				"activeServiceIds":["weekday","holiday"],
				"trips":[
					{"tripId":"test_trip1","trip":{"id":"test_trip1","routeId":"test_10","tripHeadsign":"Downtown"}},
					{"tripId":"test_trip2","trip":{"id":"test_trip2","routeId":"test_10","tripHeadsign":"Uptown"}}
				]
			}]
		}`),
	})

	result := invokeHandler(t, handler.getBlock, map[string]any{"block_id": "test_block"})
	block := dataAs[BlockResponse](t, result)
	if block.ID != "test_block" {
		t.Fatalf("block ID mapping wrong: %#v", block)
	}
	if len(block.Configurations) != 1 {
		t.Fatalf("block configurations = %d, want 1", len(block.Configurations))
	}
	config := block.Configurations[0]
	if len(config.ActiveServiceIDs) != 2 || config.ActiveServiceIDs[0] != "weekday" {
		t.Fatalf("block active service IDs mapping wrong: %#v", config.ActiveServiceIDs)
	}
	if len(config.Trips) != 2 || config.Trips[0].TripID != "test_trip1" || config.Trips[1].Headsign != "Uptown" {
		t.Fatalf("block trip mapping wrong: %#v", config.Trips)
	}
}

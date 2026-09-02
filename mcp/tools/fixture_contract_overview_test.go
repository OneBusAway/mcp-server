package tools

import "testing"

func TestGetStopOverviewContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": tzAwareAgency,
		"/api/where/arrivals-and-departures-for-stop/test_1013.json": `{
			"code":200,
			"data":{
				"entry":{
					"stopId":"test_1013",
					"arrivalsAndDepartures":[
						{"tripId":"test_trip1","routeId":"test_10","routeShortName":"10","tripHeadsign":"Downtown","predicted":true,"predictedArrivalTime":1770000360000,"scheduledArrivalTime":1770000300000},
						{"tripId":"test_trip2","routeId":"test_11","routeShortName":"11","tripHeadsign":"Uptown","predicted":false,"scheduledArrivalTime":1770000900000}
					]
				},
				"references":{"stops":[{"id":"test_1013","name":"Fixture Stop","lat":27.9,"lon":-82.4}]}
			}
		}`,
	})

	result := invokeHandler(t, handler.getStopOverview, map[string]any{"stop_id": "test_1013"})
	overview := dataAs[StopOverviewResponse](t, result)
	if overview.StopID != "test_1013" || overview.StopName != "Fixture Stop" {
		t.Fatalf("overview identity mapping wrong (references not resolved): %#v", overview)
	}
	if overview.Lat != 27.9 || overview.Lon != -82.4 {
		t.Fatalf("overview stop location mapping wrong: %#v", overview)
	}
	// The overview must dedupe routes across arrivals: two arrivals across
	// two routes yield exactly two routes_serving entries.
	if len(overview.Routes) != 2 || overview.Routes[0] != "test_10" || overview.Routes[1] != "test_11" {
		t.Fatalf("overview routes_serving mapping wrong: %#v", overview.Routes)
	}
	if len(overview.Next) != 2 {
		t.Fatalf("overview next_arrivals = %d, want 2", len(overview.Next))
	}
	first := overview.Next[0]
	if first.TripID != "test_trip1" || !first.Predicted || first.PredictedArrivalMS != 1_770_000_360_000 {
		t.Fatalf("overview first arrival mapping wrong: %#v", first)
	}
	assertRequestPath(t, upstream, "/api/where/arrivals-and-departures-for-stop/test_1013.json", map[string]string{
		"minutesBefore": "0",
		"minutesAfter":  "60",
	})
}

package tools

import (
	"context"
	"testing"

	"oba-mcp/client"
	"oba-mcp/internal/obafixture"
)

func TestArrivalsHandlerTransformsFixedOBADTOs(t *testing.T) {
	upstream := obafixture.New(map[string]obafixture.Response{
		"/api/where/agency/test.json": {Body: `{
			"code": 200,
			"data": {"entry": {"id": "test", "timezone": "America/Los_Angeles"}}
		}`},
		"/api/where/arrivals-and-departures-for-stop/test_1013.json": {Body: `{
			"code": 200,
			"data": {"entry": {
				"stopId": "test_1013",
				"arrivalsAndDepartures": [{
					"tripId": "test_trip", "routeId": "test_10", "routeShortName": "10",
					"tripHeadsign": "Downtown", "vehicleId": "test_bus", "predicted": true,
					"scheduledArrivalTime": 1770000300000,
					"predictedArrivalTime": 1770000360000,
					"tripStatus": {"orientation": 90, "position": {"lat": 27.9488, "lon": -82.4582}}
				}]
			}}
		}`},
	})
	t.Cleanup(upstream.Close)

	handler := &Handler{client: client.New(upstream.URL, "fixture-api-key", nil, nil)}
	result, err := handler.getArrivalsForStop(context.Background(), toolRequest(map[string]any{"stop_id": "test_1013"}))
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned tool error: %#v", result)
	}

	envelope, ok := result.StructuredContent.(SuccessEnvelope[any])
	if !ok {
		t.Fatalf("structured content type = %T, want SuccessEnvelope[any]", result.StructuredContent)
	}
	page, ok := envelope.Data.(Page[ArrivalResponse])
	if !ok {
		t.Fatalf("data type = %T, want Page[ArrivalResponse]", envelope.Data)
	}
	if len(page.Items) != 1 {
		t.Fatalf("arrivals = %#v, want one fixture arrival", page.Items)
	}
	arrival := page.Items[0]
	if arrival.TripID != "test_trip" || arrival.RouteName != "10" || !arrival.Predicted {
		t.Fatalf("arrival identity = %#v", arrival)
	}
	if arrival.PredictedArrivalMS != 1_770_000_360_000 || arrival.Timezone != "America/Los_Angeles" {
		t.Fatalf("machine-readable time contract = %#v", arrival)
	}
	if arrival.NumberOfStopsAway != nil {
		t.Fatalf("number_of_stops_away = %v, want nil because the fixture omitted it", *arrival.NumberOfStopsAway)
	}

	requests := upstream.Requests()
	if len(requests) != 2 {
		t.Fatalf("fixture requests = %#v, want agency timezone lookup and arrivals lookup", requests)
	}
	if got := requests[1].Query; got.Get("minutesBefore") != "0" || got.Get("minutesAfter") != "30" || got.Get("key") != "fixture-api-key" {
		t.Fatalf("arrivals query = %#v, want validated default window and API key", got)
	}
}

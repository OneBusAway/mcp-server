package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func testClient(t *testing.T, body string, check func(*http.Request)) *OBAClient {
	t.Helper()
	oba := New("https://oba.test", "test-key", nil, nil)
	oba.httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		check(request)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(body))}, nil
	})}
	return oba
}

func TestGetStopDecodesTypedEnvelope(t *testing.T) {
	oba := testClient(t, `{"code":200,"data":{"entry":{"id":"unitrans_1","name":"Memorial Union","lat":38.5,"lon":-121.7,"routeIds":["unitrans_A"]}}}`, func(request *http.Request) {
		if request.URL.Path != "/api/where/stop/unitrans_1.json" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.URL.Query().Get("key") != "test-key" {
			t.Fatal("API key was not supplied")
		}
	})
	response, err := oba.GetStop(context.Background(), "unitrans_1")
	if err != nil {
		t.Fatalf("GetStop returned error: %v", err)
	}
	if response.Data.Entry.ID != "unitrans_1" || response.Data.Entry.Name != "Memorial Union" {
		t.Fatalf("unexpected typed stop: %#v", response.Data.Entry)
	}
	if got := response.Data.Entry.RouteIDs; len(got) != 1 || got[0] != "unitrans_A" {
		t.Fatalf("unexpected route IDs: %#v", got)
	}
}

func TestArrivalsForStopDecodesNestedTypedFields(t *testing.T) {
	oba := testClient(t, `{"code":200,"data":{"entry":{"stopId":"unitrans_1","arrivalsAndDepartures":[{"tripId":"unitrans_trip","routeId":"unitrans_A","predicted":true,"scheduledArrivalTime":1000,"tripStatus":{"vehicleId":"unitrans_bus","position":{"lat":38.5,"lon":-121.7}}}]}}}`, func(*http.Request) {})
	response, err := oba.ArrivalsForStop(context.Background(), "unitrans_1", nil)
	if err != nil {
		t.Fatalf("ArrivalsForStop returned error: %v", err)
	}
	arrival := response.Data.Entry.ArrivalsAndDepartures[0]
	if arrival.TripStatus == nil || arrival.TripStatus.Position == nil || arrival.TripStatus.Position.Lat != 38.5 {
		t.Fatalf("nested fields were not decoded into DTOs: %#v", arrival)
	}
}

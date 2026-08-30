package tools

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAgencyResponseJSONContract(t *testing.T) {
	want := AgencyResponse{
		ID:       "unitrans",
		Name:     "Unitrans",
		URL:      "https://unitrans.ucdavis.edu",
		Phone:    "530-752-2877",
		Timezone: "America/Los_Angeles",
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got AgencyResponse
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded response = %#v, want %#v", got, want)
	}
}

func TestAgencySummaryJSONContract(t *testing.T) {
	want := []AgencySummary{{
		ID:   "unitrans",
		Name: "Unitrans",
		Lat:  38.5382,
		Lon:  -121.7617,
	}}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got []AgencySummary
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded response = %#v, want %#v", got, want)
	}
}

func TestToolResponseTypesMarshal(t *testing.T) {
	testCases := []struct {
		name  string
		value any
	}{
		{"stop", StopResponse{ID: "unitrans_1", Name: "Memorial Union", RouteIDs: []string{"unitrans_A"}}},
		{"stop IDs", StopIDsResponse{"unitrans_1"}},
		{"route", RouteResponse{ID: "unitrans_A", ShortName: "A", AgencyID: "unitrans"}},
		{"route IDs", RouteIDsResponse{"unitrans_A"}},
		{"arrival", ArrivalResponse{TripID: "unitrans_trip", RouteID: "unitrans_A", Predicted: true, Timezone: "America/Los_Angeles", PredictedArrivalMS: 1_770_000_300_000}},
		{"overview", StopOverviewResponse{StopID: "unitrans_1", Next: []ArrivalResponse{{TripID: "unitrans_trip"}}}},
		{"stop schedule", StopScheduleResponse{StopID: "unitrans_1", Timezone: "America/Los_Angeles", Routes: []RouteScheduleResponse{{RouteID: "unitrans_A"}}}},
		{"route stops", RouteStopsResponse{Directions: []RouteDirectionStopsResponse{{Direction: "Outbound"}}}},
		{"trip", TripResponse{ID: "unitrans_trip", RouteID: "unitrans_A"}},
		{"trip details", TripDetailsResponse{TripID: "unitrans_trip"}},
		{"vehicle", VehicleResponse{VehicleID: "unitrans_bus"}},
		{"route trip", RouteTripResponse{TripID: "unitrans_trip"}},
		{"route schedule", RouteScheduleStructureResponse{RouteID: "unitrans_A", Timezone: "America/Los_Angeles"}},
		{"shape", ShapeResponse{Points: "encoded"}},
		{"block", BlockResponse{ID: "unitrans_block"}},
		{"current time", CurrentTimeResponse{TimeMS: 1_767_225_600_000, TimeDisplay: "2026-01-01T00:00:00Z"}},
		{"metadata", MetadataResponse{RealtimeFeeds: map[string]FeedFreshness{"trip_updates": {UpdatedAtMS: 1_767_225_600_000, AgeSeconds: 10, Status: "fresh"}}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := json.Marshal(testCase.value); err != nil {
				t.Fatalf("response type does not marshal: %v", err)
			}
		})
	}
}

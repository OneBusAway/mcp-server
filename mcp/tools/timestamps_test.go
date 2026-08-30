package tools

import (
	"testing"
	"time"

	"oba-mcp/client"
)

func TestArrivalResponsePreservesMachineReadableTimes(t *testing.T) {
	arrival := arrivalResponse(client.ArrivalAndDeparture{
		TripID:                 "unitrans_trip",
		ServiceDate:            1_770_000_000_000,
		ScheduledArrivalTime:   1_770_000_300_000,
		ScheduledDepartureTime: 1_770_000_360_000,
		Predicted:              true,
		PredictedArrivalTime:   1_770_000_330_000,
		PredictedDepartureTime: 1_770_000_390_000,
	}, time.FixedZone("America/Los_Angeles", -8*60*60))

	if arrival.Timezone != "America/Los_Angeles" {
		t.Fatalf("timezone = %q", arrival.Timezone)
	}
	if arrival.ScheduledArrivalMS != 1_770_000_300_000 || arrival.PredictedArrivalMS != 1_770_000_330_000 {
		t.Fatalf("arrival timestamps = %#v", arrival)
	}
	if arrival.ScheduledArrivalDisplay == "" || arrival.PredictedArrivalDisplay == "" {
		t.Fatalf("display timestamps were not set: %#v", arrival)
	}
}

func TestStopScheduleResponseUsesAgencyTimezone(t *testing.T) {
	loc := time.FixedZone("America/Los_Angeles", -8*60*60)
	response := stopScheduleResponse("unitrans_stop", client.ScheduleForStop{
		Date: 1_770_000_000_000,
		StopRouteSchedules: []client.StopRouteSchedule{{
			RouteID: "unitrans_A",
			StopRouteDirectionSchedules: []client.StopRouteDirectionSchedule{{
				ScheduleStopTimes: []client.ScheduleStopTime{{TripID: "unitrans_trip", DepartureTime: 1_770_000_300_000}},
			}},
		}},
	}, loc)

	if response.Timezone != "America/Los_Angeles" {
		t.Fatalf("timezone = %q", response.Timezone)
	}
	trip := response.Routes[0].Directions[0].Trips[0]
	if trip.DepartureMS != 1_770_000_300_000 || trip.DepartureDisplay == "" {
		t.Fatalf("departure = %#v", trip)
	}
}

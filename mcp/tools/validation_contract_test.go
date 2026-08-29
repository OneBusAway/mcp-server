package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

type toolHandler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)

func toolRequest(arguments map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: arguments}}
}

func TestHandlersRejectInvalidArgumentsBeforeUpstreamCall(t *testing.T) {
	handler := &Handler{}
	tests := []struct {
		name      string
		handler   toolHandler
		arguments map[string]any
	}{
		{"agency ID", handler.getAgency, map[string]any{"agency_id": "bad/id"}},
		{"stop ID", handler.getStop, map[string]any{"stop_id": "bad/id"}},
		{"stop search", handler.searchStops, map[string]any{"query": "   "}},
		{"nearby stops coordinates", handler.findStopsNearLocation, map[string]any{"lat": 91.0, "lon": 0.0}},
		{"nearby stops mixed bounds", handler.findStopsNearLocation, map[string]any{"lat": 1.0, "lon": 1.0, "radius": 10.0, "lat_span": 1.0, "lon_span": 1.0}},
		{"stops for agency", handler.getStopsForAgency, map[string]any{"agency_id": "bad?id"}},
		{"stop schedule date", handler.getStopSchedule, map[string]any{"stop_id": "unitrans_1", "date": "2026-02-29"}},
		{"stop IDs for agency", handler.getStopIDsForAgency, map[string]any{"agency_id": "../unitrans"}},
		{"arrivals stop ID", handler.getArrivalsForStop, map[string]any{"stop_id": "stop/1"}},
		{"specific arrival timestamp", handler.getArrivalAndDepartureForStop, map[string]any{"stop_id": "unitrans_1", "trip_id": "unitrans_trip", "service_date": 1.5}},
		{"arrivals location radius", handler.getArrivalsForLocation, map[string]any{"lat": 1.0, "lon": 1.0, "radius": 5001.0}},
		{"route ID", handler.getRoute, map[string]any{"route_id": "route/1"}},
		{"route search", handler.searchRoutes, map[string]any{"query": "x", "max_count": 20.5}},
		{"routes for agency", handler.getRoutesForAgency, map[string]any{"agency_id": "agency%2froute"}},
		{"routes location span", handler.getRoutesForLocation, map[string]any{"lat": 1.0, "lon": 1.0, "lat_span": 0.0}},
		{"routes location incomplete span", handler.getRoutesForLocation, map[string]any{"lat": 1.0, "lon": 1.0, "lon_span": 1.0}},
		{"stops for route", handler.getStopsForRoute, map[string]any{"route_id": "route?x"}},
		{"trips for route time", handler.getTripsForRoute, map[string]any{"route_id": "unitrans_A", "time": 1.5}},
		{"schedule for route date", handler.getScheduleForRoute, map[string]any{"route_id": "unitrans_A", "date": "not-a-date"}},
		{"route IDs for agency", handler.getRouteIDsForAgency, map[string]any{"agency_id": "agency/route"}},
		{"shape ID", handler.getShape, map[string]any{"shape_id": "shape#1"}},
		{"trip ID", handler.getTrip, map[string]any{"trip_id": "trip/1"}},
		{"trip details boolean", handler.getTripDetails, map[string]any{"trip_id": "unitrans_trip", "include_status": "true"}},
		{"trip for vehicle time", handler.getTripForVehicle, map[string]any{"vehicle_id": "unitrans_1", "time": -1.0}},
		{"vehicles for agency", handler.getVehiclesForAgency, map[string]any{"agency_id": "agency\nname"}},
		{"trips for location longitude", handler.getTripsForLocation, map[string]any{"lat": 0.0, "lon": 181.0}},
		{"block ID", handler.getBlock, map[string]any{"block_id": "block?x"}},
		{"stop overview", handler.getStopOverview, map[string]any{"stop_id": "stop%2f1"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := testCase.handler(context.Background(), toolRequest(testCase.arguments))
			if err != nil {
				t.Fatalf("handler returned protocol error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("invalid arguments did not produce a tool error: %#v", result)
			}
		})
	}
}

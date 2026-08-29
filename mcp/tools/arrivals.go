package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerArrivalTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_arrival_and_departure_for_stop",
			mcp.WithDescription("Get a single specific arrival/departure at a stop for a known trip. Use this when you have a specific trip_id and service_date (from get_arrivals_for_stop results) and need precise real-time data for that one trip."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID (e.g. 'unitrans_12345')")),
			mcp.WithNumber("service_date", mcp.Required(), mcp.Description("Service date as Unix milliseconds (from arrivals list)")),
			mcp.WithString("vehicle_id", mcp.Description("Optional vehicle ID to disambiguate")),
		),
		h.getArrivalAndDepartureForStop,
	)

	s.AddTool(
		mcp.NewTool("get_arrivals_for_stop",
			mcp.WithDescription("Get arrivals and departures at a stop. Returns up to 10 arrivals in the next 30 minutes by default. Pass 'time' (epoch ms) to query at a specific moment (e.g. 7 AM). Pass 'minutes_after' to widen or narrow the window. Real-time predictions only available near current time."),
			mcp.WithString("stop_id", mcp.Required(), mcp.Description("Stop ID (e.g. 'unitrans_22274')")),
			mcp.WithNumber("minutes_before", mcp.Description("Include arrivals from this many minutes in the past (default: 5)")),
			mcp.WithNumber("minutes_after", mcp.Description("Include arrivals up to this many minutes ahead (default: 30)")),
			mcp.WithNumber("time", mcp.Description("Query arrivals at a specific time (Unix milliseconds since epoch). Use to find buses at 7 AM: compute epoch ms for that time. Defaults to now.")),
		),
		h.getArrivalsForStop,
	)

	s.AddTool(
		mcp.NewTool("get_arrivals_for_location",
			mcp.WithDescription("Get real-time arrivals and departures for all stops within a radius of GPS coordinates. Returns the same arrival detail as get_arrivals_for_stop but for every nearby stop at once. Useful for 'what buses are near me right now?' Returns up to 10 arrivals."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters")),
			mcp.WithNumber("minutes_before", mcp.Description("Minutes before now to include (default: 5)")),
			mcp.WithNumber("minutes_after", mcp.Description("Minutes after now to include (default: 35)")),
			mcp.WithNumber("time", mcp.Description("Query at specific time (epoch ms, defaults to now)")),
		),
		h.getArrivalsForLocation,
	)
}

type arrivalSummary struct {
	TripID             string `json:"trip_id"`
	ServiceDate        int64  `json:"service_date,omitempty"`
	RouteID            string `json:"route_id"`
	RouteName          string `json:"route_name"`
	Headsign           string `json:"headsign"`
	Predicted          bool   `json:"predicted"`
	ScheduledArrival   string `json:"scheduled_arrival"`
	PredictedArrival   string `json:"predicted_arrival,omitempty"`
	ScheduledDeparture string `json:"scheduled_departure"`
	PredictedDeparture string `json:"predicted_departure,omitempty"`
	// A pointer distinguishes a reported zero (the vehicle is at the stop) from
	// an unavailable value. Do not omit zero: clients use it as the arrival
	// signal.
	NumberOfStopsAway *int    `json:"number_of_stops_away,omitempty"`
	DistanceFromStop  float64 `json:"distance_from_stop_meters,omitempty"`
	Status            string  `json:"status"`
	VehicleID         string  `json:"vehicle_id,omitempty"`
	VehicleLat        float64 `json:"vehicle_lat,omitempty"`
	VehicleLon        float64 `json:"vehicle_lon,omitempty"`
	VehicleBearing    float64 `json:"vehicle_bearing,omitempty"`
}

func (h *Handler) getArrivalsForStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := req.RequireString("stop_id")
	if err != nil || stopID == "" {
		return mcp.NewToolResultError("stop_id is required"), nil
	}

	loc := h.client.TimezoneFor(client.AgencyIDFromEntityID(stopID))

	params := url.Values{}
	if mb := req.GetFloat("minutes_before", 0); mb > 0 {
		params.Set("minutesBefore", fmt.Sprintf("%.0f", mb))
	}
	params.Set("minutesAfter", fmt.Sprintf("%.0f", req.GetFloat("minutes_after", 30)))
	if t := req.GetFloat("time", 0); t > 0 {
		params.Set("time", fmt.Sprintf("%.0f", t))
	}

	resp, err := h.client.Get("/api/where/arrivals-and-departures-for-stop/"+stopID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No arrival data found for this stop."), nil
	}

	arrivalsRaw := client.AsSlice(entry["arrivalsAndDepartures"])
	if len(arrivalsRaw) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No upcoming arrivals at stop %s.", stopID)), nil
	}

	results := make([]arrivalSummary, 0, len(arrivalsRaw))
	for _, item := range arrivalsRaw {
		a, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ar := arrivalSummary{
			TripID:           client.StrVal(a["tripId"]),
			ServiceDate:      int64(client.FloatVal(a["serviceDate"])),
			RouteID:          client.StrVal(a["routeId"]),
			RouteName:        client.StrVal(a["routeShortName"]),
			Headsign:         client.StrVal(a["tripHeadsign"]),
			Status:           client.StrVal(a["status"]),
			VehicleID:        client.StrVal(a["vehicleId"]),
			Predicted:        a["predicted"] == true,
			DistanceFromStop: client.FloatVal(a["distanceFromStop"]),
		}
		if value, ok := a["numberOfStopsAway"]; ok {
			stopsAway := int(client.FloatVal(value))
			ar.NumberOfStopsAway = &stopsAway
		}
		if scheduled, ok := a["scheduledArrivalTime"].(float64); ok && scheduled > 0 {
			ar.ScheduledArrival = client.FormatRelativeTime(scheduled, loc)
		}
		if schedDep, ok := a["scheduledDepartureTime"].(float64); ok && schedDep > 0 {
			ar.ScheduledDeparture = client.FormatRelativeTime(schedDep, loc)
		}
		if ar.Predicted {
			if predicted, ok := a["predictedArrivalTime"].(float64); ok && predicted > 0 {
				ar.PredictedArrival = client.FormatRelativeTime(predicted, loc)
			}
			if predDep, ok := a["predictedDepartureTime"].(float64); ok && predDep > 0 {
				ar.PredictedDeparture = client.FormatRelativeTime(predDep, loc)
			}
			if ts, ok := a["tripStatus"].(map[string]any); ok {
				if pos, ok := ts["position"].(map[string]any); ok {
					ar.VehicleLat = client.FloatVal(pos["lat"])
					ar.VehicleLon = client.FloatVal(pos["lon"])
				}
				ar.VehicleBearing = client.FloatVal(ts["orientation"])
			}
		}
		results = append(results, ar)
	}

	total := len(results)
	note := ""
	if total > 10 {
		note = fmt.Sprintf(" (capped at 10; %d in window — pass a shorter minutes_after to see fewer)", total)
		results = results[:10]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Arrivals at stop %s (%d shown%s):\n%s", stopID, len(results), note, out)), nil
}

func (h *Handler) getArrivalAndDepartureForStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stopID, err := req.RequireString("stop_id")
	if err != nil || stopID == "" {
		return mcp.NewToolResultError("stop_id is required"), nil
	}
	tripID, err := req.RequireString("trip_id")
	if err != nil || tripID == "" {
		return mcp.NewToolResultError("trip_id is required"), nil
	}
	serviceDate, err := req.RequireFloat("service_date")
	if err != nil {
		return mcp.NewToolResultError("service_date is required"), nil
	}

	loc := h.client.TimezoneFor(client.AgencyIDFromEntityID(stopID))

	params := url.Values{
		"tripId":      {tripID},
		"serviceDate": {fmt.Sprintf("%.0f", serviceDate)},
	}
	if vehicleID := req.GetString("vehicle_id", ""); vehicleID != "" {
		params.Set("vehicleId", vehicleID)
	}

	resp, err := h.client.Get("/api/where/arrival-and-departure-for-stop/"+stopID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No arrival found for that trip at this stop."), nil
	}

	ar := arrivalSummary{
		TripID:           client.StrVal(entry["tripId"]),
		ServiceDate:      int64(client.FloatVal(entry["serviceDate"])),
		RouteID:          client.StrVal(entry["routeId"]),
		RouteName:        client.StrVal(entry["routeShortName"]),
		Headsign:         client.StrVal(entry["tripHeadsign"]),
		Status:           client.StrVal(entry["status"]),
		VehicleID:        client.StrVal(entry["vehicleId"]),
		Predicted:        entry["predicted"] == true,
		DistanceFromStop: client.FloatVal(entry["distanceFromStop"]),
	}
	if value, ok := entry["numberOfStopsAway"]; ok {
		stopsAway := int(client.FloatVal(value))
		ar.NumberOfStopsAway = &stopsAway
	}
	if scheduled, ok := entry["scheduledArrivalTime"].(float64); ok && scheduled > 0 {
		ar.ScheduledArrival = client.FormatRelativeTime(scheduled, loc)
	}
	if schedDep, ok := entry["scheduledDepartureTime"].(float64); ok && schedDep > 0 {
		ar.ScheduledDeparture = client.FormatRelativeTime(schedDep, loc)
	}
	if ar.Predicted {
		if predicted, ok := entry["predictedArrivalTime"].(float64); ok && predicted > 0 {
			ar.PredictedArrival = client.FormatRelativeTime(predicted, loc)
		}
		if predDep, ok := entry["predictedDepartureTime"].(float64); ok && predDep > 0 {
			ar.PredictedDeparture = client.FormatRelativeTime(predDep, loc)
		}
	}

	out, _ := json.MarshalIndent(ar, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Arrival for trip %s at stop %s:\n%s", tripID, stopID, out)), nil
}

func (h *Handler) getArrivalsForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, err := req.RequireFloat("lat")
	if err != nil {
		return mcp.NewToolResultError("lat is required"), nil
	}
	lon, err := req.RequireFloat("lon")
	if err != nil {
		return mcp.NewToolResultError("lon is required"), nil
	}

	params := url.Values{
		"lat":      {fmt.Sprintf("%f", lat)},
		"lon":      {fmt.Sprintf("%f", lon)},
		"maxCount": {"10"},
	}
	if r := req.GetFloat("radius", 0); r > 0 {
		params.Set("radius", fmt.Sprintf("%.0f", r))
	}
	if mb := req.GetFloat("minutes_before", 0); mb > 0 {
		params.Set("minutesBefore", fmt.Sprintf("%.0f", mb))
	}
	if ma := req.GetFloat("minutes_after", 0); ma > 0 {
		params.Set("minutesAfter", fmt.Sprintf("%.0f", ma))
	}
	if t := req.GetFloat("time", 0); t > 0 {
		params.Set("time", fmt.Sprintf("%.0f", t))
	}

	resp, err := h.client.Get("/api/where/arrivals-and-departures-for-location.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No arrival data found near that location."), nil
	}

	arrivalsRaw := client.AsSlice(entry["arrivalsAndDepartures"])
	if len(arrivalsRaw) == 0 {
		return mcp.NewToolResultText("No upcoming arrivals near that location."), nil
	}

	var loc = h.client.TimezoneFor("")
	for _, item := range arrivalsRaw {
		if a, ok := item.(map[string]any); ok {
			if agencyID := client.AgencyIDFromEntityID(client.StrVal(a["routeId"])); agencyID != "" {
				loc = h.client.TimezoneFor(agencyID)
				break
			}
		}
	}

	results := make([]arrivalSummary, 0, len(arrivalsRaw))
	for _, item := range arrivalsRaw {
		a, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ar := arrivalSummary{
			TripID:           client.StrVal(a["tripId"]),
			ServiceDate:      int64(client.FloatVal(a["serviceDate"])),
			RouteID:          client.StrVal(a["routeId"]),
			RouteName:        client.StrVal(a["routeShortName"]),
			Headsign:         client.StrVal(a["tripHeadsign"]),
			Predicted:        a["predicted"] == true,
			Status:           client.StrVal(a["status"]),
			VehicleID:        client.StrVal(a["vehicleId"]),
			DistanceFromStop: client.FloatVal(a["distanceFromStop"]),
		}
		if value, ok := a["numberOfStopsAway"]; ok {
			stopsAway := int(client.FloatVal(value))
			ar.NumberOfStopsAway = &stopsAway
		}
		if scheduled, ok := a["scheduledArrivalTime"].(float64); ok && scheduled > 0 {
			ar.ScheduledArrival = client.FormatRelativeTime(scheduled, loc)
		}
		if schedDep, ok := a["scheduledDepartureTime"].(float64); ok && schedDep > 0 {
			ar.ScheduledDeparture = client.FormatRelativeTime(schedDep, loc)
		}
		if ar.Predicted {
			if predicted, ok := a["predictedArrivalTime"].(float64); ok && predicted > 0 {
				ar.PredictedArrival = client.FormatRelativeTime(predicted, loc)
			}
			if predDep, ok := a["predictedDepartureTime"].(float64); ok && predDep > 0 {
				ar.PredictedDeparture = client.FormatRelativeTime(predDep, loc)
			}
			if ts, ok := a["tripStatus"].(map[string]any); ok {
				if pos, ok := ts["position"].(map[string]any); ok {
					ar.VehicleLat = client.FloatVal(pos["lat"])
					ar.VehicleLon = client.FloatVal(pos["lon"])
				}
				ar.VehicleBearing = client.FloatVal(ts["orientation"])
			}
		}
		results = append(results, ar)
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Arrivals near (%.4f, %.4f) — %d shown:\n%s", lat, lon, len(results), out)), nil
}

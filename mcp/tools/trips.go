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

func (h *Handler) registerTripTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_block",
			mcp.WithDescription("Get the block configuration for a block ID — the ordered sequence of trips a vehicle makes in a single day. Block IDs come from trip details."),
			mcp.WithString("block_id", mcp.Required(), mcp.Description("Block ID (from a trip's blockId field)")),
		),
		h.getBlock,
	)

	s.AddTool(
		mcp.NewTool("get_trip",
			mcp.WithDescription("Get basic info for a trip by ID: route, headsign, direction, service dates."),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID (e.g. 'unitrans_12345')")),
		),
		h.getTrip,
	)

	s.AddTool(
		mcp.NewTool("get_trip_details",
			mcp.WithDescription("Get full real-time status for a trip: vehicle position, schedule deviation, next stops, and phase. Use when the user asks where a specific bus is right now."),
			mcp.WithString("trip_id", mcp.Required(), mcp.Description("Trip ID (e.g. 'unitrans_12345')")),
			mcp.WithNumber("time", mcp.Description("Query trip at specific time (epoch ms)")),
			mcp.WithBoolean("include_schedule", mcp.Description("Include full stop schedule in response (default true)")),
			mcp.WithBoolean("include_status", mcp.Description("Include real-time status in response (default true)")),
		),
		h.getTripDetails,
	)

	s.AddTool(
		mcp.NewTool("get_trip_for_vehicle",
			mcp.WithDescription("Get the current trip being served by a specific vehicle. Use when the user asks about a specific bus by vehicle number."),
			mcp.WithString("vehicle_id", mcp.Required(), mcp.Description("Vehicle ID (e.g. 'unitrans_1')")),
			mcp.WithNumber("time", mcp.Description("Query trip at specific time (epoch ms)")),
		),
		h.getTripForVehicle,
	)

	s.AddTool(
		mcp.NewTool("get_vehicles_for_agency",
			mcp.WithDescription("Get real-time positions and status of every active vehicle for an agency. Use ONLY when the user explicitly asks for the whole agency fleet or all buses running; for buses approaching a specific stop, use get_arrivals_for_stop instead."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getVehiclesForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_trips_for_location",
			mcp.WithDescription("Get active trips near GPS coordinates. Useful for finding buses currently operating in an area."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters (default: 500)")),
		),
		h.getTripsForLocation,
	)
}

func (h *Handler) getTrip(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tripID, err := req.RequireString("trip_id")
	if err != nil || tripID == "" {
		return mcp.NewToolResultError("trip_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/trip/"+tripID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No trip found with ID %q.", tripID)), nil
	}

	type result struct {
		ID          string `json:"id"`
		RouteID     string `json:"route_id"`
		Headsign    string `json:"headsign,omitempty"`
		DirectionID string `json:"direction_id,omitempty"`
		ServiceID   string `json:"service_id,omitempty"`
	}

	out, _ := json.MarshalIndent(result{
		ID:          client.StrVal(entry["id"]),
		RouteID:     client.StrVal(entry["routeId"]),
		Headsign:    client.StrVal(entry["tripHeadsign"]),
		DirectionID: client.StrVal(entry["directionId"]),
		ServiceID:   client.StrVal(entry["serviceId"]),
	}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Trip %s:\n%s", tripID, out)), nil
}

func (h *Handler) getTripDetails(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tripID, err := req.RequireString("trip_id")
	if err != nil || tripID == "" {
		return mcp.NewToolResultError("trip_id is required"), nil
	}

	params := url.Values{}
	if t := req.GetFloat("time", 0); t > 0 {
		params.Set("time", fmt.Sprintf("%.0f", t))
	}
	args := req.GetArguments()
	if _, ok := args["include_schedule"]; ok {
		params.Set("includeSchedule", fmt.Sprintf("%v", req.GetBool("include_schedule", true)))
	}
	if _, ok := args["include_status"]; ok {
		params.Set("includeStatus", fmt.Sprintf("%v", req.GetBool("include_status", true)))
	}

	resp, err := h.client.Get("/api/where/trip-details/"+tripID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No trip found with that ID."), nil
	}

	type result struct {
		TripID        string  `json:"trip_id"`
		RouteID       string  `json:"route_id,omitempty"`
		ShapeID       string  `json:"shape_id,omitempty"`
		Phase         string  `json:"phase,omitempty"`
		Status        string  `json:"status,omitempty"`
		VehicleID     string  `json:"vehicle_id,omitempty"`
		Lat           float64 `json:"lat,omitempty"`
		Lon           float64 `json:"lon,omitempty"`
		Bearing       float64 `json:"bearing,omitempty"`
		DeviationMins float64 `json:"deviation_mins,omitempty"`
	}

	detail := result{TripID: tripID}
	if tripStatus, ok := entry["status"].(map[string]any); ok {
		detail.Phase = client.StrVal(tripStatus["phase"])
		detail.Status = client.StrVal(tripStatus["status"])
		detail.VehicleID = client.StrVal(tripStatus["vehicleId"])
		detail.DeviationMins = client.FloatVal(tripStatus["scheduleDeviation"]) / 60
		detail.Bearing = client.FloatVal(tripStatus["orientation"])
		if pos, ok := tripStatus["position"].(map[string]any); ok {
			detail.Lat = client.FloatVal(pos["lat"])
			detail.Lon = client.FloatVal(pos["lon"])
		}
	}
	// OBA commonly places the trip definition in references.trips rather than
	// entry.trip. Read both forms so route shapes are available to map clients.
	trip, _ := entry["trip"].(map[string]any)
	if trip == nil {
		if refs, ok := data["references"].(map[string]any); ok {
			for _, ref := range client.AsSlice(refs["trips"]) {
				candidate, ok := ref.(map[string]any)
				if ok && client.StrVal(candidate["id"]) == tripID {
					trip = candidate
					break
				}
			}
		}
	}
	if trip != nil {
		detail.RouteID = client.StrVal(trip["routeId"])
		detail.ShapeID = client.StrVal(trip["shapeId"])
	}

	out, _ := json.MarshalIndent(detail, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Trip details for %s:\n%s", tripID, out)), nil
}

func (h *Handler) getTripForVehicle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	vehicleID, err := req.RequireString("vehicle_id")
	if err != nil || vehicleID == "" {
		return mcp.NewToolResultError("vehicle_id is required"), nil
	}

	params := url.Values{}
	if t := req.GetFloat("time", 0); t > 0 {
		params.Set("time", fmt.Sprintf("%.0f", t))
	}

	resp, err := h.client.Get("/api/where/trip-for-vehicle/"+vehicleID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No active trip found for vehicle %q.", vehicleID)), nil
	}

	type result struct {
		TripID        string  `json:"trip_id,omitempty"`
		RouteID       string  `json:"route_id,omitempty"`
		Headsign      string  `json:"headsign,omitempty"`
		Phase         string  `json:"phase,omitempty"`
		VehicleID     string  `json:"vehicle_id,omitempty"`
		Lat           float64 `json:"lat,omitempty"`
		Lon           float64 `json:"lon,omitempty"`
		DeviationMins float64 `json:"deviation_mins,omitempty"`
	}

	v := result{}
	if status, ok := entry["status"].(map[string]any); ok {
		v.TripID = client.StrVal(status["activeTripId"])
		v.Phase = client.StrVal(status["phase"])
		v.VehicleID = client.StrVal(status["vehicleId"])
		v.DeviationMins = client.FloatVal(status["scheduleDeviation"]) / 60
		if pos, ok := status["position"].(map[string]any); ok {
			v.Lat = client.FloatVal(pos["lat"])
			v.Lon = client.FloatVal(pos["lon"])
		}
		if activeTrip, ok := status["activeTrip"].(map[string]any); ok {
			v.RouteID = client.StrVal(activeTrip["routeId"])
			v.Headsign = client.StrVal(activeTrip["tripHeadsign"])
		}
	}
	if v.TripID == "" {
		if trip, ok := entry["trip"].(map[string]any); ok {
			v.TripID = client.StrVal(trip["id"])
			if v.RouteID == "" {
				v.RouteID = client.StrVal(trip["routeId"])
			}
			if v.Headsign == "" {
				v.Headsign = client.StrVal(trip["tripHeadsign"])
			}
		}
	}

	out, _ := json.MarshalIndent(v, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Current trip for vehicle %s:\n%s", vehicleID, out)), nil
}

func (h *Handler) getVehiclesForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/vehicles-for-agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No active vehicles for agency %s.", agencyID)), nil
	}

	type vehicleSummary struct {
		VehicleID     string  `json:"vehicle_id"`
		TripID        string  `json:"trip_id,omitempty"`
		RouteID       string  `json:"route_id,omitempty"`
		Phase         string  `json:"phase,omitempty"`
		Lat           float64 `json:"lat,omitempty"`
		Lon           float64 `json:"lon,omitempty"`
		DeviationMins float64 `json:"deviation_mins,omitempty"`
	}

	results := make([]vehicleSummary, 0, len(list))
	for _, item := range list {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sv := vehicleSummary{
			VehicleID: client.StrVal(v["vehicleId"]),
			Phase:     client.StrVal(v["phase"]),
		}
		if pos, ok := v["location"].(map[string]any); ok {
			sv.Lat = client.FloatVal(pos["lat"])
			sv.Lon = client.FloatVal(pos["lon"])
		}
		if ts, ok := v["tripStatus"].(map[string]any); ok {
			sv.TripID = client.StrVal(ts["activeTripId"])
			sv.DeviationMins = client.FloatVal(ts["scheduleDeviation"]) / 60
			if activeTrip, ok := ts["activeTrip"].(map[string]any); ok {
				sv.RouteID = client.StrVal(activeTrip["routeId"])
			}
		}
		results = append(results, sv)
	}

	total := len(results)
	note := ""
	if total > 50 {
		note = fmt.Sprintf(" (capped at 50; %d active)", total)
		results = results[:50]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Active vehicles for agency %s (%d shown%s):\n%s", agencyID, len(results), note, out)), nil
}

func (h *Handler) getTripsForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lat, err := req.RequireFloat("lat")
	if err != nil {
		return mcp.NewToolResultError("lat is required"), nil
	}
	lon, err := req.RequireFloat("lon")
	if err != nil {
		return mcp.NewToolResultError("lon is required"), nil
	}
	radius := req.GetFloat("radius", 500)

	params := url.Values{
		"lat":    {fmt.Sprintf("%f", lat)},
		"lon":    {fmt.Sprintf("%f", lon)},
		"radius": {fmt.Sprintf("%.0f", radius)},
	}

	resp, err := h.client.Get("/api/where/trips-for-location.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText("No active trips found near that location."), nil
	}

	type tripSummary struct {
		TripID    string  `json:"trip_id"`
		RouteID   string  `json:"route_id,omitempty"`
		Headsign  string  `json:"headsign,omitempty"`
		VehicleID string  `json:"vehicle_id,omitempty"`
		Phase     string  `json:"phase,omitempty"`
		Lat       float64 `json:"lat,omitempty"`
		Lon       float64 `json:"lon,omitempty"`
	}

	results := make([]tripSummary, 0, len(list))
	for _, item := range list {
		t, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ts := tripSummary{TripID: client.StrVal(t["tripId"])}
		if trip, ok := t["trip"].(map[string]any); ok {
			ts.RouteID = client.StrVal(trip["routeId"])
			ts.Headsign = client.StrVal(trip["tripHeadsign"])
		}
		if status, ok := t["status"].(map[string]any); ok {
			ts.VehicleID = client.StrVal(status["vehicleId"])
			ts.Phase = client.StrVal(status["phase"])
			if pos, ok := status["position"].(map[string]any); ok {
				ts.Lat = client.FloatVal(pos["lat"])
				ts.Lon = client.FloatVal(pos["lon"])
			}
		}
		results = append(results, ts)
	}

	total := len(results)
	note := ""
	if total > 20 {
		note = fmt.Sprintf(" (capped at 20; %d in area)", total)
		results = results[:20]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Active trips near (%.4f, %.4f) — %d shown%s:\n%s", lat, lon, len(results), note, out)), nil
}

func (h *Handler) getBlock(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	blockID, err := req.RequireString("block_id")
	if err != nil || blockID == "" {
		return mcp.NewToolResultError("block_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/block/"+blockID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No block found with ID %q.", blockID)), nil
	}

	type tripInBlock struct {
		TripID   string `json:"trip_id"`
		RouteID  string `json:"route_id,omitempty"`
		Headsign string `json:"headsign,omitempty"`
	}
	type configResult struct {
		ServiceIDs []string      `json:"active_service_ids"`
		Trips      []tripInBlock `json:"trips"`
	}

	var configs []configResult
	for _, cfgRaw := range client.AsSlice(entry["configurations"]) {
		cfg, ok := cfgRaw.(map[string]any)
		if !ok {
			continue
		}
		cr := configResult{}
		for _, sid := range client.AsSlice(cfg["activeServiceIds"]) {
			cr.ServiceIDs = append(cr.ServiceIDs, fmt.Sprintf("%v", sid))
		}
		for _, tRaw := range client.AsSlice(cfg["trips"]) {
			t, ok := tRaw.(map[string]any)
			if !ok {
				continue
			}
			trip, _ := t["trip"].(map[string]any)
			if trip == nil {
				continue
			}
			cr.Trips = append(cr.Trips, tripInBlock{
				TripID:   client.StrVal(trip["id"]),
				RouteID:  client.StrVal(trip["routeId"]),
				Headsign: client.StrVal(trip["tripHeadsign"]),
			})
		}
		configs = append(configs, cr)
	}

	out, _ := json.MarshalIndent(configs, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Block %s configurations:\n%s", blockID, out)), nil
}

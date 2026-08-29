package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"oba-mcp/client"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerRouteTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_route_ids_for_agency",
			mcp.WithDescription("Get a flat list of all route IDs for an agency. Use get_routes_for_agency if you also need names and details."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getRouteIDsForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_shape",
			mcp.WithDescription("Get the encoded polyline shape for a GTFS shape ID. Returns the path geometry a vehicle follows. Shape IDs come from trip details or stop-times data."),
			mcp.WithString("shape_id", mcp.Required(), mcp.Description("GTFS shape ID")),
		),
		h.getShape,
	)

	s.AddTool(
		mcp.NewTool("get_route",
			mcp.WithDescription("Get details for a single route by ID: short name, long name, agency, color. Use when you already have a route_id."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
		),
		h.getRoute,
	)

	s.AddTool(
		mcp.NewTool("search_routes",
			mcp.WithDescription("Search routes by number or name (e.g. 'E', 'express'). Returns matching route IDs and names. Use this to find a route_id when you only have a name."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Route number or name to search for")),
			mcp.WithNumber("max_count", mcp.Description("Max results to return (default: 5)")),
		),
		h.searchRoutes,
	)

	s.AddTool(
		mcp.NewTool("get_routes_for_agency",
			mcp.WithDescription("List routes operated by an agency. Returns up to 30 by default."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getRoutesForAgency,
	)

	s.AddTool(
		mcp.NewTool("get_routes_for_location",
			mcp.WithDescription("Find routes that serve an area near GPS coordinates."),
			mcp.WithNumber("lat", mcp.Required(), mcp.Description("Latitude")),
			mcp.WithNumber("lon", mcp.Required(), mcp.Description("Longitude")),
			mcp.WithNumber("radius", mcp.Description("Search radius in meters (default: 500)")),
			mcp.WithNumber("lat_span", mcp.Description("Bounding box latitude span (alternative to radius)")),
			mcp.WithNumber("lon_span", mcp.Description("Bounding box longitude span (alternative to radius)")),
		),
		h.getRoutesForLocation,
	)

	s.AddTool(
		mcp.NewTool("get_stops_for_route",
			mcp.WithDescription("Get all stops on a route, organized by direction (inbound/outbound)."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
		),
		h.getStopsForRoute,
	)

	s.AddTool(
		mcp.NewTool("get_trips_for_route",
			mcp.WithDescription("Get active trips currently running on a route, with real-time status and vehicle info."),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithNumber("time", mcp.Description("Query trips at specific time (epoch ms)")),
			mcp.WithBoolean("include_schedule", mcp.Description("Include full stop schedule in response (default true)")),
			mcp.WithBoolean("include_status", mcp.Description("Include real-time status in response (default true)")),
		),
		h.getTripsForRoute,
	)

	s.AddTool(
		mcp.NewTool("get_schedule_for_route",
			mcp.WithDescription("Get the structure of a route's schedule for a date: directions, trip IDs, and stop order. Does not include per-stop departure times (use get_stop_schedule for a stop's timetable with times). Use this to answer 'what trips run on route E?' or 'how many trips does route L have?'"),
			mcp.WithString("route_id", mcp.Required(), mcp.Description("Route ID (e.g. 'unitrans_E')")),
			mcp.WithString("date", mcp.Description("Date in YYYY-MM-DD format (defaults to today)")),
		),
		h.getScheduleForRoute,
	)
}

type routeSummary struct {
	ID        string `json:"id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name,omitempty"`
	AgencyID  string `json:"agency_id"`
	Color     string `json:"color,omitempty"`
}

func routeFromMap(m map[string]any) routeSummary {
	return routeSummary{
		ID:        client.StrVal(m["id"]),
		ShortName: client.StrVal(m["shortName"]),
		LongName:  client.StrVal(m["longName"]),
		AgencyID:  client.StrVal(m["agencyId"]),
		Color:     client.StrVal(m["color"]),
	}
}

func (h *Handler) getRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := req.RequireString("route_id")
	if err != nil || routeID == "" {
		return mcp.NewToolResultError("route_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/route/"+routeID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No route found with ID %q.", routeID)), nil
	}

	out, _ := json.MarshalIndent(routeFromMap(entry), "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Route %s:\n%s", routeID, out)), nil
}

func (h *Handler) searchRoutes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	params := url.Values{
		"input":    {query},
		"maxCount": {fmt.Sprintf("%.0f", req.GetFloat("max_count", 5))},
	}

	resp, err := h.client.Get("/api/where/search/route.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No routes found matching %q.", query)), nil
	}

	results := make([]routeSummary, 0, len(list))
	for _, item := range list {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, routeFromMap(r))
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d routes matching %q:\n%s", len(results), query, out)), nil
}

func (h *Handler) getRoutesForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/routes-for-agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	results := make([]routeSummary, 0, len(list))
	for _, item := range list {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, routeFromMap(r))
	}

	total := len(results)
	note := ""
	if total > 30 {
		note = fmt.Sprintf(" (showing 30 of %d — search by name for a specific route)", total)
		results = results[:30]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Routes for agency %s (%d shown%s):\n%s", agencyID, len(results), note, out)), nil
}

func (h *Handler) getRoutesForLocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if ls := req.GetFloat("lat_span", 0); ls > 0 {
		params.Set("latSpan", fmt.Sprintf("%f", ls))
	}
	if ls := req.GetFloat("lon_span", 0); ls > 0 {
		params.Set("lonSpan", fmt.Sprintf("%f", ls))
	}

	resp, err := h.client.Get("/api/where/routes-for-location.json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText("No routes found near that location."), nil
	}

	results := make([]routeSummary, 0, len(list))
	for _, item := range list {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, routeFromMap(r))
	}

	total := len(results)
	note := ""
	if total > 20 {
		note = fmt.Sprintf(" (capped at 20; %d near that location)", total)
		results = results[:20]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d routes near (%.4f, %.4f)%s:\n%s", len(results), lat, lon, note, out)), nil
}

func (h *Handler) getStopsForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := req.RequireString("route_id")
	if err != nil || routeID == "" {
		return mcp.NewToolResultError("route_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/stops-for-route/"+routeID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No stops found for this route."), nil
	}

	type refStop struct {
		Name string
		Lat  float64
		Lon  float64
	}
	stopLookup := map[string]refStop{}
	if refs, ok := data["references"].(map[string]any); ok {
		for _, s := range client.AsSlice(refs["stops"]) {
			stop, ok := s.(map[string]any)
			if !ok {
				continue
			}
			stopLookup[client.StrVal(stop["id"])] = refStop{
				Name: client.StrVal(stop["name"]),
				Lat:  client.FloatVal(stop["lat"]),
				Lon:  client.FloatVal(stop["lon"]),
			}
		}
	}

	type directionGroup struct {
		Direction string        `json:"direction"`
		Stops     []stopSummary `json:"stops"`
	}
	type routePolyline struct {
		Points string  `json:"encoded_polyline"`
		Length float64 `json:"length,omitempty"`
	}

	var groups []directionGroup
	for _, sg := range client.AsSlice(entry["stopGroupings"]) {
		grouping, ok := sg.(map[string]any)
		if !ok {
			continue
		}
		for _, sub := range client.AsSlice(grouping["stopGroups"]) {
			group, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			nameObj, _ := group["name"].(map[string]any)
			stopIDs := client.AsSlice(group["stopIds"])
			stops := make([]stopSummary, 0, len(stopIDs))
			for _, sid := range stopIDs {
				id := fmt.Sprintf("%v", sid)
				ref := stopLookup[id]
				stops = append(stops, stopSummary{ID: id, Name: ref.Name, Lat: ref.Lat, Lon: ref.Lon})
			}
			dirNote := ""
			if len(stops) > 30 {
				dirNote = fmt.Sprintf(" [first 30 of %d]", len(stops))
				stops = stops[:30]
			}
			groups = append(groups, directionGroup{
				Direction: client.StrVal(nameObj["name"]) + dirNote,
				Stops:     stops,
			})
		}
	}

	polylines := make([]routePolyline, 0)
	for _, item := range client.AsSlice(entry["polylines"]) {
		polyline, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if points := client.StrVal(polyline["points"]); points != "" {
			polylines = append(polylines, routePolyline{
				Points: points,
				Length: client.FloatVal(polyline["length"]),
			})
		}
	}

	// Preserve the direction and stop information, but include the canonical
	// OBA geometry. Consumers must draw these polylines instead of joining
	// stop coordinates with straight segments.
	out, _ := json.MarshalIndent(struct {
		Directions []directionGroup `json:"directions"`
		Polylines  []routePolyline  `json:"polylines,omitempty"`
	}{
		Directions: groups,
		Polylines:  polylines,
	}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Stops for route %s:\n%s", routeID, out)), nil
}

func (h *Handler) getTripsForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := req.RequireString("route_id")
	if err != nil || routeID == "" {
		return mcp.NewToolResultError("route_id is required"), nil
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

	resp, err := h.client.Get("/api/where/trips-for-route/"+routeID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No active trips found for route %s.", routeID)), nil
	}

	type tripSummary struct {
		TripID    string  `json:"trip_id"`
		Headsign  string  `json:"headsign,omitempty"`
		Phase     string  `json:"phase,omitempty"`
		VehicleID string  `json:"vehicle_id,omitempty"`
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
			ts.Headsign = client.StrVal(trip["tripHeadsign"])
		}
		if status, ok := t["status"].(map[string]any); ok {
			ts.Phase = client.StrVal(status["phase"])
			ts.VehicleID = client.StrVal(status["vehicleId"])
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
		note = fmt.Sprintf(" (capped at 20; %d active)", total)
		results = results[:20]
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Active trips for route %s (%d shown%s):\n%s", routeID, len(results), note, out)), nil
}

func (h *Handler) getScheduleForRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeID, err := req.RequireString("route_id")
	if err != nil || routeID == "" {
		return mcp.NewToolResultError("route_id is required"), nil
	}

	params := url.Values{}
	if date := req.GetString("date", ""); date != "" {
		params.Set("date", date)
	}

	resp, err := h.client.Get("/api/where/schedule-for-route/"+routeID+".json", params)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText("No schedule data found for this route."), nil
	}

	type directionSummary struct {
		DirectionID string   `json:"direction_id"`
		Headsign    string   `json:"headsign"`
		TripCount   int      `json:"trip_count"`
		TripIDs     []string `json:"trip_ids"`
		StopIDs     []string `json:"stop_ids"`
	}
	type scheduleResult struct {
		RouteID    string             `json:"route_id"`
		Date       string             `json:"date,omitempty"`
		Directions []directionSummary `json:"directions"`
	}

	result := scheduleResult{RouteID: routeID}
	if d := client.FloatVal(entry["scheduleDate"]); d > 0 {
		result.Date = time.UnixMilli(int64(d)).Format("2006-01-02")
	}

	for _, grpRaw := range client.AsSlice(entry["stopTripGroupings"]) {
		grp, ok := grpRaw.(map[string]any)
		if !ok {
			continue
		}
		dir := directionSummary{
			DirectionID: client.StrVal(grp["directionId"]),
			Headsign:    client.StrVal(grp["tripHeadsign"]),
		}
		for _, tid := range client.AsSlice(grp["tripIds"]) {
			dir.TripIDs = append(dir.TripIDs, fmt.Sprintf("%v", tid))
		}
		for _, sid := range client.AsSlice(grp["stopIds"]) {
			dir.StopIDs = append(dir.StopIDs, fmt.Sprintf("%v", sid))
		}
		dir.TripCount = len(dir.TripIDs)
		result.Directions = append(result.Directions, dir)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Route schedule structure for %s:\n%s\nFor departure times at a specific stop, use get_stop_schedule.", routeID, out)), nil
}

func (h *Handler) getRouteIDsForAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/route-ids-for-agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	ids := make([]string, 0, len(list))
	for _, item := range list {
		ids = append(ids, fmt.Sprintf("%v", item))
	}

	out, _ := json.MarshalIndent(ids, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Route IDs for agency %s (%d total):\n%s", agencyID, len(ids), out)), nil
}

func (h *Handler) getShape(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	shapeID, err := req.RequireString("shape_id")
	if err != nil || shapeID == "" {
		return mcp.NewToolResultError("shape_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/shape/"+shapeID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No shape found with ID %q.", shapeID)), nil
	}

	type result struct {
		Points string  `json:"encoded_polyline"`
		Length float64 `json:"length,omitempty"`
	}
	out, _ := json.MarshalIndent(result{
		Points: client.StrVal(entry["points"]),
		Length: client.FloatVal(entry["length"]),
	}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Shape %s:\n%s", shapeID, out)), nil
}

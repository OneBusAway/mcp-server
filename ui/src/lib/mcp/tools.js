/**
 * Centralized MCP tool name registry.
 *
 * The MCP server may be configured with OBA_TOOL_PROFILE=rider (16 tools) or
 * OBA_TOOL_PROFILE=all (29 tools). RIDER_TOOLS is the UI's filter applied to
 * whatever listTools() returns; if the server exposes fewer tools the
 * intersection is used automatically — no validation failure.
 */

/** Tool names the rider UI may call. Subset of the full 29-tool MCP surface. */
export const RIDER_TOOLS = new Set([
	'get_agencies',
	'get_agency',
	'get_stop',
	'search_stops',
	'find_stops_near_location',
	'get_stop_overview',
	'get_stops_for_agency',
	'get_arrivals_for_stop',
	'get_arrivals_for_location',
	'get_arrival_and_departure_for_stop',
	'get_stop_schedule',
	'get_route',
	'search_routes',
	'get_routes_for_agency',
	'get_routes_for_location',
	'get_stops_for_route',
	'get_trip_details',
	'get_trip_for_vehicle',
	'get_trips_for_route',
	'get_vehicles_for_agency',
	'get_current_time',
]);

/** Tools that produce map geometry; triggers the map_loading SSE card. */
export const MAP_RENDERING_TOOLS = new Set([
	'get_arrivals_for_stop',
	'find_stops_near_location',
	'search_stops',
	'get_stop',
	'get_stop_overview',
	'get_stops_for_route',
	'get_trips_for_route',
	'get_trip_details',
	'get_trips_for_location',
]);

/** Narrow tool set for arrival-focused single-stop requests. */
export const ARRIVAL_FOCUSED_TOOLS = new Set(['search_stops', 'get_arrivals_for_stop']);

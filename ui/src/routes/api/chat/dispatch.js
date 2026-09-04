/**
 * MCP tool dispatch for the chat route.
 *
 * dispatchTool calls the MCP server, extracts data from Phase-4 structured
 * envelopes (or falls back gracefully to legacy plain values), emits SSE
 * cards when applicable, and returns the result for the model's tool history.
 */

import { unwrap, items, normalizeArrivals } from '$lib/result.js';
import { MAP_RENDERING_TOOLS } from '$lib/mcp/tools.js';
import { modelToolError } from '$lib/mcp/tool-error.js';
import { vehicleFromTripDetails } from '$lib/vehicles.js';
import {
	createMapState,
	addMarkers,
	addDirections,
	addVehicles,
	addRouteId,
	addRouteData,
	addRouteToMap,
	toMapMarkers,
	routeStopMarkers,
	routeDirections,
	markCurrentStop,
} from './map.js';

export { createMapState };

/**
 * Call one MCP tool, emit any applicable SSE cards, and return the raw
 * result for the model's tool history.
 *
 * @param {string}   name       - tool name
 * @param {object}   input      - tool arguments
 * @param {object}   controller - ReadableStream controller
 * @param {Function} sse        - sse(controller, obj) emitter
 * @param {object}   mapState   - shared map state for this turn
 * @param {object}   emitted    - dedup sets: { routeIds, stopIds, arrivalStopIds }
 */
export async function dispatchTool(name, input, controller, sse, mapState, emitted, mcp) {
	const _e = emitted ?? { routeIds: new Set(), stopIds: new Set(), arrivalStopIds: new Set() };
	if (name === 'get_arrivals_for_stop' && mapState.requiresStopChoice) {
		return {
			data: null,
			clarification_required: true,
			message:
				'Multiple stops matched the search. Ask the user to choose a stop ID before requesting arrivals.',
		};
	}
	// Map geometry is accumulated across tool calls and emitted once at the end
	// of the turn. Tell the browser immediately so it can reserve the map space
	// and show progress while those calls are still running.
	if (MAP_RENDERING_TOOLS.has(name) && !_e.mapLoading) {
		_e.mapLoading = true;
		sse(controller, { t: 'card', v: { type: 'map_loading' } });
	}
	let result;
	try {
		result = await mcp.callTool(name, input);
		const d = unwrap(result);
		const list = items(result);

		if (name === 'get_arrivals_for_stop') {
			const stopMeta = await _handleArrivalsForStop(
				input,
				d,
				list,
				controller,
				sse,
				mapState,
				_e,
				mcp,
			);
			if (stopMeta) result = { ...result, stop_meta: stopMeta };
		} else if (name === 'find_stops_near_location') {
			const stops = list.length ? list : Array.isArray(d) ? d : [];
			if (stops.length) addMarkers(mapState, markCurrentStop(toMapMarkers(stops)));
		} else if (name === 'search_stops') {
			const stops = list.length ? list : Array.isArray(d) ? d : [];
			if (stops.length) {
				for (const stop of stops) {
					if (stop?.id) mapState.stopsById.set(stop.id, stop);
				}
				addMarkers(mapState, markCurrentStop(toMapMarkers(stops)));
			}
			if (stops.length > 1) {
				mapState.requiresStopChoice = true;
				result = {
					...result,
					clarification_required: true,
					model_instruction:
						'Do not call another tool or choose the first result. Ask the user which listed stop ID they mean.',
				};
			}
		} else if (name === 'get_stop' && d?.lat) {
			mapState.stopsById.set(input.stop_id, { ...d, id: d.id ?? input.stop_id });
			addMarkers(mapState, [
				{
					id: input.stop_id,
					lat: d.lat,
					lon: d.lon,
					name: d.name ?? input.stop_id,
					is_current: true,
				},
			]);
		} else if (name === 'get_stop_overview') {
			const stopMeta = _handleStopOverview(input, d, controller, sse, mapState, _e);
			if (stopMeta) result = { ...result, stop_meta: stopMeta };
		} else if (name === 'get_stops_for_route') {
			if (!_e.routeIds.has(input.route_id)) {
				if (addRouteData(mapState, input.route_id, d)) _e.routeIds.add(input.route_id);
			}
		} else if (name === 'get_trips_for_route') {
			const trips = list.length ? list : Array.isArray(d) ? d : [];
			if (trips.length) {
				addVehicles(
					mapState,
					trips.map((trip) => ({
						...trip,
						route_id: input.route_id,
						route_short_name: String(input.route_id).split('_').at(-1),
					})),
				);
			}
			await addRouteToMap(mapState, input.route_id, _e, mcp);
		} else if (name === 'get_trip_details' && d?.lat != null && d?.lon != null) {
			const vehicle = vehicleFromTripDetails({ ...d, trip_id: d.trip_id ?? input.trip_id });
			if (vehicle) addVehicles(mapState, [vehicle]);
			await addRouteToMap(mapState, d.active_route_id ?? d.route_id, _e, mcp);
		} else if (name === 'get_trips_for_location') {
			const trips = list.length ? list : Array.isArray(d) ? d : [];
			if (trips.length)
				addVehicles(
					mapState,
					trips.filter((v) => v.lat && v.lon),
				);
		} else if (
			name === 'search_routes' ||
			name === 'get_routes_for_agency' ||
			name === 'get_routes_for_location'
		) {
			const routes = (list.length ? list : Array.isArray(d) ? d : []).slice(0, 6);
			if (routes.length) {
				sse(controller, {
					t: 'card',
					v: {
						type: 'route_suggestions',
						routes: routes.map((r) => ({
							id: r.id,
							name: r.short_name ?? r.id,
							description: r.long_name ?? '',
							color: r.color || null,
						})),
					},
				});
			}
		}
		// get_vehicles_for_agency is intentionally a no-op for map purposes.
	} catch (e) {
		result = modelToolError(e);
	}
	return result;
}

// ---------------------------------------------------------------------------
// Per-tool handlers
// ---------------------------------------------------------------------------

async function _handleArrivalsForStop(input, d, list, controller, sse, mapState, _e, mcp) {
	const arrivals = normalizeArrivals(list.length ? list : Array.isArray(d) ? d : []);
	const searchedStop = mapState.stopsById.get(input.stop_id);
	const stopName = searchedStop?.name ?? input.stop_id;
	const arrivalWindow = {
		minutes_before: 0,
		minutes_after: Number.isFinite(input.minutes_after) ? input.minutes_after : 60,
	};

	if (arrivals.length && !_e.arrivalStopIds.has(input.stop_id)) {
		sse(controller, {
			t: 'card',
			v: {
				type: 'arrivals',
				stop_id: input.stop_id,
				stop_name: stopName,
				arrivals,
				arrival_window: arrivalWindow,
			},
		});
		_e.arrivalStopIds.add(input.stop_id);
	}

	// Scheduled arrivals are still useful map context: show the stop and its
	// routes even when the agency has not supplied real-time predictions.
	const routeIds = [...new Set(arrivals.map((a) => a.route_id).filter(Boolean))].slice(0, 6);
	const stopLookup = searchedStop
		? Promise.resolve(searchedStop)
		: mcp.callTool('get_stop', { stop_id: input.stop_id });
	const lookupResults = await Promise.allSettled([
		stopLookup,
		...routeIds.map((route_id) => mcp.callTool('get_stops_for_route', { route_id })),
		...routeIds.map((route_id) => mcp.callTool('get_trips_for_route', { route_id, limit: 50 })),
	]);
	const [stopRes] = lookupResults;
	const routeResults = lookupResults.slice(1, 1 + routeIds.length);
	const vehicleResults = lookupResults.slice(1 + routeIds.length);

	let stopLat = null,
		stopLon = null;
	if (stopRes.status === 'fulfilled') {
		const stopData = unwrap(stopRes.value);
		if (stopData?.lat != null && stopData?.lon != null) {
			stopLat = stopData.lat;
			stopLon = stopData.lon;
			mapState.stopsById.set(input.stop_id, { ...stopData, id: stopData.id ?? input.stop_id });
		}
	}

	const routeNames = new Map(
		arrivals.flatMap((a) => [
			[a.route_id, a.route_name ?? a.route_id],
			[a.active_route_id, a.active_route_id?.split('_').at(-1) ?? a.active_route_id],
		]),
	);
	const directions = routeResults.flatMap((routeRes, index) => {
		const routeData = routeRes.status === 'fulfilled' ? unwrap(routeRes.value) : null;
		addMarkers(mapState, routeStopMarkers(routeData));
		const routeName = routeNames.get(routeIds[index]) ?? routeIds[index];
		return routeDirections(routeData, routeName, routeIds[index]);
	});
	const mappedVehicles = vehicleResults.flatMap((vehicleRes, index) => {
		if (vehicleRes.status !== 'fulfilled') return [];
		const routeId = routeIds[index];
		const routeName = routeNames.get(routeId) ?? routeId.split('_').at(-1);
		return items(vehicleRes.value)
			.map((vehicle) =>
				vehicleFromTripDetails(vehicle, {
					route_short_name: routeName,
					headsign: vehicle.headsign,
				}),
			)
			.filter(Boolean);
	});
	const realtimeTripIds = [...new Set(mappedVehicles.map((vehicle) => vehicle.trip_id))].slice(
		0,
		6,
	);

	const trip_info = {};
	for (const vehicle of mappedVehicles) {
		if (vehicle.trip_id) {
			trip_info[vehicle.trip_id] = {
				route_short_name: vehicle.route_short_name,
				headsign: vehicle.headsign,
			};
		}
	}

	if (stopLat != null) {
		addMarkers(mapState, [
			{ id: input.stop_id, lat: stopLat, lon: stopLon, name: stopName, is_current: true },
		]);
	}
	addDirections(mapState, directions);
	addVehicles(mapState, mappedVehicles);
	for (const tripId of realtimeTripIds) {
		if (!mapState.tripIds.has(tripId)) {
			mapState.tripIds.add(tripId);
			mapState.vehicleTripIds.push(tripId);
		}
	}
	Object.assign(mapState.tripInfo, trip_info);
	for (const routeId of routeIds) {
		addRouteId(mapState, routeId);
		_e.routeIds.add(routeId);
	}
	_e.stopIds.add(input.stop_id);

	return stopLat != null && stopLon != null ? { lat: stopLat, lon: stopLon, name: stopName } : null;
}

function _handleStopOverview(input, d, controller, sse, mapState, _e) {
	const stopName = d?.stop_name ?? input.stop_id;
	if (d?.lat) {
		addMarkers(mapState, [
			{ id: input.stop_id, lat: d.lat, lon: d.lon, name: stopName, is_current: true },
		]);
	}
	const nextArrivals = normalizeArrivals(d?.next_arrivals ?? []);
	if (nextArrivals.length && !_e.arrivalStopIds.has(input.stop_id)) {
		sse(controller, {
			t: 'card',
			v: { type: 'arrivals', stop_id: input.stop_id, stop_name: stopName, arrivals: nextArrivals },
		});
		_e.arrivalStopIds.add(input.stop_id);
	}

	return d?.lat != null && d?.lon != null ? { lat: d.lat, lon: d.lon, name: stopName } : null;
}

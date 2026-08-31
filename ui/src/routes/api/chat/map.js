/**
 * Map-state management and geometry helpers for the chat server.
 *
 * All functions that receive a route result accept an already-unwrapped
 * RouteStopsResponse object (call unwrap() from $lib/result.js before passing).
 */

import { unwrap } from '$lib/result.js';

// ---------------------------------------------------------------------------
// Geometry
// ---------------------------------------------------------------------------

/**
 * Decode a Google-encoded polyline into [lon, lat] coordinate pairs
 * (MapLibre order).
 */
export function decodePolyline(encoded) {
	const coordinates = [];
	let index = 0, lat = 0, lon = 0;
	while (index < encoded.length) {
		let result = 0, shift = 0, byte;
		do {
			byte = encoded.charCodeAt(index++) - 63;
			result |= (byte & 0x1f) << shift;
			shift += 5;
		} while (byte >= 0x20 && index < encoded.length);
		lat += result & 1 ? ~(result >> 1) : result >> 1;

		result = 0; shift = 0;
		do {
			byte = encoded.charCodeAt(index++) - 63;
			result |= (byte & 0x1f) << shift;
			shift += 5;
		} while (byte >= 0x20 && index < encoded.length);
		lon += result & 1 ? ~(result >> 1) : result >> 1;
		coordinates.push([lon / 1e5, lat / 1e5]);
	}
	return coordinates;
}

/** Convert a stops array to map marker objects. */
export function toMapMarkers(stops) {
	return (Array.isArray(stops) ? stops : [])
		.filter((s) => s.lat && s.lon)
		.map((s) => ({ lat: s.lat, lon: s.lon, name: s.name, id: s.id }));
}

/**
 * Flatten a RouteStopsResponse into stop marker objects.
 * Accepts the unwrapped data object, not the full envelope.
 */
export function routeStopMarkers(route) {
	return (route?.directions ?? [])
		.flatMap((direction) => direction.stops ?? [])
		.map((stop) => ({ id: stop.id, lat: stop.lat, lon: stop.lon, name: stop.name }));
}

/**
 * Prefer the agency's actual shape, with an ordered stop-path fallback when
 * the upstream route does not include encoded polylines.
 */
export function routeDirections(route, routeName) {
	const shapePaths = (route?.polylines ?? []).flatMap((polyline) => {
		const coordinates = decodePolyline(polyline.encoded_polyline ?? '');
		return coordinates.length >= 2 ? [{ direction: routeName, coordinates }] : [];
	});
	if (shapePaths.length) return shapePaths;

	return (route?.directions ?? []).flatMap((direction) => {
		const coordinates = (direction.stops ?? [])
			.filter((stop) => stop?.lat != null && stop?.lon != null)
			.map((stop) => [stop.lon, stop.lat]);
		return coordinates.length >= 2
			? [{ direction: direction.direction || routeName, coordinates }]
			: [];
	});
}

/** Mark the single marker in a list as the current/focused stop. */
export function markCurrentStop(markers) {
	if (markers.length === 1) markers[0].is_current = true;
	return markers;
}

// ---------------------------------------------------------------------------
// Map state
// ---------------------------------------------------------------------------

export function createMapState() {
	return {
		markers: [], directions: [], vehicles: [], routeIds: [], vehicleTripIds: [], tripInfo: {},
		stopsById: new Map(),
		requiresStopChoice: false,
		markerKeys: new Set(), markerIndexes: new Map(), currentMarkerKey: null,
		directionKeys: new Set(), vehicleKeys: new Set(), routeIdSet: new Set(), tripIds: new Set(),
	};
}

export function addMarkers(mapState, markers) {
	for (const marker of markers) {
		if (marker?.lat == null || marker?.lon == null) continue;
		const key = marker.id ?? `${marker.lat},${marker.lon}`;
		const isCurrent = marker.is_current && (!mapState.currentMarkerKey || mapState.currentMarkerKey === key);
		if (mapState.markerKeys.has(key)) {
			const index = mapState.markerIndexes.get(key);
			if (isCurrent && !mapState.markers[index].is_current) {
				mapState.markers[index] = { ...mapState.markers[index], ...marker, is_current: true };
				mapState.currentMarkerKey = key;
			}
			continue;
		}
		mapState.markerKeys.add(key);
		mapState.markerIndexes.set(key, mapState.markers.length);
		mapState.markers.push({ ...marker, is_current: isCurrent });
		if (isCurrent) mapState.currentMarkerKey = key;
	}
}

export function addDirections(mapState, directions) {
	for (const direction of directions) {
		const coordinates = direction?.coordinates ?? [];
		if (coordinates.length < 2) continue;
		const key = `${direction.direction ?? ''}:${JSON.stringify(coordinates)}`;
		if (mapState.directionKeys.has(key)) continue;
		mapState.directionKeys.add(key);
		mapState.directions.push(direction);
	}
}

export function addVehicles(mapState, vehicles) {
	for (const vehicle of vehicles) {
		if (vehicle?.lat == null || vehicle?.lon == null) continue;
		const key = vehicle.vehicle_id ?? vehicle.trip_id ?? `${vehicle.lat},${vehicle.lon}`;
		if (mapState.vehicleKeys.has(key)) continue;
		mapState.vehicleKeys.add(key);
		mapState.vehicles.push(vehicle);
	}
}

export function addRouteId(mapState, routeId) {
	if (!routeId || mapState.routeIdSet.has(routeId)) return;
	mapState.routeIdSet.add(routeId);
	mapState.routeIds.push(routeId);
}

/**
 * Add route geometry and stops to the map state.
 * @param {object} mapState
 * @param {string} routeId
 * @param {object} route - unwrapped RouteStopsResponse (not the full envelope)
 * @returns {boolean} true if any geometry was added
 */
export function addRouteData(mapState, routeId, route) {
	const routeStops = routeStopMarkers(route);
	addMarkers(mapState, routeStops);
	const directions = routeDirections(route, routeId);
	addDirections(mapState, directions);
	if (routeStops.length || directions.length) addRouteId(mapState, routeId);
	return routeStops.length > 0 || directions.length > 0;
}

/**
 * Fetch and add a route's geometry to the map state unless already emitted.
 */
export async function addRouteToMap(mapState, routeId, emitted, mcp) {
	if (!routeId || emitted.routeIds.has(routeId)) return;
	const result = await mcp.callTool('get_stops_for_route', { route_id: routeId });
	if (addRouteData(mapState, routeId, unwrap(result))) emitted.routeIds.add(routeId);
}

export function flushMapState(controller, mapState, sseFn) {
	if (!mapState.markers.length && !mapState.directions.length && !mapState.vehicles.length) return;
	sseFn(controller, {
		t: 'card',
		v: {
			type: 'combined_map',
			markers: mapState.markers,
			directions: mapState.directions,
			vehicles: mapState.vehicles,
			route_ids: mapState.routeIds,
			trip_ids: mapState.vehicleTripIds,
			trip_info: mapState.tripInfo,
		},
	});
}

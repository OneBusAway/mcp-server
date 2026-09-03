/** Return true when a vehicle has usable geographic coordinates. */
export function hasVehiclePosition(vehicle) {
	return (
		vehicle != null &&
		vehicle.lat != null &&
		vehicle.lon != null &&
		vehicle.lat !== '' &&
		vehicle.lon !== '' &&
		Number.isFinite(Number(vehicle.lat)) &&
		Number.isFinite(Number(vehicle.lon))
	);
}

function cleanId(value) {
	if (value == null) return null;
	const id = String(value).trim();
	return id || null;
}

// Route numbers are not unique across agencies. Prefer the full OneBusAway
// route ID whenever a vehicle supplies one.
export function vehicleMatchesRoute(vehicle, routeId) {
	const vehicleRouteId = cleanId(vehicle?.active_route_id ?? vehicle?.route_id);
	const candidateRouteId = cleanId(routeId);
	if (vehicleRouteId) return vehicleRouteId === candidateRouteId;
	const routeName = cleanId(vehicle?.route_short_name);
	return !!routeName && routeName === candidateRouteId?.split('_').at(-1);
}

export function vehicleRequiresRouteGeometry(vehicle) {
	return !!cleanId(vehicle?.active_route_id ?? vehicle?.route_id);
}

export function vehicleFromArrival(arrival) {
	if (
		arrival?.vehicle_lat == null ||
		arrival?.vehicle_lon == null ||
		(arrival.active_trip_id && !arrival.active_route_id)
	) {
		return null;
	}
	const activeTripId = arrival.active_trip_id ?? arrival.trip_id;
	const activeRouteId = arrival.active_route_id ?? arrival.route_id;
	return {
		lat: arrival.vehicle_lat,
		lon: arrival.vehicle_lon,
		vehicle_id: arrival.vehicle_id,
		trip_id: arrival.trip_id,
		arrival_route_id: arrival.route_id,
		active_trip_id: activeTripId,
		active_route_id: activeRouteId,
		active_shape_id: arrival.active_shape_id,
		route_id: activeRouteId,
		route_short_name: arrival.active_route_id?.split('_').at(-1) ?? arrival.route_name,
		headsign: arrival.active_headsign ?? arrival.headsign,
		stops_away: arrival.number_of_stops_away,
		};
}

export function vehicleFromTripDetails(update, tripInfo = {}) {
	if (
		update?.lat == null ||
		update?.lon == null ||
		(update.active_trip_id && !update.active_route_id)
	) {
		return null;
	}
	const activeTripId = update.active_trip_id ?? update.trip_id;
	const activeRouteId = update.active_route_id ?? update.route_id;
	return {
		...tripInfo,
		...update,
		arrival_route_id: update.route_id,
		active_trip_id: activeTripId,
		...(activeRouteId != null && { active_route_id: activeRouteId, route_id: activeRouteId }),
		route_short_name: update.active_route_id?.split('_').at(-1) ?? tripInfo.route_short_name,
	};
}

/** Markers are keyed by active trip ID, with vehicle ID as the fallback. */
export function vehicleAliases(vehicle) {
	const aliases = [];
	const activeTripId = cleanId(vehicle?.active_trip_id);
	const tripId = cleanId(vehicle?.trip_id);
	const vehicleId = cleanId(vehicle?.vehicle_id);
	if (activeTripId) aliases.push(`active-trip:${activeTripId}`);
	if (vehicleId) aliases.push(`vehicle:${vehicleId}`);
	if (tripId) aliases.push(`arrival-trip:${tripId}`);
	return aliases;
}

/** Anonymous records still need a deterministic marker key. */
export function vehicleKey(vehicle) {
	const alias = vehicleAliases(vehicle)[0];
	if (alias) return alias;
	if (!hasVehiclePosition(vehicle)) return null;
	const route =
		cleanId(vehicle?.active_route_id ?? vehicle?.route_id ?? vehicle?.route_short_name) ??
		'unknown';
	return `anonymous:${route}:${Number(vehicle.lon).toFixed(6)},${Number(vehicle.lat).toFixed(6)}`;
}

function mergeDefined(existing, incoming) {
	const merged = { ...existing };
	for (const [key, value] of Object.entries(incoming ?? {})) {
		// Arrival lists are chronological. When an interlined physical vehicle is
		// also advertised for a later trip, keep the first/current active trip key.
		if (key === 'trip_id' && existing.trip_id) continue;
		if (value !== undefined && value !== null && value !== '') merged[key] = value;
	}
	return merged;
}

/**
 * Merge duplicate trip/vehicle records. Incoming non-empty fields win except
 * trip_id (first/current active trip is kept); route metadata from an earlier
 * response is retained when a later poll omits it.
 */
export function deduplicateVehicles(vehicles) {
	const result = [];
	const aliasIndexes = new Map();
	const anonymousIndexes = new Map();

	for (const vehicle of Array.isArray(vehicles) ? vehicles : []) {
		if (!hasVehiclePosition(vehicle)) continue;
		const normalized = { ...vehicle, lat: Number(vehicle.lat), lon: Number(vehicle.lon) };
		const aliases = vehicleAliases(normalized);
		let index = aliases.map((alias) => aliasIndexes.get(alias)).find((value) => value != null);

		if (index == null && aliases.length === 0) {
			index = anonymousIndexes.get(vehicleKey(normalized));
		}

		if (index == null) {
			index = result.length;
			result.push(normalized);
		} else {
			result[index] = mergeDefined(result[index], normalized);
		}

		for (const alias of new Set([...aliases, ...vehicleAliases(result[index])])) {
			aliasIndexes.set(alias, index);
		}
		if (aliases.length === 0) anonymousIndexes.set(vehicleKey(result[index]), index);
	}

	return result;
}

export function replaceRefreshedVehicles(current, updates, refreshedTripIds) {
	const refreshed =
		refreshedTripIds instanceof Set ? refreshedTripIds : new Set(refreshedTripIds ?? []);
	const retained = (Array.isArray(current) ? current : []).filter(
		(vehicle) => !vehicle?.trip_id || !refreshed.has(vehicle.trip_id),
	);
	return deduplicateVehicles([...retained, ...(Array.isArray(updates) ? updates : [])]);
}

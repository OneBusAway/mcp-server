export function trackedArrivalRequest(tracker) {
	const input = {
		stop_id: tracker.stop_id,
		trip_id: tracker.trip_id,
		service_date: tracker.service_date,
	};
	if (tracker.vehicle_id) input.vehicle_id = tracker.vehicle_id;
	return { name: 'get_arrival_and_departure_for_stop', input };
}

export function shouldPollMapVehicles({ agencyId, vehicleTripIds, hasStopTracker, stopId = null }) {
	// Arrival maps already receive vehicle status from their stop-arrivals
	// response. Refreshing those trip IDs independently can return a different
	// active trip and overwrite the map with an unrelated vehicle location.
	if (stopId) return false;
	return !!agencyId || ((vehicleTripIds?.length ?? 0) > 0 && !hasStopTracker);
}

/**
 * Keep the stop-wide arrivals feed current until per-trip tracking takes
 * ownership. hasStopTracker is true when *any* tracker exists for the stop —
 * once tracking is active for any arrival there, precise per-trip polling owns
 * the panel and the broad query pauses for all arrivals at that stop.
 */
export function shouldPollArrivalsPanel({ stopId, hasStopTracker }) {
	return !!stopId && !hasStopTracker;
}

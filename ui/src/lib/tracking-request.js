export function trackedArrivalRequest(tracker) {
	const input = {
		stop_id: tracker.stop_id,
		trip_id: tracker.trip_id,
		service_date: tracker.service_date,
	};
	if (tracker.vehicle_id) input.vehicle_id = tracker.vehicle_id;
	return { name: 'get_arrival_and_departure_for_stop', input };
}

export function shouldPollMapVehicles({ agencyId, vehicleTripIds, hasStopTracker }) {
	return !!agencyId || ((vehicleTripIds?.length ?? 0) > 0 && !hasStopTracker);
}

/**
 * Keep the stop-wide arrivals feed current until precise per-trip tracking takes
 * ownership. This prevents tracking from accidentally restarting the broad
 * arrivals query that the user explicitly narrowed to one trip.
 */
export function shouldPollArrivalsPanel({ stopId, hasStopTracker }) {
	return !!stopId && !hasStopTracker;
}

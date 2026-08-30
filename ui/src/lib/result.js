/**
 * Normalization helpers for Phase-4 MCP structured envelopes.
 *
 * Phase-4 success:  { data, meta: { generated_at_ms, truncated, cache }, warnings }
 * Phase-4 error:    thrown as Error with .code, .retryable, .retryAfterMs
 * Legacy (pre-4):   plain value — array, object, or string
 *
 * All helpers accept either shape so callers need no version checks.
 */

/** Extract the data payload from an envelope or return the plain legacy value. */
export function unwrap(result) {
	if (result != null && typeof result === 'object' && 'meta' in result) {
		return result.data ?? null;
	}
	return result ?? null;
}

/** Extract an array of items from a paginated or plain-array result. */
export function items(result) {
	const d = unwrap(result);
	if (d == null) return [];
	if (Array.isArray(d)) return d;
	if (Array.isArray(d?.items)) return d.items;
	return [];
}

/** Return the meta block from a Phase-4 envelope, or null for legacy results. */
export function resultMeta(result) {
	return result != null && typeof result === 'object' && 'meta' in result
		? (result.meta ?? null)
		: null;
}

/** True when the result envelope reports a truncated list. */
export function isTruncated(result) {
	return resultMeta(result)?.truncated === true;
}

/**
 * Normalize arrival field names across Phase-4 and legacy shapes into a
 * single object that exposes both old and new field names. Components can
 * use either without version checks.
 *
 * Phase-4 → legacy aliases added:
 *   route_name           → route_short_name
 *   scheduled_arrival_display → scheduled_arrival
 *   predicted_arrival_display → predicted_arrival
 *   service_date_ms      → service_date
 */
export function normalizeArrival(a) {
	if (a == null || typeof a !== 'object') return a;
	const routeName = a.route_name ?? a.route_short_name ?? '';
	const schedDisplay = a.scheduled_arrival_display ?? a.scheduled_arrival ?? '';
	const predDisplay = a.predicted_arrival_display ?? a.predicted_arrival ?? '';
	const schedDepDisplay = a.scheduled_departure_display ?? a.scheduled_departure ?? '';
	const predDepDisplay = a.predicted_departure_display ?? a.predicted_departure ?? '';
	const serviceDate = a.service_date ?? a.service_date_ms ?? 0;
	return {
		...a,
		route_name: routeName,
		route_short_name: routeName,
		scheduled_arrival: schedDisplay,
		scheduled_arrival_display: schedDisplay,
		predicted_arrival: predDisplay,
		predicted_arrival_display: predDisplay,
		scheduled_departure: schedDepDisplay,
		scheduled_departure_display: schedDepDisplay,
		predicted_departure: predDepDisplay,
		predicted_departure_display: predDepDisplay,
		service_date: serviceDate,
		service_date_ms: a.service_date_ms ?? a.service_date ?? 0,
	};
}

/** Normalize an array of arrivals. */
export function normalizeArrivals(arrivals) {
	return (Array.isArray(arrivals) ? arrivals : []).map(normalizeArrival);
}

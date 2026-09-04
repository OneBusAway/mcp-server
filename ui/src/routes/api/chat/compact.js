/**
 * Model-facing compaction of MCP tool results.
 *
 * The UI keeps the full result for cards and maps; the model only sees the
 * fields it needs to answer or make a follow-up call. This is the single
 * biggest lever on input tokens per turn.
 *
 * Every compactor also returns `ledgerAdds` — {kind, id, name} entries the
 * client should remember so future turns can reference identifiers without
 * re-fetching. Values (times, positions, counts) never go in the ledger.
 */

import { unwrap, items, normalizeArrivals } from '$lib/result.js';

const MAX_LIST_ITEMS = 20;

function truncate(list, cap = MAX_LIST_ITEMS) {
	const arr = Array.isArray(list) ? list : [];
	return { items: arr.slice(0, cap), truncated: arr.length > cap };
}

function compactArrivalRow(a) {
	return {
		route: a.route_name ?? a.route_short_name ?? null,
		route_id: a.route_id ?? null,
		trip_id: a.trip_id ?? null,
		vehicle_id: a.vehicle_id ?? null,
		service_date: a.service_date ?? a.service_date_ms ?? null,
		headsign: a.headsign ?? null,
		predicted: !!a.predicted,
		predicted_ms: a.predicted_arrival_ms ?? null,
		scheduled_ms: a.scheduled_arrival_ms ?? null,
		stops_away: a.number_of_stops_away ?? null,
		deviation_seconds: a.deviation_seconds ?? null,
	};
}

function compactStop(s) {
	return { id: s?.id ?? null, name: s?.name ?? null, code: s?.code ?? null };
}

function compactRoute(r) {
	return {
		id: r?.id ?? null,
		short_name: r?.short_name ?? null,
		long_name: r?.long_name ?? null,
		agency_id: r?.agency_id ?? null,
	};
}

function compactAgency(a) {
	return { id: a?.id ?? null, name: a?.name ?? null };
}

function stopLedgerEntry(stop) {
	if (!stop?.id) return null;
	return {
		kind: 'stop',
		id: stop.id,
		name: stop.name ?? stop.id,
		lat: stop.lat ?? undefined,
		lon: stop.lon ?? undefined,
	};
}

function ledgerFromArrivals(arrivals, stopId, stopName, stopMeta) {
	const entries = [];
	if (stopId) {
		entries.push({
			kind: 'stop',
			id: stopId,
			name: stopName ?? stopId,
			lat: stopMeta?.lat ?? undefined,
			lon: stopMeta?.lon ?? undefined,
		});
	}
	const seenRoutes = new Set();
	for (const a of arrivals) {
		const rid = a.route_id;
		if (rid && !seenRoutes.has(rid)) {
			seenRoutes.add(rid);
			entries.push({
				kind: 'route',
				id: rid,
				name: a.route_name ?? a.route_short_name ?? rid,
			});
		}
	}
	return entries;
}

// ---------------------------------------------------------------------------
// Per-tool compactors
// ---------------------------------------------------------------------------

function compact_get_arrivals_for_stop(input, result) {
	const arrivals = normalizeArrivals(items(result));
	const cap = truncate(arrivals);
	const stopId = input?.stop_id ?? null;
	const stopName = arrivals[0]?.stop_name ?? result?.stop_meta?.name ?? null;
	return {
		payload: {
			stop_id: stopId,
			stop_name: stopName,
			arrivals: cap.items.map(compactArrivalRow),
			truncated: cap.truncated,
			map_rendered: true,
		},
		ledgerAdds: ledgerFromArrivals(cap.items, stopId, stopName, result?.stop_meta),
	};
}

function compact_get_arrival_and_departure_for_stop(input, result) {
	const d = unwrap(result);
	const one = d ? compactArrivalRow(d) : null;
	return {
		payload: { stop_id: input?.stop_id ?? null, arrival: one },
		ledgerAdds: one?.route_id
			? [{ kind: 'route', id: one.route_id, name: one.route ?? one.route_id }]
			: [],
	};
}

function compact_get_arrivals_for_location(_input, result) {
	const arrivals = normalizeArrivals(items(result));
	const cap = truncate(arrivals);
	return {
		payload: {
			arrivals: cap.items.map(compactArrivalRow),
			truncated: cap.truncated,
			map_rendered: true,
		},
		ledgerAdds: ledgerFromArrivals(cap.items, null, null),
	};
}

function compact_get_stop_overview(input, result) {
	const d = unwrap(result) ?? {};
	const arrivals = normalizeArrivals(d.next_arrivals ?? []);
	const cap = truncate(arrivals, 10);
	const stopId = input?.stop_id ?? d.stop_id ?? null;
	const stopName = d.stop_name ?? null;
	const stopMeta =
		result?.stop_meta ?? (d.lat != null && d.lon != null ? { lat: d.lat, lon: d.lon } : null);
	return {
		payload: {
			stop_id: stopId,
			stop_name: stopName,
			routes: (d.routes ?? []).slice(0, 20).map((r) => ({
				id: r.id ?? null,
				short_name: r.short_name ?? null,
			})),
			next_arrivals: cap.items.map(compactArrivalRow),
			truncated: cap.truncated,
			map_rendered: true,
		},
		ledgerAdds: ledgerFromArrivals(cap.items, stopId, stopName, stopMeta),
	};
}

function compact_get_stop(input, result) {
	const d = unwrap(result);
	const stop = d ? compactStop(d) : null;
	const entry = stopLedgerEntry(d ? { ...d, id: d.id ?? input?.stop_id ?? stop?.id } : null);
	return {
		payload: stop,
		ledgerAdds: entry ? [entry] : [],
	};
}

function compact_search_stops(_input, result) {
	const rawStops = items(result);
	const list = rawStops.map(compactStop);
	const cap = truncate(list);
	const rawCap = rawStops.slice(0, cap.items.length);
	return {
		payload: { stops: cap.items, truncated: cap.truncated },
		ledgerAdds: rawCap.map(stopLedgerEntry).filter(Boolean),
	};
}

function compact_find_stops_near_location(input, result) {
	return compact_search_stops(input, result);
}

function compact_get_stops_for_agency(input, result) {
	return compact_search_stops(input, result);
}

function compact_get_route(_input, result) {
	const d = unwrap(result);
	const route = d ? compactRoute(d) : null;
	return {
		payload: route,
		ledgerAdds: route?.id
			? [{ kind: 'route', id: route.id, name: route.short_name ?? route.long_name ?? route.id }]
			: [],
	};
}

function compact_search_routes(_input, result) {
	const list = items(result).map(compactRoute);
	const cap = truncate(list);
	return {
		payload: { routes: cap.items, truncated: cap.truncated },
		ledgerAdds: cap.items
			.filter((r) => r.id)
			.map((r) => ({ kind: 'route', id: r.id, name: r.short_name ?? r.long_name ?? r.id })),
	};
}

function compact_get_routes_for_agency(input, result) {
	return compact_search_routes(input, result);
}

function compact_get_routes_for_location(input, result) {
	return compact_search_routes(input, result);
}

function compact_get_stops_for_route(input, result) {
	const d = unwrap(result) ?? {};
	const routeId = input?.route_id ?? null;
	const directions = (d.directions ?? []).slice(0, 4).map((dir) => ({
		direction: dir.direction ?? null,
		stop_count: Array.isArray(dir.stops) ? dir.stops.length : 0,
	}));
	return {
		payload: {
			route_id: routeId,
			directions,
			map_rendered: true,
		},
		ledgerAdds: routeId ? [{ kind: 'route', id: routeId, name: routeId }] : [],
	};
}

function compact_get_trips_for_route(input, result) {
	const trips = items(result);
	const cap = truncate(trips);
	const routeId = input?.route_id ?? null;
	return {
		payload: {
			route_id: routeId,
			active_trip_count: cap.items.length,
			trips: cap.items.map((t) => ({
				trip_id: t.trip_id ?? null,
				vehicle_id: t.vehicle_id ?? null,
				headsign: t.headsign ?? null,
				predicted: !!t.predicted,
				stops_away: t.number_of_stops_away ?? null,
			})),
			truncated: cap.truncated,
			map_rendered: true,
		},
		ledgerAdds: routeId ? [{ kind: 'route', id: routeId, name: routeId }] : [],
	};
}

function compact_get_trip_details(_input, result) {
	const d = unwrap(result) ?? {};
	return {
		payload: {
			trip_id: d.trip_id ?? null,
			route_id: d.route_id ?? null,
			active_trip_id: d.active_trip_id ?? null,
			active_route_id: d.active_route_id ?? null,
			headsign: d.headsign ?? null,
			predicted: !!d.predicted,
			stops_away: d.number_of_stops_away ?? null,
			map_rendered: true,
		},
		ledgerAdds: d.route_id ? [{ kind: 'route', id: d.route_id, name: d.route_id }] : [],
	};
}

function compact_get_trip_for_vehicle(input, result) {
	return compact_get_trip_details(input, result);
}

function compact_get_vehicles_for_agency(input, result) {
	const list = items(result);
	const cap = truncate(list, 30);
	return {
		payload: {
			agency_id: input?.agency_id ?? null,
			vehicle_count: cap.items.length,
			truncated: cap.truncated,
		},
		ledgerAdds: input?.agency_id
			? [{ kind: 'agency', id: input.agency_id, name: input.agency_id }]
			: [],
	};
}

function compact_get_agency(_input, result) {
	const d = unwrap(result);
	const agency = d ? compactAgency(d) : null;
	return {
		payload: agency,
		ledgerAdds: agency?.id
			? [{ kind: 'agency', id: agency.id, name: agency.name ?? agency.id }]
			: [],
	};
}

function compact_get_agencies(_input, result) {
	const list = items(result).map(compactAgency);
	const cap = truncate(list);
	return {
		payload: { agencies: cap.items, truncated: cap.truncated },
		ledgerAdds: cap.items
			.filter((a) => a.id)
			.map((a) => ({ kind: 'agency', id: a.id, name: a.name ?? a.id })),
	};
}

function compact_get_current_time(_input, result) {
	const d = unwrap(result) ?? {};
	return {
		payload: { time_ms: d.time_ms ?? d.time ?? null, iso: d.iso ?? null },
		ledgerAdds: [],
	};
}

const COMPACTORS = {
	get_arrivals_for_stop: compact_get_arrivals_for_stop,
	get_arrival_and_departure_for_stop: compact_get_arrival_and_departure_for_stop,
	get_arrivals_for_location: compact_get_arrivals_for_location,
	get_stop_overview: compact_get_stop_overview,
	get_stop: compact_get_stop,
	search_stops: compact_search_stops,
	find_stops_near_location: compact_find_stops_near_location,
	get_stops_for_agency: compact_get_stops_for_agency,
	get_route: compact_get_route,
	search_routes: compact_search_routes,
	get_routes_for_agency: compact_get_routes_for_agency,
	get_routes_for_location: compact_get_routes_for_location,
	get_stops_for_route: compact_get_stops_for_route,
	get_trips_for_route: compact_get_trips_for_route,
	get_trip_details: compact_get_trip_details,
	get_trip_for_vehicle: compact_get_trip_for_vehicle,
	get_vehicles_for_agency: compact_get_vehicles_for_agency,
	get_agency: compact_get_agency,
	get_agencies: compact_get_agencies,
	get_current_time: compact_get_current_time,
};

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Compact a raw tool dispatch result into { payload, ledgerAdds }.
 * Falls back to a size-capped generic summary for tools without a bespoke
 * compactor, and passes through error results untouched so the model still
 * sees the error contract.
 */
export function compactToolResult(name, input, fullResult) {
	if (fullResult && typeof fullResult === 'object' && 'error' in fullResult) {
		return { payload: fullResult, ledgerAdds: [] };
	}
	if (fullResult && typeof fullResult === 'object' && fullResult.clarification_required) {
		return { payload: fullResult, ledgerAdds: [] };
	}
	const compactor = COMPACTORS[name];
	if (compactor) return compactor(input ?? {}, fullResult);
	const d = unwrap(fullResult);
	const list = items(fullResult);
	if (list.length) {
		const cap = truncate(list);
		return { payload: { items: cap.items, truncated: cap.truncated }, ledgerAdds: [] };
	}
	return { payload: d ?? null, ledgerAdds: [] };
}

/**
 * Merge new entries into a ledger, keeping the newest name per identifier.
 * Stop entries carry optional lat/lon so future "stops near <known stop_id>"
 * turns can skip re-fetching coordinates.
 */
export function mergeLedger(ledger, adds) {
	const next = {
		stops: normalizeStopBucket(ledger?.stops),
		routes: { ...(ledger?.routes ?? {}) },
		agencies: { ...(ledger?.agencies ?? {}) },
	};
	for (const entry of adds ?? []) {
		if (!entry?.id) continue;
		if (entry.kind === 'stop') {
			const prev = next.stops[entry.id] ?? {};
			next.stops[entry.id] = {
				name: entry.name ?? prev.name ?? entry.id,
				lat: entry.lat ?? prev.lat,
				lon: entry.lon ?? prev.lon,
			};
		} else {
			const bucket = next[entry.kind + 's'];
			if (bucket) bucket[entry.id] = entry.name ?? entry.id;
		}
	}
	return next;
}

// Tolerate the legacy `{id: name}` shape from older clients.
function normalizeStopBucket(bucket) {
	const out = {};
	for (const [id, value] of Object.entries(bucket ?? {})) {
		out[id] = typeof value === 'string' ? { name: value } : { ...value };
	}
	return out;
}

const LEDGER_LIMIT = 30;

/** Render the session ledger as a short block for the system prompt. */
export function formatLedgerForSystem(ledger) {
	if (!ledger) return '';
	const stops = Object.entries(ledger.stops ?? {}).slice(-LEDGER_LIMIT);
	const routes = Object.entries(ledger.routes ?? {}).slice(-LEDGER_LIMIT);
	const agencies = Object.entries(ledger.agencies ?? {}).slice(-LEDGER_LIMIT);
	if (!stops.length && !routes.length && !agencies.length) return '';
	const lines = ['Known identifiers from this session (reuse these instead of re-fetching):'];
	if (stops.length)
		lines.push(`- Stops: ${stops.map(([id, value]) => formatStopEntry(id, value)).join(', ')}`);
	if (routes.length)
		lines.push(`- Routes: ${routes.map(([id, name]) => `${id} "${name}"`).join(', ')}`);
	if (agencies.length)
		lines.push(`- Agencies: ${agencies.map(([id, name]) => `${id} "${name}"`).join(', ')}`);
	return lines.join('\n');
}

function formatStopEntry(id, value) {
	if (typeof value === 'string') return `${id} "${value}"`;
	const base = `${id} "${value.name ?? id}"`;
	return value.lat != null && value.lon != null
		? `${base} @(${value.lat.toFixed(5)},${value.lon.toFixed(5)})`
		: base;
}

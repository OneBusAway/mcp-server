/**
 * Global arrival tracking store.
 * Polls `get_arrival_and_departure_for_stop` every 30s per tracked trip.
 * Polls `get_arrivals_for_stop` every 30s per tracked stop (live panel + map refresh).
 * Sends a browser notification when the bus is 1 stop away or arriving.
 * Never involves the LLM — all updates are direct MCP calls.
 */

import { browser } from '$app/environment';
import { callTool } from '$lib/mcp.js';
import { unwrap, items, normalizeArrival, normalizeArrivals } from '$lib/result.js';

async function requestNotifPermission() {
	if (!browser || !('Notification' in window)) return false;
	if (Notification.permission === 'granted') return true;
	if (Notification.permission === 'denied') return false;
	return (await Notification.requestPermission()) === 'granted';
}

function sendBrowserNotif(title, body) {
	if (!browser || !('Notification' in window) || Notification.permission !== 'granted') return;
	try {
		new Notification(title, { body, icon: '/favicon.svg', tag: 'oba-tracker' });
	} catch {}
}

function createTrackingStore() {
	/** @type {Array<{
	 *   id: string, key: string, intervalId: number,
	 *   stop_id: string, stop_name: string,
	 *   trip_id: string, service_date: number,
	 *   route_name: string, headsign: string,
	 *   stops_away: number | null, predicted_arrival: string,
	 *   status: 'tracking' | 'approaching' | 'arriving' | 'passed' | 'done',
	 *   notified_approach: boolean, notified_arrival: boolean
	 * }>} */
	let trackers = $state([]);

	/** @type {Record<string, any[]>} stop_id → latest arrivals array */
	let stopArrivals = $state({});

	/** @type {Record<string, number>} stop_id → interval ID */
	const _stopIntervals = {};
	// Keep an arrival visible long enough to receive its at-stop status. Without
	// this window, OBA can drop a vehicle as soon as it reaches the stop.
	const TRACKING_MINUTES_BEFORE = 10;

	function _update(id, patch) {
		trackers = trackers.map(t => (t.id === id ? { ...t, ...patch } : t));
	}

	function _markArrived(tracker, arrival, passed = false) {
		if (!tracker.notified_arrival) {
			sendBrowserNotif(
				`🚌 Route ${tracker.route_name} ${passed ? 'has passed' : 'arriving!'}`,
				`${tracker.headsign || 'Your vehicle'} ${passed ? `has passed ${tracker.stop_name}` : `is at ${tracker.stop_name}`}`
			);
		}
		_update(tracker.id, {
			stops_away: 0,
			status: passed ? 'passed' : 'arriving',
			predicted_arrival: arrival,
			notified_arrival: true,
		});
		if (!tracker.notified_arrival) setTimeout(() => remove(tracker.id), 90_000);
	}

	async function _poll(trackerId) {
		const tracker = trackers.find(t => t.id === trackerId);
		if (!tracker || tracker.status === 'done') return;

		try {
			const result = await callTool('get_arrival_and_departure_for_stop', {
				stop_id: tracker.stop_id,
				trip_id: tracker.trip_id,
				service_date: tracker.service_date,
			});

			// Phase-4: "not found" comes as a success envelope with null data.
			// Legacy: a plain string starting with "No arrival found".
			const raw = unwrap(result);
			if (!raw || !raw.trip_id || (typeof raw === 'string' && /^No arrival found/i.test(raw))) {
				_markArrived(tracker, tracker.predicted_arrival, true);
				return;
			}

			const data = normalizeArrival(raw);
			const stopsAway = data?.number_of_stops_away;
			const arrival = data?.predicted_arrival || data?.scheduled_arrival || tracker.predicted_arrival;

			if (typeof stopsAway !== 'number') return;

			if (stopsAway === 0) {
				_markArrived(tracker, arrival);
			} else if (stopsAway === 1 && !tracker.notified_approach) {
				sendBrowserNotif(
					`🚌 Route ${tracker.route_name} — 1 stop away`,
					`${tracker.headsign || ''} approaching ${tracker.stop_name}`
				);
				_update(trackerId, { stops_away: 1, status: 'approaching', notified_approach: true, predicted_arrival: arrival });
			} else {
				_update(trackerId, { stops_away: stopsAway, predicted_arrival: arrival });
			}
		} catch {}
	}

	/** Fetch full arrivals for a stop and expose them reactively for live panel/map updates. */
	async function _pollStop(stop_id) {
		try {
			const result = await callTool('get_arrivals_for_stop', {
				stop_id,
				minutes_before: TRACKING_MINUTES_BEFORE,
				minutes_after: 60,
			});
			const arrivals = normalizeArrivals(items(result));
			stopArrivals = { ...stopArrivals, [stop_id]: arrivals };
		} catch {}
	}

	/**
	 * Add tracking for predicted arrivals at a stop.
	 * @param {{ stop_id: string, stop_name: string, arrivals: any[] }} opts
	 */
	async function add({ stop_id, stop_name, arrivals }) {
		await requestNotifPermission();

		const eligible = arrivals
			.map(normalizeArrival)
			.filter(a => a.predicted && a.trip_id && a.service_date);
		if (!eligible.length) return 0;

		let added = 0;
		for (const a of eligible) {
			const key = `${stop_id}__${a.trip_id}`;
			if (trackers.some(t => t.key === key)) continue;

			const id = crypto.randomUUID();
			const intervalId = setInterval(() => _poll(id), 30_000);

			trackers = [
				...trackers,
				{
					id, key, intervalId,
					stop_id, stop_name,
					trip_id: a.trip_id,
					service_date: a.service_date,
					route_name: a.route_name,
					headsign: a.headsign || '',
					stops_away: a.number_of_stops_away ?? null,
					predicted_arrival: a.predicted_arrival || a.scheduled_arrival || '',
					status: 'tracking',
					notified_approach: false,
					notified_arrival: false,
				},
			];
			added++;
			_poll(id); // immediate poll — don't wait 30s for first update
		}

		// Start stop-level refresh if this is the first tracker for this stop
		if (added > 0 && !_stopIntervals[stop_id]) {
			_stopIntervals[stop_id] = setInterval(() => _pollStop(stop_id), 30_000);
			_pollStop(stop_id); // immediate — refresh arrivals panel and map right away
		}

		return added;
	}

	function remove(trackerId) {
		const t = trackers.find(x => x.id === trackerId);
		if (t) {
			clearInterval(t.intervalId);
			trackers = trackers.filter(x => x.id !== trackerId);
			// Stop stop-level refresh when no more trackers remain for this stop
			if (!trackers.some(x => x.stop_id === t.stop_id)) {
				clearInterval(_stopIntervals[t.stop_id]);
				delete _stopIntervals[t.stop_id];
				const next = { ...stopArrivals };
				delete next[t.stop_id];
				stopArrivals = next;
			}
		}
	}

	function clear() {
		for (const t of trackers) clearInterval(t.intervalId);
		for (const id of Object.values(_stopIntervals)) clearInterval(id);
		Object.keys(_stopIntervals).forEach(k => delete _stopIntervals[k]);
		trackers = [];
		stopArrivals = {};
	}

	return {
		get trackers() { return trackers; },
		/** Reactive map of stop_id → latest arrivals (populated when tracking is active). */
		get stopArrivals() { return stopArrivals; },
		add,
		remove,
		clear,
	};
}

export const tracking = createTrackingStore();

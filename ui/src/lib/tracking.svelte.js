/**
 * Global arrival tracking store.
 * Polls `get_arrivals_for_stop` every 15s per tracked stop (status, panel, and map).
 * Sends a browser notification when the bus is 1 stop away or arriving.
 * Never involves the LLM — all updates are direct MCP calls.
 */

import { browser } from '$app/environment';
import { callTool } from '$lib/mcp.js';
import { items, normalizeArrival, normalizeArrivals } from '$lib/result.js';

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

// Synthesized two-tone chime — avoids shipping an audio file and works offline.
// AudioContext is created lazily since browsers block it before the first user gesture,
// but by the time a tracker fires the user has already clicked "Track" so it resumes fine.
// Ascending A5→E6 for arrival, descending E6→A5 for departure so the two are distinguishable by ear.
function playChime(departing = false) {
	if (!browser) return;
	try {
		const AC = window.AudioContext || window.webkitAudioContext;
		if (!AC) return;
		const ctx = new AC();
		const now = ctx.currentTime;
		const notes = departing
			? [
				{ freq: 1318.5, at: 0.00, dur: 0.20 },  // E6
				{ freq: 880.0,  at: 0.18, dur: 0.32 },  // A5
			]
			: [
				{ freq: 880.0,  at: 0.00, dur: 0.20 },  // A5
				{ freq: 1318.5, at: 0.18, dur: 0.32 },  // E6
			];
		for (const n of notes) {
			const osc = ctx.createOscillator();
			const gain = ctx.createGain();
			osc.type = 'sine';
			osc.frequency.value = n.freq;
			gain.gain.setValueAtTime(0.0001, now + n.at);
			gain.gain.exponentialRampToValueAtTime(0.25, now + n.at + 0.02);
			gain.gain.exponentialRampToValueAtTime(0.0001, now + n.at + n.dur);
			osc.connect(gain).connect(ctx.destination);
			osc.start(now + n.at);
			osc.stop(now + n.at + n.dur + 0.02);
		}
		setTimeout(() => ctx.close?.(), 1000);
	} catch {}
}

function createTrackingStore() {
	/** @type {Array<{
	 *   id: string, key: string,
	 *   stop_id: string, stop_name: string,
	 *   trip_id: string, service_date: number,
	 *   route_name: string, headsign: string,
	 *   stops_away: number | null, predicted_arrival: string,
	 *   arrival_ms: number, departure_ms: number,
	 *   status: 'tracking' | 'approaching' | 'arriving' | 'passed' | 'done',
	 *   notified_approach: boolean, notified_arrival: boolean, notified_departure: boolean,
	 *   passed_at: number | null, deviation_seconds: number | null, deviation_label: string | null
	 * }>} */
	let trackers = $state([]);

	/** @type {Record<string, any[]>} stop_id → latest arrivals array */
	let stopArrivals = $state({});

	/** @type {Record<string, number>} stop_id → interval ID */
	const _stopIntervals = {};
	const POLL_INTERVAL_MS = 15_000;
	// Keep an arrival visible long enough to receive its at-stop status. Without
	// this window, OBA can drop a vehicle as soon as it reaches the stop.
	const TRACKING_MINUTES_BEFORE = 15;

	function _update(id, patch) {
		trackers = trackers.map(t => (t.id === id ? { ...t, ...patch } : t));
	}

	function _markArrived(tracker, arrival, passed = false) {
		tracker = trackers.find((item) => item.id === tracker.id) ?? tracker;
		// Separate flags so a bus that we saw arrive AND then leave chimes twice —
		// once on arrival, once on departure.
		const flag = passed ? 'notified_departure' : 'notified_arrival';
		if (!tracker[flag]) {
			sendBrowserNotif(
				`🚌 Route ${tracker.route_name} ${passed ? 'has passed' : 'arriving!'}`,
				`${tracker.headsign || 'Your vehicle'} ${passed ? `has passed ${tracker.stop_name}` : `is at ${tracker.stop_name}`}`
			);
			playChime(passed);
		}
		_update(tracker.id, {
			stops_away: 0,
			status: passed ? 'passed' : 'arriving',
			predicted_arrival: arrival,
			[flag]: true,
			// Record when the bus first passed so _poll can expire the tracker after
			// the arrivals lookback window — no immediate auto-remove.
			...(passed && tracker.passed_at == null ? { passed_at: Date.now() } : {}),
		});
	}

	function _arrivalTimes(data, tracker) {
		return {
			arrivalMs: (data?.predicted_arrival_ms > 0 ? data.predicted_arrival_ms : 0)
				|| (data?.scheduled_arrival_ms > 0 ? data.scheduled_arrival_ms : 0)
				|| tracker.arrival_ms || 0,
			departureMs: (data?.predicted_departure_ms > 0 ? data.predicted_departure_ms : 0)
				|| (data?.scheduled_departure_ms > 0 ? data.scheduled_departure_ms : 0)
				|| tracker.departure_ms || 0,
		};
	}

	// Apply one arrival response to a tracker. number_of_stops_away is optional
	// in OBA, so timestamps and distance provide the arrival/pass fallback.
	function _applyArrivalUpdate(tracker, data) {
		tracker = trackers.find((item) => item.id === tracker.id) ?? tracker;
		const now = Date.now();
		const stopsAway = data?.number_of_stops_away;
		const distance = Number(data?.distance_from_stop_meters);
		const arrival = data?.predicted_arrival || data?.scheduled_arrival || tracker.predicted_arrival;
		const { arrivalMs, departureMs } = _arrivalTimes(data, tracker);
		const passedByTime = departureMs > 0
			? now > departureMs + 30_000
			: arrivalMs > 0 && now > arrivalMs + 2 * 60_000;
		if (passedByTime) {
			_markArrived(tracker, arrival, true);
			return;
		}

		const insideArrivalWindow = arrivalMs > 0
			&& now >= arrivalMs - 60_000
			&& (departureMs <= 0 || now <= departureMs + 30_000);
		const atStopByDistance = Number.isFinite(distance) && distance >= 0 && distance <= 50
			&& (arrivalMs <= 0 || now >= arrivalMs - 2 * 60_000);
		if (stopsAway === 0 || insideArrivalWindow || atStopByDistance) {
			_markArrived(tracker, arrival);
			_update(tracker.id, { arrival_ms: arrivalMs, departure_ms: departureMs });
			return;
		}

		const approachingByTime = arrivalMs > now && arrivalMs-now <= 5 * 60_000;
		const approachingByDistance = Number.isFinite(distance) && distance > 50 && distance <= 300;
		const approaching = stopsAway === 1 || approachingByTime || approachingByDistance;
		if (approaching && !tracker.notified_approach) {
			sendBrowserNotif(
				`🚌 Route ${tracker.route_name} — approaching`,
				`${tracker.headsign || ''} approaching ${tracker.stop_name}`
			);
		}
		_update(tracker.id, {
			stops_away: typeof stopsAway === 'number' ? stopsAway : tracker.stops_away,
			predicted_arrival: arrival,
			arrival_ms: arrivalMs,
			departure_ms: departureMs,
			status: approaching ? 'approaching' : 'tracking',
			notified_approach: tracker.notified_approach || approaching,
			deviation_seconds: data?.deviation_seconds ?? tracker.deviation_seconds,
			deviation_label: data?.deviation_label ?? tracker.deviation_label,
		});
	}

	/** Fetch full arrivals for a stop and expose them reactively for live panel/map updates. */
	async function _pollStop(stop_id) {
		const prev = stopArrivals[stop_id] ?? [];
		const now = Date.now();
		for (const tracker of trackers.filter((item) => item.stop_id === stop_id)) {
			if (tracker.status === 'passed' && tracker.passed_at != null && now - tracker.passed_at >= TRACKING_MINUTES_BEFORE * 60_000) {
				remove(tracker.id);
			}
		}
		if (!trackers.some((item) => item.stop_id === stop_id)) return;
		try {
			const result = await callTool('get_arrivals_for_stop', {
				stop_id,
				minutes_before: TRACKING_MINUTES_BEFORE,
				minutes_after: 60,
				limit: 50,
			});
			const fresh = normalizeArrivals(items(result));
			const freshIds = new Set(fresh.map(a => a.trip_id));
			// Keep arrivals from previous response for 1 extra cycle to prevent flicker.
			// Only carry over non-ghost trips with a future arrival time.
			const ghosts = prev
				.filter(a => !a._ghost && !freshIds.has(a.trip_id))
				.filter(a => (a.predicted_arrival_ms || a.scheduled_arrival_ms || 0) > now)
				.map(a => ({ ...a, _ghost: true }));
			stopArrivals = { ...stopArrivals, [stop_id]: [...fresh, ...ghosts] };

			// One stop-level response updates every tracker for this stop, including
			// current positions for recently departed trips retained by minutes_before.
			for (const tracker of trackers.filter((item) => item.stop_id === stop_id)) {
				const arrival = fresh.find((item) => item.trip_id === tracker.trip_id);
				if (arrival) {
					_applyArrivalUpdate(tracker, arrival);
				} else if (tracker.arrival_ms > 0 && now > tracker.arrival_ms + 2 * 60_000) {
					// Once an expected trip disappears after its arrival window, treat the
					// missing row as a departure signal.
					_markArrived(tracker, tracker.predicted_arrival, true);
				}
			}
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

			trackers = [
				...trackers,
				{
					id, key,
					stop_id, stop_name,
					trip_id: a.trip_id,
					service_date: a.service_date,
					route_name: a.route_name,
					headsign: a.headsign || '',
					stops_away: a.number_of_stops_away ?? null,
					predicted_arrival: a.predicted_arrival || a.scheduled_arrival || '',
					arrival_ms: (a.predicted_arrival_ms > 0 ? a.predicted_arrival_ms : a.scheduled_arrival_ms) || 0,
					departure_ms: (a.predicted_departure_ms > 0 ? a.predicted_departure_ms : a.scheduled_departure_ms) || 0,
					status: 'tracking',
					notified_approach: false,
					notified_arrival: false,
					notified_departure: false,
					passed_at: null,
					deviation_seconds: a.deviation_seconds ?? null,
					deviation_label: a.deviation_label ?? null,
				},
			];
			added++;
		}

		// Start stop-level refresh if this is the first tracker for this stop
		if (added > 0 && !_stopIntervals[stop_id]) {
			_stopIntervals[stop_id] = setInterval(() => _pollStop(stop_id), POLL_INTERVAL_MS);
			_pollStop(stop_id); // immediate — refresh arrivals panel and map right away
		}

		return added;
	}

	function remove(trackerId) {
		const t = trackers.find(x => x.id === trackerId);
		if (t) {
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

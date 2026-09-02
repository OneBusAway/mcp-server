<script>
	import { callTool } from '$lib/mcp.js';
	import { items, normalizeArrivals } from '$lib/result.js';
	import { tracking } from '$lib/tracking.svelte.js';
	import ArrivalRow from './ArrivalRow.svelte';
	import BusLoader from './BusLoader.svelte';
	import Icon from './Icon.svelte';

	/** @type {{ stopId?: string | null, stopName?: string, onClose?: () => void, initialArrivals?: any[] }} */
	let { stopId = null, stopName = '', onClose = null, initialArrivals = [] } = $props();

	let arrivals = $state([]);
	let loading = $state(false);
	let error = $state('');
	let lastAt = $state(null);
	let lastInitialArrivals = null;
	const MINUTES_BEFORE = 15;
	const stopLabel = $derived(
		stopName && stopId && stopName !== stopId
			? `${stopName} · ${stopId}`
			: stopName || (stopId ? `Stop ${stopId}` : 'Select a stop'),
	);

	function ensureTrackedInArrivals(nextArrivals) {
		const tracked = tracking.trackers.filter((item) => item.stop_id === stopId);
		const result = [...nextArrivals];
		for (const tracker of tracked) {
			const idx = result.findIndex((arrival) => arrival.trip_id === tracker.trip_id);
			if (idx !== -1) {
				// Overlay the tracking state so the row keeps its arrival/departure
				// status across stop-feed refreshes.
				const existing = result[idx];
				result[idx] = {
					...existing,
					predicted: existing.predicted || Boolean(tracker.predicted_arrival),
					predicted_arrival: tracker.predicted_arrival || existing.predicted_arrival,
					predicted_arrival_display:
						tracker.predicted_arrival || existing.predicted_arrival_display,
					number_of_stops_away: tracker.stops_away ?? existing.number_of_stops_away,
					deviation_seconds: tracker.deviation_seconds ?? existing.deviation_seconds ?? 0,
					deviation_label: tracker.deviation_label ?? existing.deviation_label ?? null,
				};
				continue;
			}
			// OBA can briefly omit a tracked trip between updates. Keep a lightweight
			// row so refreshing the panel never makes the user's tracker disappear.
			result.push({
				trip_id: tracker.trip_id,
				service_date: tracker.service_date,
				service_date_ms: tracker.service_date,
				route_name: tracker.route_name,
				route_short_name: tracker.route_name,
				headsign: tracker.headsign,
				predicted: true,
				predicted_arrival: tracker.predicted_arrival,
				predicted_arrival_display: tracker.predicted_arrival,
				scheduled_arrival: tracker.predicted_arrival,
				scheduled_arrival_display: tracker.predicted_arrival,
				number_of_stops_away: tracker.stops_away,
				deviation_seconds: tracker.deviation_seconds ?? 0,
				deviation_label: tracker.deviation_label ?? null,
			});
		}
		return result;
	}

	// This derives from both the stop feed and tracker state, so status changes
	// update the panel immediately after each stop-level poll.
	const displayArrivals = $derived(ensureTrackedInArrivals(arrivals));
	const hasRealtimeData = $derived(displayArrivals.some((arrival) => arrival.predicted));
	const trackedArrivals = $derived(displayArrivals.filter((a) => !!trackedArrival(a)));
	const nonTrackedArrivals = $derived(
		displayArrivals.filter((a) => !trackedArrival(a)).slice(0, 10),
	);

	async function load() {
		if (!stopId) return;
		loading = true;
		error = '';
		try {
			const result = await callTool('get_arrivals_for_stop', {
				stop_id: stopId,
				minutes_before: MINUTES_BEFORE,
				minutes_after: 60,
				limit: 50,
			});
			arrivals = ensureTrackedInArrivals(normalizeArrivals(items(result)));
			lastAt = new Date();
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	function trackedArrival(arrival) {
		return tracking.trackers.find(
			(tracker) => tracker.stop_id === stopId && tracker.trip_id === arrival.trip_id,
		);
	}

	// Arrival cards stream their data in after mounting. Sync only when the
	// input list itself changes, leaving subsequent tracker polls in control.
	$effect(() => {
		const incoming = initialArrivals;
		if (incoming === lastInitialArrivals) return;
		lastInitialArrivals = incoming;
		arrivals = [...incoming];
		lastAt = incoming.length ? new Date() : null;
	});

	async function toggleArrivalTracking(arrival) {
		const existing = trackedArrival(arrival);
		if (existing) {
			tracking.remove(existing.id);
			return;
		}

		let trackable = arrival;
		if (!trackable.service_date && !trackable.service_date_ms) {
			try {
				const result = await callTool('get_arrivals_for_stop', {
					stop_id: stopId,
					minutes_before: MINUTES_BEFORE,
					minutes_after: 60,
					limit: 50,
				});
				const refreshed = normalizeArrivals(items(result));
				trackable = refreshed.find((item) => item.trip_id === arrival.trip_id) ?? arrival;
			} catch {}
		}
		await tracking.add({ stop_id: stopId, stop_name: stopName, arrivals: [trackable] });
	}

	// Auto-load when stopId changes — skip when initialArrivals already provided
	$effect(() => {
		const sid = stopId;
		if (!sid || initialArrivals.length > 0) return;
		arrivals = [];
		load();
	});

	// Live updates from tracking store (fires when tracking polls this stop)
	$effect(() => {
		if (!stopId) return;
		const liveData = tracking.stopArrivals[stopId];
		if (Array.isArray(liveData) && liveData.length) {
			arrivals = ensureTrackedInArrivals(liveData);
			lastAt = new Date();
		}
	});

	// Periodic auto-refresh for live panels only. Chat history cards (those
	// mounted with initialArrivals) skip this interval — they receive updates
	// via tracking.stopArrivals when a trip is being tracked, and don't need
	// their own ongoing MCP requests.
	$effect(() => {
		if (!stopId || initialArrivals.length > 0) return;
		const id = setInterval(() => load(), 30_000);
		return () => clearInterval(id);
	});
</script>

<section class="flex h-full flex-col bg-white dark:bg-zinc-900">
	<div class="flex items-center gap-2 border-b border-zinc-200 px-3 py-2.5 dark:border-zinc-800">
		<div class="min-w-0 flex-1">
			<h2 class="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">
				{stopLabel}
			</h2>
			{#if lastAt}
				<p class="text-xs text-zinc-400 dark:text-zinc-500">
					Updated {lastAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
				</p>
			{/if}
			{#if arrivals.length && !hasRealtimeData}
				<p class="text-xs text-amber-700 dark:text-amber-400">No real-time data available</p>
			{/if}
		</div>
		<div class="flex shrink-0 items-center gap-1">
			{#if stopId}
				<button
					type="button"
					onclick={load}
					disabled={loading}
					class="rounded-full p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
					title="Refresh"
				>
					<Icon name="refresh-cw" cls="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
				</button>
			{/if}
			{#if onClose}
				<button
					type="button"
					onclick={onClose}
					class="rounded-full p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
					title="Close"
				>
					<svg
						xmlns="http://www.w3.org/2000/svg"
						width="13"
						height="13"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2.5"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
					</svg>
				</button>
			{/if}
		</div>
	</div>

	<div class="flex-1 overflow-y-auto text-zinc-900 dark:text-zinc-100">
		{#if !stopId}
			<div class="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
				<Icon name="map-pin" cls="h-8 w-8 text-zinc-300 dark:text-zinc-500" />
				<p class="text-sm text-zinc-500 dark:text-zinc-400">
					Click a stop on the map to see arrivals
				</p>
			</div>
		{:else if loading && arrivals.length === 0}
			<BusLoader label="Fetching arrivals…" size="sm" />
		{:else if error}
			<div class="p-4">
				<p
					class="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-400"
				>
					<Icon name="alert-triangle" cls="h-4 w-4 shrink-0" />
					{error}
				</p>
			</div>
		{:else if arrivals.length === 0}
			<div class="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
				<Icon name="clock" cls="h-8 w-8 text-zinc-300 dark:text-zinc-500" />
				<p class="text-sm text-zinc-500 dark:text-zinc-400">
					No upcoming arrivals in the next 60 min
				</p>
			</div>
		{:else}
			<div class="divide-y divide-zinc-100 dark:divide-zinc-800/60">
				{#if trackedArrivals.length > 0}
					<div
						class="flex items-center gap-1.5 border-b border-amber-100 bg-amber-50 px-3 py-1 dark:border-amber-500/10 dark:bg-amber-500/5"
					>
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="11"
							height="11"
							viewBox="0 0 24 24"
							fill="none"
							stroke="#f59e0b"
							stroke-width="2.5"
							stroke-linecap="round"
							stroke-linejoin="round"
							><circle cx="12" cy="12" r="10" /><circle cx="12" cy="12" r="3" /><line
								x1="12"
								y1="2"
								x2="12"
								y2="5"
							/><line x1="12" y1="19" x2="12" y2="22" /><line x1="2" y1="12" x2="5" y2="12" /><line
								x1="19"
								y1="12"
								x2="22"
								y2="12"
							/></svg
						>
						<span class="text-xs font-semibold text-amber-700 dark:text-amber-400">Tracking</span>
					</div>
					{#each trackedArrivals as arrival (arrival.trip_id)}
						<ArrivalRow
							{arrival}
							trackingActive={true}
							onToggleTracking={() => toggleArrivalTracking(arrival)}
						/>
					{/each}
					{#if nonTrackedArrivals.length > 0}
						<div class="px-3 py-1 text-xs text-zinc-400 dark:text-zinc-500">Other arrivals</div>
					{/if}
				{/if}
				{#each nonTrackedArrivals as arrival (arrival.trip_id ?? arrival.route_short_name + arrival.scheduled_arrival)}
					<ArrivalRow
						{arrival}
						trackingActive={false}
						onToggleTracking={() => toggleArrivalTracking(arrival)}
					/>
				{/each}
			</div>
		{/if}
	</div>
</section>

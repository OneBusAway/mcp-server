<script>
	import { untrack } from 'svelte';
	import { callTool } from '$lib/mcp.js';
	import { items, normalizeArrivals } from '$lib/result.js';
	import { tracking } from '$lib/tracking.svelte.js';
	import { shouldPollArrivalsPanel } from '$lib/tracking-request.js';
	import ArrivalRow from './ArrivalRow.svelte';
	import BusLoader from './BusLoader.svelte';
	import Icon from './Icon.svelte';

	/** @type {{ stopId?: string | null, stopName?: string, onClose?: () => void, initialArrivals?: any[], minutesBefore?: number, minutesAfter?: number }} */
	let {
		stopId = null,
		stopName = '',
		onClose = null,
		initialArrivals = [],
		minutesBefore = 0,
		minutesAfter = 60,
	} = $props();

	let arrivals = $state([]);
	let loading = $state(false);
	let error = $state('');
	let lastAt = $state(null);
	let lastInitialArrivals = null;
	const stopTitle = $derived(stopName || (stopId ? `Stop ${stopId}` : 'Select a stop'));
	const hasStopTracker = $derived(
		tracking.trackers.some((tracker) => tracker.stop_id === stopId),
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
	const arrivalRouteColors = $derived.by(() => {
		const palette = ['#e46f5b', '#9278d0', '#377fba', '#d18a2e', '#318a78', '#b5548d'];
		const result = new Map();
		for (const arrival of displayArrivals) {
			const route = arrival.route_name ?? arrival.route_short_name ?? '';
			if (!result.has(route)) result.set(route, palette[result.size % palette.length]);
		}
		return result;
	});

	function arrivalColor(arrival) {
		return (
			arrivalRouteColors.get(arrival.route_name ?? arrival.route_short_name ?? '') ?? '#78aa37'
		);
	}

	async function load() {
		if (!stopId) return;
		loading = true;
		error = '';
		try {
			const result = await callTool('get_arrivals_for_stop', {
				stop_id: stopId,
				minutes_before: minutesBefore,
				minutes_after: minutesAfter,
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

	function mergeTrackedUpdates(current, updates) {
		const trackedTripIds = new Set(
			tracking.trackers
				.filter((tracker) => tracker.stop_id === stopId)
				.map((tracker) => tracker.trip_id),
		);
		const result = [...current];
		for (const update of updates) {
			if (!update?.trip_id || !trackedTripIds.has(update.trip_id)) continue;
			const index = result.findIndex((arrival) => arrival.trip_id === update.trip_id);
			if (index === -1) result.push(update);
			else result[index] = { ...result[index], ...update };
		}
		return ensureTrackedInArrivals(result);
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
					minutes_before: minutesBefore,
					minutes_after: minutesAfter,
					limit: 50,
				});
				const refreshed = normalizeArrivals(items(result));
				trackable = refreshed.find((item) => item.trip_id === arrival.trip_id) ?? arrival;
			} catch {}
		}
		await tracking.add({
			stop_id: stopId,
			stop_name: stopName,
			arrivals: [trackable],
		});
	}

	// Auto-load when stopId changes — skip when initialArrivals already provided
	$effect(() => {
		const sid = stopId;
		if (!sid || initialArrivals.length > 0) return;
		arrivals = [];
		load();
	});

	// Live updates from the precise per-trip tracking requests.
	$effect(() => {
		if (!stopId) return;
		const liveData = tracking.stopArrivals[stopId];
		if (Array.isArray(liveData) && liveData.length) {
			arrivals = mergeTrackedUpdates(
				untrack(() => arrivals),
				liveData,
			);
			lastAt = new Date();
		}
	});

	// Refresh every stop panel, including chat cards initialized from streamed
	// data. Once tracking starts, precise per-trip polling owns the live row and
	// this broad stop query pauses so Track never reloads the full arrivals list.
	$effect(() => {
		if (!shouldPollArrivalsPanel({ stopId, hasStopTracker })) return;
		const id = setInterval(() => load(), 30_000);
		return () => clearInterval(id);
	});
</script>

<section class="flex h-full flex-col bg-white dark:bg-[#1a1d17]">
	<div
		class="flex items-center gap-2 border-b border-[#cedadf] bg-[#f7fafb] px-4 py-3 dark:border-[#34382f] dark:bg-[#20231d]"
	>
		<span
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 border-[#377fba]"
		>
			<span class="h-3 w-3 rounded-full bg-[#377fba]"></span>
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
				<h2
					class="oba-heading truncate text-[22px] leading-tight text-[#172126] dark:text-[#f0f2ed]"
				>
					{stopTitle}
				</h2>
				{#if stopId}
					<span class="text-xs text-[#7d8377] dark:text-[#858b80]">Stop #{stopId}</span>
				{/if}
			</div>
			{#if lastAt}
				<p class="text-xs text-[#7d8377] dark:text-[#858b80]">
					Updated {lastAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · {hasStopTracker
						? 'tracked row refreshes every 15 s'
						: 'refreshes every 30 s'}
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
					class="flex h-9 items-center justify-center gap-1.5 rounded-[6px] border border-[#cedadf] px-2.5 text-xs font-bold text-[#172126] transition hover:bg-[#eaf0f2] disabled:opacity-50 dark:border-[#34382f] dark:text-[#f0f2ed] dark:hover:bg-[#292d25]"
					title="Refresh"
				>
					<Icon name="refresh-cw" cls="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
					<span class="hidden sm:inline">Refresh</span>
				</button>
			{/if}
			{#if onClose}
				<button
					type="button"
					onclick={onClose}
					class="flex h-9 w-9 items-center justify-center rounded-[5px] text-[#7d8377] transition hover:bg-[#e8e9e5] hover:text-[#1d211a] dark:text-[#858b80] dark:hover:bg-[#292d25] dark:hover:text-[#f0f2ed]"
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

	<div class="flex-1 overflow-y-auto text-[#1d211a] dark:text-[#f0f2ed]">
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
			<div class="divide-y divide-[#e8e9e5] dark:divide-[#292d25]">
				{#if trackedArrivals.length > 0}
					<div
						class="flex items-center gap-2 border-b border-oba-200 bg-oba-50 px-5 py-2 dark:border-oba-800 dark:bg-oba-900/35"
					>
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="11"
							height="11"
							viewBox="0 0 24 24"
							fill="none"
							stroke="#628e29"
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
						<span
							class="text-[10.5px] font-bold uppercase tracking-[0.12em] text-oba-800 dark:text-oba-300"
							>Tracking</span
						>
					</div>
					<div class="bg-[#f7faf2] dark:bg-oba-900/20">
						{#each trackedArrivals as arrival (arrival.trip_id)}
							<ArrivalRow
								{arrival}
								routeColor={arrivalColor(arrival)}
								trackingActive={true}
								onToggleTracking={() => toggleArrivalTracking(arrival)}
							/>
						{/each}
					</div>
					{#if nonTrackedArrivals.length > 0}
						<div
							class="bg-[#eaf0f2] px-4 py-1.5 text-[10.5px] font-bold uppercase tracking-[0.1em] text-[#7b898f] dark:bg-[#20231d] dark:text-[#858b80]"
						>
							Other arrivals
						</div>
					{/if}
				{/if}
				{#each nonTrackedArrivals as arrival (arrival.trip_id ?? arrival.route_short_name + arrival.scheduled_arrival)}
					<ArrivalRow
						{arrival}
						routeColor={arrivalColor(arrival)}
						trackingActive={false}
						onToggleTracking={() => toggleArrivalTracking(arrival)}
					/>
				{/each}
			</div>
		{/if}
	</div>
	{#if stopId && arrivals.length > 0}
		<div
			class="flex shrink-0 items-center justify-between gap-3 border-t border-[#cedadf] bg-[#eaf0f2] px-5 py-2 text-xs text-[#64737a] dark:border-[#34382f] dark:bg-[#20231d] dark:text-[#a5ab9f]"
		>
			<div class="flex items-center gap-3">
				<span class="inline-flex items-center gap-1.5"
					><span class="h-2.5 w-2.5 rounded-full bg-[#36b99a]"></span>Live</span
				>
				<span class="inline-flex items-center gap-1.5"
					><span class="h-2.5 w-2.5 rounded-full border border-[#8d9389]"></span>Scheduled</span
				>
			</div>
			<span class="hidden sm:inline">OneBusAway live arrivals</span>
		</div>
	{/if}
</section>

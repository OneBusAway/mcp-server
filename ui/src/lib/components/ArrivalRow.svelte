<script>
	import StatusDot from './StatusDot.svelte';

	/**
	 * @typedef {{
	 *   route_short_name: string,
	 *   route_long_name?: string,
	 *   headsign?: string,
	 *   scheduled_arrival: string,
	 *   predicted_arrival?: string,
	 *   predicted: boolean,
	 *   deviation_label?: string,
	 *   deviation_seconds?: number,
	 *   number_of_stops_away?: number,
	 * }} Arrival
	 */
	/** @type {{ arrival: Arrival, trackingActive?: boolean, onToggleTracking?: (() => void) | null }} */
	let { arrival, trackingActive = false, onToggleTracking = null } = $props();

	const late    = (arrival.deviation_seconds ?? 0) > 60;
	const early   = (arrival.deviation_seconds ?? 0) < -60;
	const ontime  = !late && !early;
	const devColor = late ? '#ef4444' : early ? '#3b82f6' : '#4caf50';
</script>

<div class="flex items-center gap-3 rounded-lg px-3 py-2.5 transition hover:bg-zinc-50 dark:hover:bg-zinc-800/60">
	<!-- Route badge -->
	<span class="flex h-8 w-10 shrink-0 items-center justify-center rounded-md bg-oba-500 text-xs font-bold text-white">
		{arrival.route_short_name}
	</span>

	<!-- Info -->
	<div class="min-w-0 flex-1">
		<p class="truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">
			{arrival.headsign ?? arrival.route_long_name ?? '—'}
		</p>
		<div class="flex items-center gap-1.5 text-xs text-zinc-500 dark:text-zinc-400">
			<StatusDot live={arrival.predicted} color={arrival.predicted ? '#4caf50' : '#a1a1aa'} />
			{arrival.predicted ? 'Live' : 'Scheduled'}
			{#if arrival.number_of_stops_away != null && arrival.number_of_stops_away > 0}
				· {arrival.number_of_stops_away} stop{arrival.number_of_stops_away === 1 ? '' : 's'} away
			{/if}
		</div>
	</div>

	<!-- Time -->
	<div class="shrink-0 text-right">
		<p class="text-sm font-semibold {arrival.predicted ? 'text-oba-700 dark:text-oba-400' : 'text-zinc-600 dark:text-zinc-300'}">
			{arrival.predicted ? (arrival.predicted_arrival ?? arrival.scheduled_arrival) : arrival.scheduled_arrival}
		</p>
		{#if arrival.deviation_label}
			<span class="text-xs font-medium" style="color: {devColor}">
				{arrival.deviation_label}
			</span>
		{/if}
	</div>

	{#if onToggleTracking && arrival.predicted && arrival.trip_id}
		<button
			type="button"
			onclick={onToggleTracking}
			title={trackingActive ? 'Stop tracking this bus' : 'Track this bus'}
			aria-label={trackingActive ? 'Stop tracking this bus' : 'Track this bus'}
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition {trackingActive ? 'bg-oba-100 text-oba-700 dark:bg-oba-500/20 dark:text-oba-300' : 'text-zinc-400 hover:bg-zinc-100 hover:text-oba-600 dark:hover:bg-zinc-800 dark:hover:text-oba-400'}"
		>
			<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z"/>
				<circle cx="12" cy="12" r="2.5"/>
			</svg>
		</button>
	{/if}
</div>

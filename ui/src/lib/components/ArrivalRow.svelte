<script>
	import StatusDot from './StatusDot.svelte';

	/**
	 * @typedef {{
	 *   route_name?: string,
	 *   route_short_name?: string,
	 *   headsign?: string,
	 *   scheduled_arrival_display?: string,
	 *   scheduled_arrival?: string,
	 *   predicted_arrival_display?: string,
	 *   predicted_arrival?: string,
	 *   predicted: boolean,
	 *   deviation_label?: string,
	 *   deviation_seconds?: number,
	 *   number_of_stops_away?: number,
	 *   trip_id?: string,
	 *   vehicle_id?: string,
	 *   predicted_arrival_ms?: number,
	 *   scheduled_arrival_ms?: number,
	 * }} Arrival
	 */
	/** @type {{ arrival: Arrival, trackingActive?: boolean, onToggleTracking?: (() => void) | null, routeColor?: string }} */
	let {
		arrival,
		trackingActive = false,
		onToggleTracking = null,
		routeColor = '#78aa37',
	} = $props();

	const routeLabel = $derived(arrival.route_name ?? arrival.route_short_name ?? '');
	const schedTime = $derived(arrival.scheduled_arrival_display ?? arrival.scheduled_arrival ?? '');
	const predTime = $derived(
		arrival.predicted_arrival_display ?? arrival.predicted_arrival ?? schedTime,
	);
	const arrivalMs = $derived(
		(arrival.predicted ? arrival.predicted_arrival_ms : 0) || arrival.scheduled_arrival_ms || 0,
	);
	const eta = $derived.by(() => {
		if (!arrivalMs) return arrival.predicted ? predTime : schedTime;
		const minutes = Math.ceil((arrivalMs - Date.now()) / 60_000);
		if (minutes <= 0) return 'Due';
		return `${minutes} min`;
	});

	const late = $derived((arrival.deviation_seconds ?? 0) > 60);
	const early = $derived((arrival.deviation_seconds ?? 0) < -60);
	const isOnTime = $derived(!late && !early);
	const devColor = $derived(late ? '#e7604e' : early ? '#a58ae4' : '#36b99a');
	const canTrack = $derived(Boolean(onToggleTracking && arrival.predicted && arrival.trip_id));
</script>

<div
	class="arrival-row flex min-h-[78px] items-center gap-3.5 px-5 py-3 transition hover:bg-[#f7fafb] dark:hover:bg-[#20231d]"
>
	<!-- Route badge -->
	<span
		class="flex h-9 min-w-[48px] shrink-0 items-center justify-center rounded-[5px] px-2 text-base font-bold {arrival.predicted
			? 'text-white'
			: 'border border-dashed bg-transparent'}"
		style={arrival.predicted
			? `background:${routeColor}`
			: `border-color:${routeColor};color:${routeColor}`}
	>
		{routeLabel}
	</span>

	<!-- Info -->
	<div class="min-w-0 flex-1">
		<p class="truncate text-[15px] font-bold text-[#172126] dark:text-[#f0f2ed]">
			{arrival.headsign ?? '—'}
		</p>
		<div
			class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-[#6d7369] dark:text-[#a5ab9f]"
		>
			<StatusDot live={arrival.predicted} color={arrival.predicted ? '#36b99a' : '#8d9389'} />
			{arrival.predicted ? 'Live' : 'Scheduled'}
			{#if arrival.number_of_stops_away != null && arrival.number_of_stops_away > 0}
				· {arrival.number_of_stops_away} stop{arrival.number_of_stops_away === 1 ? '' : 's'} away
			{/if}
			{#if arrival.vehicle_id}
				· Bus {arrival.vehicle_id}
			{/if}
		</div>
	</div>

	<!-- Time -->
	<div class="w-[86px] shrink-0 text-right">
		<p
			class="text-[22px] font-bold leading-none tracking-tight {arrival.predicted
				? 'text-oba-700 dark:text-oba-400'
				: 'text-[#5f6659] dark:text-[#a5ab9f]'}"
		>
			{eta}
		</p>
		<p class="mt-1 text-xs leading-none text-[#7d8377] dark:text-[#858b80]">
			{arrival.predicted ? predTime : schedTime}
		</p>
		{#if arrival.deviation_label}
			<span class="mt-1 block text-xs font-semibold leading-none" style="color: {devColor}">
				{arrival.deviation_label}
			</span>
		{:else if arrival.predicted && isOnTime}
			<span class="mt-1 block text-xs font-semibold leading-none" style="color: #36b99a"
				>On time</span
			>
		{/if}
	</div>

	{#if onToggleTracking}
		<button
			type="button"
			onclick={onToggleTracking}
			disabled={!canTrack}
			title={trackingActive ? 'Stop tracking this bus' : 'Track this bus'}
			aria-label={trackingActive ? 'Stop tracking this bus' : 'Track this bus'}
			class="flex h-9 min-w-[72px] shrink-0 items-center justify-center gap-1.5 rounded-[6px] border px-2.5 text-xs font-bold transition disabled:cursor-not-allowed disabled:opacity-40 {trackingActive
				? 'border-oba-500 bg-oba-50 text-oba-800 hover:bg-oba-100 dark:border-oba-400 dark:bg-oba-900/45 dark:text-oba-300'
				: 'border-[#cedadf] text-[#172126] hover:border-oba-500 hover:bg-oba-50 dark:border-[#34382f] dark:text-[#f0f2ed] dark:hover:border-oba-400 dark:hover:bg-oba-900/35'}"
		>
			<svg
				xmlns="http://www.w3.org/2000/svg"
				width="16"
				height="16"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				stroke-linejoin="round"
			>
				<path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z" />
				<circle cx="12" cy="12" r="2.5" />
			</svg>
			{trackingActive ? 'Untrack' : 'Track'}
		</button>
	{/if}
</div>

<style>
	@media (max-width: 520px) {
		.arrival-row {
			gap: 0.65rem;
			padding-inline: 0.75rem;
		}
		button {
			min-width: 44px;
			padding-inline: 0.65rem;
			font-size: 0;
		}
	}
</style>

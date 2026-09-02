<script>
	/** @type {{ stopId: string, stopName: string, arrivals: any[] }} */
	let { stopName, arrivals } = $props();

	const eligible = $derived(arrivals.filter((a) => a.predicted && a.trip_id));
</script>

{#if eligible.length > 0}
	<div
		class="flex items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 dark:border-amber-700/40 dark:bg-amber-500/10"
	>
		<svg
			xmlns="http://www.w3.org/2000/svg"
			width="15"
			height="15"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
			class="shrink-0 text-amber-600 dark:text-amber-400"
		>
			<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
			<path d="M13.73 21a2 2 0 0 1-3.46 0" />
		</svg>
		<span class="text-sm text-oba-700 dark:text-oba-300">
			Select the eye icon beside a live arrival to track or untrack that bus at <strong
				>{stopName}</strong
			>.
		</span>
	</div>
{/if}

<script>
	import { browser } from '$app/environment';
	import { tracking } from '$lib/tracking.svelte.js';

	const STATUS_COLOR = {
		tracking: '#6b7280',
		approaching: '#f59e0b',
		arriving: '#22c55e',
		passed: '#22c55e',
		done: '#6b7280',
	};
	const STATUS_LABEL = {
		tracking: null,
		approaching: 'Approaching!',
		arriving: 'Arriving now! 🎉',
		passed: 'Passed this stop',
		done: 'Done',
	};

	function stopTracking(event, trackerId) {
		event.preventDefault();
		event.stopPropagation();
		tracking.remove(trackerId);
	}
</script>

{#if browser && tracking.trackers.length > 0}
	<div
		class="fixed bottom-4 right-4 flex w-72 flex-col gap-2"
		style="z-index: 10000; pointer-events: auto;"
	>
		{#each tracking.trackers as t (t.id)}
			<div
				class="flex items-center gap-2.5 rounded-xl border bg-white px-3 py-2.5 shadow-lg dark:bg-zinc-900
				{t.status === 'arriving' || t.status === 'passed'
					? 'border-green-300 dark:border-green-700'
					: t.status === 'approaching'
						? 'border-amber-300 dark:border-amber-700'
						: 'border-zinc-200 dark:border-zinc-700'}"
			>
				<!-- Live status dot -->
				<div
					class="h-2.5 w-2.5 shrink-0 rounded-full {t.status === 'arriving' ? 'animate-pulse' : ''}"
					style="background: {STATUS_COLOR[t.status] ?? '#6b7280'};"
				></div>

				<!-- Info -->
				<div class="min-w-0 flex-1">
					<p class="truncate text-xs font-semibold text-zinc-800 dark:text-zinc-100">
						Route {t.route_name}{t.headsign ? ` · ${t.headsign}` : ''}
					</p>
					<p class="text-xs text-zinc-500 dark:text-zinc-400">
						{#if STATUS_LABEL[t.status]}
							<span
								class={t.status === 'arriving' || t.status === 'passed'
									? 'font-semibold text-green-600 dark:text-green-400'
									: 'font-semibold text-amber-600 dark:text-amber-400'}
							>
								{STATUS_LABEL[t.status]}
							</span>
						{:else if t.stops_away != null && t.stops_away > 0}
							{t.stops_away} stop{t.stops_away === 1 ? '' : 's'} away
							{#if t.predicted_arrival}
								· {t.predicted_arrival}
							{/if}
						{:else if t.stops_away === 0}
							At stop
						{:else if t.predicted_arrival}
							Expected · {t.predicted_arrival}
						{:else}
							Waiting for update…
						{/if}
					</p>
					<p class="truncate text-xs text-zinc-400 dark:text-zinc-500">{t.stop_name}</p>
				</div>

				<!-- Cancel -->
				<button
					type="button"
					onpointerdown={(event) => stopTracking(event, t.id)}
					title="Stop tracking"
					aria-label={`Stop tracking Route ${t.route_name}`}
					class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 active:scale-95 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
					style="position: relative; z-index: 1; pointer-events: auto; touch-action: manipulation;"
				>
					<svg
						xmlns="http://www.w3.org/2000/svg"
						width="12"
						height="12"
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
			</div>
		{/each}
	</div>
{/if}

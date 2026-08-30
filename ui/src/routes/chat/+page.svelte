<script>
	import { tick } from 'svelte';
	import { marked } from 'marked';
	import { chat } from '$lib/chat.svelte.js';
	import { settings } from '$lib/settings.svelte.js';
	import { callTool } from '$lib/mcp.js';
	import { unwrap } from '$lib/result.js';
	import Icon from '$lib/components/Icon.svelte';
	import BusLoader from '$lib/components/BusLoader.svelte';
	import ArrivalRow from '$lib/components/ArrivalRow.svelte';
	import ArrivalsPanel from '$lib/components/ArrivalsPanel.svelte';
	import MapCard from '$lib/components/MapCard.svelte';
	import TrackOffer from '$lib/components/TrackOffer.svelte';

	marked.use({ gfm: true, breaks: true });

	let bottomEl;
	let textarea;

	// Track which route IDs are being drawn (for loading states on suggestion buttons)
	let drawingRoutes = $state(new Set());
	// Track which route IDs failed to draw (no valid stop coordinates)
	let drawErrors = $state(new Set());

	function orderedToolCards(cards) {
		const priority = (card) => {
			if (['map', 'route_map', 'vehicle_map', 'arrivals_route_map', 'combined_map'].includes(card.type)) return 0;
			if (card.type === 'arrivals') return 1;
			return 2;
		};
		return [...cards].sort((a, b) => priority(a) - priority(b));
	}

	function isRouteDrawn(routeId) {
		return chat.messages.some((message) =>
			(message.toolCards ?? []).some((card) =>
				(card.type === 'route_map' && card.route_id === routeId) ||
				(card.type === 'combined_map' && card.route_ids?.includes(routeId))
			)
		);
	}

	function arrivalPanelStopId(message) {
		return (message.toolCards ?? []).find((card) => card.type === 'arrivals')?.stop_id ?? null;
	}

	function addMarkersToMap(msg, markers) {
		const cards = msg.toolCards ?? [];
		const routeMap = cards.find((card) => card.type === 'route_map');
		if (!routeMap) {
			msg.toolCards = [...cards, { type: 'map', markers }];
			return;
		}

		const seen = new Set();
		const mergedMarkers = [...(routeMap.markers ?? []), ...markers].filter((marker) => {
			const key = marker.id ?? `${marker.lat},${marker.lon}`;
			if (seen.has(key)) return false;
			seen.add(key);
			return true;
		});
		msg.toolCards = cards.map((card) =>
			card === routeMap ? { ...card, markers: mergedMarkers } : card
		);
	}

	async function drawRoute(routeId, routeName, cardColor, msg) {
		if (drawingRoutes.has(routeId) || isRouteDrawn(routeId)) return;
		drawingRoutes = new Set([...drawingRoutes, routeId]);
		const errNext = new Set(drawErrors);
		errNext.delete(routeId);
		drawErrors = errNext;
		try {
			const result = await callTool('get_stops_for_route', { route_id: routeId });
			const routeData = unwrap(result);
			if (!routeData?.polylines?.length) {
				drawErrors = new Set([...drawErrors, routeId]);
				return;
			}
			const directions = routeData.polylines
				.map((polyline) => ({ direction: routeName, encodedPolyline: polyline.encoded_polyline }))
				.filter((line) => line.encodedPolyline);
			if (directions.length) {
				msg.toolCards = [
					...(msg.toolCards ?? []),
					{ type: 'route_map', route_id: routeId, route_name: routeName, color: cardColor, directions }
				];
				await tick();
				bottomEl?.scrollIntoView({ behavior: 'smooth' });
			} else {
				drawErrors = new Set([...drawErrors, routeId]);
			}
		} catch (e) {
			console.error('drawRoute failed:', e);
			drawErrors = new Set([...drawErrors, routeId]);
		} finally {
			const next = new Set(drawingRoutes);
			next.delete(routeId);
			drawingRoutes = next;
		}
	}

	const SUGGESTIONS = [
		'What buses are near downtown?',
		'When is the next bus at my stop?',
		'Which routes serve the university?',
		'Show me 5 stops near me',
	];

	// Scroll to bottom on new messages, loading, and every streaming text update
	$effect(() => {
		const _ = chat.messages.length
			+ (chat.loading   ? 1 : 0)
			+ (chat.messages.at(-1)?.text?.length ?? 0); // re-runs on each streamed chunk
		tick().then(() =>
			bottomEl?.scrollIntoView({ behavior: chat.streaming ? 'instant' : 'smooth' })
		);
	});

	// Re-fit textarea height when draft is restored after an error
	$effect(() => {
		const d = chat.draft;
		tick().then(() => {
			if (!textarea) return;
			textarea.style.height = 'auto';
			if (d) textarea.style.height = Math.min(textarea.scrollHeight, 140) + 'px';
		});
	});

	async function submit(text = chat.draft.trim()) {
		if (!text || chat.loading || chat.streaming) return;

		if (!settings.apiKey && settings.provider !== 'ollama' && settings.provider !== 'llama-server') {
			chat.error = `Set your ${settings.providerConfig?.label ?? 'AI'} key in Settings first.`;
			return;
		}

		chat.draft = '';
		if (textarea) { textarea.style.height = 'auto'; }

		await chat.send(text, {
			provider:  settings.provider,
			apiKey:    settings.apiKey,
			model:     settings.model,
			toolMode:  settings.toolMode,
		});
	}

	function handleKey(e) {
		if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); }
	}

	function autoResize(e) {
		e.target.style.height = 'auto';
		e.target.style.height = Math.min(e.target.scrollHeight, 140) + 'px';
	}
</script>

<svelte:head>
	<title>Chat · OBA Transit</title>
</svelte:head>

<style>
	:global(.md p)            { margin-bottom: 0.45rem; }
	:global(.md p:last-child) { margin-bottom: 0; }
	:global(.md strong)       { font-weight: 600; }
	:global(.md em)           { font-style: italic; }
	:global(.md code)         {
		font-family: ui-monospace, monospace; font-size: 0.8em;
		background: rgb(0 0 0 / 0.07); padding: 0.1em 0.35em; border-radius: 4px;
	}
	:global(.md ul)           { list-style: disc; margin: 0.4rem 0 0.4rem 1.2rem; }
	:global(.md ol)           { list-style: decimal; margin: 0.4rem 0 0.4rem 1.2rem; }
	:global(.md li)           { margin: 0.15rem 0; }
	:global(.md table)        { border-collapse: collapse; width: 100%; margin: 0.5rem 0; font-size: 0.82rem; }
	:global(.md th)           { background: rgb(0 0 0 / 0.06); font-weight: 600; padding: 0.3rem 0.6rem; text-align: left; border-bottom: 1px solid rgb(0 0 0 / 0.1); }
	:global(.md td)           { padding: 0.28rem 0.6rem; border-bottom: 1px solid rgb(0 0 0 / 0.06); vertical-align: top; }
	:global(.md tr:last-child td) { border-bottom: none; }
	:global(.md h1, .md h2, .md h3) { font-weight: 600; margin: 0.5rem 0 0.25rem; }
	:global(.dark .md code)   { background: rgb(255 255 255 / 0.1); }
	:global(.dark .md th)     { background: rgb(255 255 255 / 0.06); border-color: rgb(255 255 255 / 0.1); }
	:global(.dark .md td)     { border-color: rgb(255 255 255 / 0.06); }

	/* Blinking cursor appended while streaming */
	.streaming-cursor::after {
		content: '▋';
		display: inline;
		font-size: 0.85em;
		line-height: 1;
		margin-left: 1px;
		animation: blink 0.9s step-end infinite;
		color: #4caf50;
		vertical-align: baseline;
	}
	@keyframes blink { 0%, 100% { opacity: 1; } 50% { opacity: 0; } }
</style>

<div class="flex h-[calc(100vh-57px)] flex-col">

	<!-- ── Messages ─────────────────────────────────────────────── -->
	<div class="flex-1 overflow-y-auto px-4 py-6">
		<div class="mx-auto max-w-3xl space-y-5">

			<!-- Empty state -->
			{#if chat.messages.length === 0 && !chat.loading}
				<div class="py-12 text-center">
					<div class="mx-auto mb-5 h-16 w-16 overflow-hidden rounded-2xl shadow-md">
						<img src="/oba-mcp.png" alt="OBA" class="h-full w-full object-cover" />
					</div>
					<h2 class="mb-1 text-xl font-bold text-zinc-900 dark:text-zinc-100">Transit Assistant</h2>
					<p class="mb-7 text-sm text-zinc-500 dark:text-zinc-400">
						Ask anything about buses, stops, routes, and real-time arrivals.
					</p>
					<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
						{#each SUGGESTIONS as s}
							<button
								type="button"
								onclick={() => submit(s)}
								class="rounded-xl border border-zinc-200 bg-white px-4 py-3 text-left text-sm text-zinc-600 shadow-sm transition hover:border-oba-300 hover:bg-oba-50 hover:text-oba-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400 dark:hover:border-oba-700 dark:hover:bg-oba-500/10 dark:hover:text-oba-300"
							>
								{s}
							</button>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Message list -->
			{#each chat.messages as msg, i (i)}
				{#if msg.role === 'user'}
					<!-- User bubble -->
					<div class="flex justify-end">
						<div class="max-w-[80%] rounded-2xl rounded-tr-sm bg-oba-500 px-4 py-2.5 text-sm leading-relaxed text-white shadow-sm">
							{msg.text}
						</div>
					</div>
				{:else}
					<!-- Assistant bubble -->
					<div class="flex gap-3">
						<div class="mt-0.5 h-7 w-7 shrink-0 overflow-hidden rounded-full shadow-sm ring-2 ring-oba-200 dark:ring-oba-900">
							<img src="/oba-mcp.png" alt="OBA" class="h-full w-full object-cover" />
						</div>

						<div class="flex-1 space-y-2">
							<!-- Tool cards -->
							{#each orderedToolCards(msg.toolCards ?? []) as card}
								{#if card.type === 'map'}
									<MapCard markers={card.markers} stopId={arrivalPanelStopId(msg)} />

								{:else if card.type === 'route_map'}
									<MapCard routes={card.directions} markers={card.markers ?? []} stopId={arrivalPanelStopId(msg)} />

								{:else if card.type === 'combined_map'}
									<MapCard
										routes={card.directions ?? []}
										markers={card.markers ?? []}
										vehicles={card.vehicles ?? []}
										stopId={arrivalPanelStopId(msg)}
										vehicleTripIds={card.trip_ids ?? []}
										tripInfo={card.trip_info ?? {}}
									/>

								{:else if card.type === 'vehicle_map'}
									<MapCard
										vehicles={card.vehicles ?? []}
										agencyId={card.agency_id ?? null}
									/>

								{:else if card.type === 'arrivals_route_map'}
									<MapCard
										routes={card.directions ?? []}
										markers={card.stop_lat != null ? [{ lat: card.stop_lat, lon: card.stop_lon, name: card.stop_name, id: card.stop_id }] : []}
										vehicles={card.vehicles ?? []}
										vehicleTripIds={card.trip_ids ?? []}
										stopId={card.stop_id ?? null}
										tripInfo={card.trip_info ?? {}}
									/>

								{:else if card.type === 'route_suggestions'}
									<!-- Only offer routes that are not already visible on a map. -->
									{#if card.routes.some((route) => !isRouteDrawn(route.id))}
										<div class="overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900">
										<div class="border-b border-zinc-100 px-4 py-2 dark:border-zinc-800">
											<p class="text-xs font-semibold uppercase tracking-wide text-zinc-400">Draw route on map</p>
										</div>
										<div class="flex flex-col divide-y divide-zinc-100 dark:divide-zinc-800">
											{#each card.routes as route}
												{#if !isRouteDrawn(route.id)}
													<div class="flex items-center justify-between px-4 py-2.5">
													<div class="min-w-0">
														<span class="text-sm font-semibold text-zinc-800 dark:text-zinc-100">
															{#if route.color}
																<span class="mr-1.5 inline-block h-2.5 w-2.5 rounded-full" style="background: #{route.color};"></span>
															{/if}
															{route.name}
														</span>
														{#if route.description}
															<span class="ml-2 truncate text-xs text-zinc-400">{route.description}</span>
														{/if}
													</div>
													{#if drawErrors.has(route.id)}
														<span class="ml-3 shrink-0 text-xs text-red-500 dark:text-red-400">No map data</span>
													{:else}
														<button
															type="button"
															onclick={() => drawRoute(route.id, route.name, route.color, msg)}
															disabled={drawingRoutes.has(route.id)}
															class="ml-3 flex shrink-0 items-center gap-1.5 rounded-lg border border-oba-200 bg-oba-50 px-3 py-1.5 text-xs font-medium text-oba-700 transition hover:bg-oba-100 disabled:cursor-wait disabled:opacity-60 dark:border-oba-800 dark:bg-oba-500/10 dark:text-oba-400 dark:hover:bg-oba-500/20"
														>
															{#if drawingRoutes.has(route.id)}
																<Icon name="loader" cls="h-3 w-3 animate-spin" />
																Drawing…
															{:else}
																<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
																	<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
																</svg>
																Draw
															{/if}
														</button>
													{/if}
												</div>
												{/if}
											{/each}
										</div>
										</div>
									{/if}

								{:else if card.type === 'arrivals'}
									{#if card.stop_id}
										<div class="h-[300px] overflow-hidden rounded-xl border border-zinc-200 shadow-sm dark:border-zinc-700">
											<ArrivalsPanel stopId={card.stop_id} stopName={card.stop_name} initialArrivals={card.arrivals ?? []} />
										</div>
									{:else}
										<div class="overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900">
											<div class="border-b border-zinc-100 px-4 py-2 dark:border-zinc-800">
												<p class="text-xs font-semibold uppercase tracking-wide text-zinc-400">Live arrivals · {card.stop_name}</p>
											</div>
											{#each (card.arrivals ?? []) as arrival}
												<ArrivalRow {arrival} />
											{/each}
										</div>
									{/if}
									<TrackOffer
										stopId={card.stop_id ?? ''}
										stopName={card.stop_name ?? ''}
										arrivals={card.arrivals ?? []}
									/>
								{/if}
							{/each}

							<!-- Markdown text -->
							{#if msg.text}
								{@const isStreaming = chat.streaming && i === chat.messages.length - 1}
								<div class="rounded-2xl rounded-tl-sm bg-white px-4 py-3 shadow-sm dark:bg-zinc-900">
									<div class="md text-sm leading-relaxed text-zinc-800 dark:text-zinc-200 {isStreaming ? 'streaming-cursor' : ''}">
										{@html marked(msg.text)}
									</div>
								</div>
							{/if}

							<!-- Map suggestion pill -->
							{#if msg.mapSuggestion}
								<div class="flex items-center gap-2.5 rounded-xl border border-oba-200 bg-oba-50 px-3 py-2.5 dark:border-oba-800 dark:bg-oba-500/10">
									<svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="shrink-0 text-oba-600 dark:text-oba-400">
										<polygon points="3 6 9 3 15 6 21 3 21 18 15 21 9 18 3 21"/>
										<line x1="9" y1="3" x2="9" y2="18"/>
										<line x1="15" y1="6" x2="15" y2="21"/>
									</svg>
									<span class="text-sm text-oba-700 dark:text-oba-300">
										Show {msg.mapSuggestion.markers.length} stop{msg.mapSuggestion.markers.length === 1 ? '' : 's'} on map?
									</span>
									<div class="ml-auto flex shrink-0 gap-1.5">
										<button
											type="button"
											onclick={() => {
													addMarkersToMap(msg, msg.mapSuggestion.markers);
													msg.mapSuggestion = null;
											}}
											class="rounded-lg bg-oba-500 px-3 py-1 text-xs font-semibold text-white transition hover:bg-oba-600 active:scale-95"
										>
											Show
										</button>
										<button
											type="button"
											onclick={() => { msg.mapSuggestion = null; }}
											class="rounded-lg border border-oba-200 px-2 py-1 text-xs text-oba-600 transition hover:bg-oba-100 dark:border-oba-800 dark:text-oba-400 dark:hover:bg-oba-500/20"
										>
											Dismiss
										</button>
									</div>
								</div>
							{/if}
						</div>
					</div>
				{/if}
			{/each}

			<!-- Loading bubble -->
			{#if chat.loading}
				<div class="flex gap-3">
					<div class="mt-0.5 h-7 w-7 shrink-0 overflow-hidden rounded-full shadow-sm ring-2 ring-oba-200 dark:ring-oba-900">
						<img src="/oba-mcp.png" alt="OBA" class="h-full w-full object-cover" />
					</div>
					<div class="rounded-2xl rounded-tl-sm bg-white px-3 py-2 shadow-sm dark:bg-zinc-900">
						<BusLoader size="sm" />
					</div>
				</div>
			{/if}

			<!-- Error -->
			{#if chat.error}
				<div class="flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-400">
					<Icon name="alert-triangle" cls="mt-0.5 h-4 w-4 shrink-0" />
					<span class="flex-1">{chat.error}</span>
					{#if chat.error.toLowerCase().includes('key') || chat.error.toLowerCase().includes('setting')}
						<a href="/settings" class="shrink-0 font-semibold underline">Settings</a>
					{/if}
				</div>
			{/if}

			<div bind:this={bottomEl}></div>
		</div>
	</div>

	<!-- ── Input bar ──────────────────────────────────────────────── -->
	<div class="border-t border-zinc-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-950">
		<div class="mx-auto max-w-3xl">

			<!-- History controls -->
			{#if chat.messages.length > 0}
				<div class="mb-2 flex items-center justify-between">
					<span class="text-xs text-zinc-400 dark:text-zinc-600">
						{chat.messages.length} message{chat.messages.length === 1 ? '' : 's'} · saved locally
					</span>
					<button
						type="button"
						onclick={() => { if (confirm('Clear chat history?')) chat.clear(); }}
						class="text-xs text-zinc-400 transition hover:text-red-500 dark:text-zinc-600 dark:hover:text-red-400"
					>
						Clear history
					</button>
				</div>
			{/if}

			<!-- Textarea + send -->
			<div class="flex items-end gap-2 rounded-2xl border border-zinc-200 bg-zinc-50 p-2 shadow-sm transition focus-within:border-oba-400 focus-within:bg-white focus-within:ring-2 focus-within:ring-oba-400/20 dark:border-zinc-700 dark:bg-zinc-900 dark:focus-within:bg-zinc-900">
				<textarea
					bind:this={textarea}
					bind:value={chat.draft}
					onkeydown={handleKey}
					oninput={autoResize}
					placeholder="Ask about buses, stops, schedules…"
					rows="1"
					disabled={chat.loading}
					class="flex-1 resize-none bg-transparent px-2 py-1 text-sm text-zinc-900 placeholder:text-zinc-400 focus:outline-none disabled:opacity-60 dark:text-zinc-100 dark:placeholder:text-zinc-500"
					style="min-height: 36px; max-height: 140px;"
				></textarea>
				{#if chat.loading || chat.streaming}
					<!-- Stop button — visible while thinking or streaming -->
					<button
						type="button"
						onclick={() => chat.abort()}
						title="Stop response"
						class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-red-500 text-white transition hover:bg-red-600 active:scale-95 dark:bg-red-600 dark:hover:bg-red-500"
					>
						<Icon name="square" cls="h-3.5 w-3.5" />
					</button>
				{:else}
					<button
						type="button"
						onclick={() => submit()}
						disabled={chat.loading || !chat.draft.trim()}
						class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-oba-500 text-white transition hover:bg-oba-600 active:scale-95 disabled:cursor-not-allowed disabled:opacity-40"
					>
						{#if chat.loading}
							<Icon name="loader" cls="h-4 w-4 animate-spin" />
						{:else}
							<Icon name="send" cls="h-4 w-4" />
						{/if}
					</button>
				{/if}
			</div>

			<p class="mt-1.5 text-center text-xs text-zinc-400 dark:text-zinc-600">
				{settings.providerConfig?.label ?? 'AI'} · <a href="/settings" class="hover:text-oba-600 dark:hover:text-oba-400">Configure</a>
			</p>
		</div>
	</div>
</div>

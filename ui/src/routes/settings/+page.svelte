<script>
	import { settings, PROVIDERS } from '$lib/settings.svelte.js';
	import Icon from '$lib/components/Icon.svelte';

	let showKey      = $state(false);
	let saved        = $state(false);
	let testResult   = $state('');
	let testing      = $state(false);
	let fetchingModels = $state(false);

	let provider   = $state(settings.provider);
	let apiKey     = $state(settings.apiKey);
	let model      = $state(settings.model);
	let mapStyle   = $state(settings.mapStyle);
	let toolMode   = $state(settings.toolMode);

	let liveModels = $state(settings.getCachedModels(provider));

	const MAP_STYLES = [
		{ id: 'https://tiles.openfreemap.org/styles/bright',   label: 'OpenFreeMap Bright — clean, minimal (recommended)' },
		{ id: 'https://tiles.openfreemap.org/styles/liberty',  label: 'OpenFreeMap Liberty — detailed' },
		{ id: 'https://demotiles.maplibre.org/style.json',     label: 'MapLibre Demo — ultra-minimal' },
	];

	const currentProvider = $derived(PROVIDERS.find((p) => p.id === provider) ?? PROVIDERS[0]);

	async function fetchLiveModels() {
		const prov = PROVIDERS.find((p) => p.id === provider);
		if (!prov?.baseUrl) return;
		fetchingModels = true;
		try {
			const res = await fetch(`${prov.baseUrl}/models`, {
				headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {}
			});
			if (!res.ok) throw new Error(`HTTP ${res.status}`);
			const data = await res.json();
			const all = (data.data ?? data.models ?? []).map((m) => ({
				id: m.id,
				label: (m.name ?? m.id).replace(/\s*\(free\)/i, '').trim() +
					(m.id.endsWith(':free') || (m.pricing?.prompt === '0') ? ' (free)' : '')
			}));
			// For OpenRouter keep only :free models
			const fetched = provider === 'openrouter' ? all.filter((m) => m.id.endsWith(':free')) : all;
			liveModels = fetched;
			settings.setCachedModels(provider, fetched);
			if (fetched.length && !fetched.find((m) => m.id === model)) {
				model = fetched[0].id;
			}
		} catch (e) {
			liveModels = [];
		} finally {
			fetchingModels = false;
		}
	}

	function onProviderChange(p) {
		provider = p;
		liveModels = settings.getCachedModels(p);
		const prov = PROVIDERS.find((x) => x.id === p);
		const modelList = liveModels ?? prov?.models ?? [];
		if (!modelList.find((m) => m.id === model)) {
			model = (modelList[0] ?? prov?.models[0])?.id ?? model;
		}
	}

	function save() {
		settings.provider   = provider;
		settings.apiKey     = apiKey;
		settings.model      = model;
		settings.mapStyle   = mapStyle;
		settings.toolMode   = toolMode;
		saved = true;
		setTimeout(() => (saved = false), 2500);
	}

	async function testConnection() {
		testing    = true;
		testResult = '';
		try {
			const res = await fetch('/api/mcp', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream' },
				body: JSON.stringify({
					jsonrpc: '2.0', id: 'test', method: 'initialize',
					params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'settings-test', version: '1' } }
				})
			});
			testResult = res.ok ? 'success' : `HTTP ${res.status}`;
		} catch (e) {
			testResult = e.message;
		} finally {
			testing = false;
		}
	}
</script>

<svelte:head>
	<title>Settings · OBA Transit</title>
</svelte:head>

<div class="mx-auto max-w-2xl px-4 py-8 sm:px-6">
	<h1 class="mb-1 text-2xl font-bold text-zinc-900 dark:text-zinc-100">Settings</h1>
	<p class="mb-6 text-sm text-zinc-500 dark:text-zinc-400">Stored in your browser's localStorage — never sent to any third party.</p>

	<form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-5">

		<!-- AI Provider -->
		<section class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
			<h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-zinc-900 dark:text-zinc-100">
				<Icon name="message-circle" cls="h-4 w-4 text-oba-500" />
				AI Provider
			</h2>

			<!-- Provider picker -->
			<div class="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
				{#each PROVIDERS as p}
					<button
						type="button"
						onclick={() => onProviderChange(p.id)}
						class="flex flex-col rounded-lg border px-3 py-2.5 text-left text-sm transition
							{provider === p.id
								? 'border-oba-400 bg-oba-50 text-oba-700 ring-1 ring-oba-400/30 dark:border-oba-600 dark:bg-oba-500/10 dark:text-oba-400'
								: 'border-zinc-200 bg-zinc-50 text-zinc-600 hover:border-zinc-300 hover:bg-white dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700'}"
					>
						<span class="font-medium">{p.label}</span>
					</button>
				{/each}
			</div>

			<div class="space-y-4">
				<!-- API key -->
				<div>
					<label for="api-key" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">
						API Key
					</label>
					<div class="relative">
						<input
							id="api-key"
							type={showKey ? 'text' : 'password'}
							bind:value={apiKey}
							placeholder={currentProvider.keyPlaceholder}
							autocomplete="off"
							class="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 pr-10 font-mono text-sm focus:border-oba-400 focus:outline-none focus:ring-2 focus:ring-oba-400/20 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100 dark:placeholder:text-zinc-500"
						/>
						<button
							type="button"
							onclick={() => (showKey = !showKey)}
							class="absolute right-2.5 top-1/2 -translate-y-1/2 text-zinc-400 transition hover:text-zinc-600 dark:hover:text-zinc-300"
						>
							<Icon name={showKey ? 'eye-off' : 'eye'} cls="h-4 w-4" />
						</button>
					</div>
					<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">{currentProvider.keyHelp}</p>
				</div>

				<!-- Tool access mode -->
				<div>
					<p class="mb-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-300">Tool access</p>
					<div class="grid grid-cols-2 gap-2">
						{#each [{ id: 'rider', label: 'Rider', desc: 'Curated tools for passenger questions' }, { id: 'all', label: 'All tools', desc: 'Every tool the MCP server exposes' }] as opt}
							<button
								type="button"
								onclick={() => (toolMode = opt.id)}
								class="flex flex-col rounded-lg border px-3 py-2.5 text-left text-sm transition
									{toolMode === opt.id
										? 'border-oba-400 bg-oba-50 text-oba-700 ring-1 ring-oba-400/30 dark:border-oba-600 dark:bg-oba-500/10 dark:text-oba-400'
										: 'border-zinc-200 bg-zinc-50 text-zinc-600 hover:border-zinc-300 hover:bg-white dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700'}"
							>
								<span class="font-medium">{opt.label}</span>
								<span class="mt-0.5 text-xs opacity-70">{opt.desc}</span>
							</button>
						{/each}
					</div>
				</div>

				<!-- Model -->
				<div>
					<div class="mb-1.5 flex items-center justify-between">
						<label for="model" class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Model</label>
						{#if currentProvider.baseUrl}
							<button
								type="button"
								onclick={fetchLiveModels}
								disabled={fetchingModels}
								class="flex items-center gap-1 text-xs text-zinc-400 transition hover:text-oba-600 disabled:opacity-50 dark:hover:text-oba-400"
								title="Fetch live model list from provider"
							>
								<Icon name="refresh-cw" cls="h-3 w-3 {fetchingModels ? 'animate-spin' : ''}" />
								{fetchingModels ? 'Fetching…' : liveModels ? `${liveModels.length} models ↺` : 'Fetch models'}
							</button>
						{/if}
					</div>
					<select
						id="model"
						bind:value={model}
						class="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 text-sm focus:border-oba-400 focus:outline-none focus:ring-2 focus:ring-oba-400/20 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100"
					>
						{#each (liveModels ?? currentProvider.models) as m}
							<option value={m.id}>{m.label}</option>
						{/each}
					</select>
					{#if liveModels?.length === 0}
						<p class="mt-1 text-xs text-amber-500">No models found — check your API key or provider URL.</p>
					{/if}
				</div>
			</div>
		</section>

		<!-- oba-mcp server -->
		<section class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
			<h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-zinc-900 dark:text-zinc-100">
				<Icon name="activity" cls="h-4 w-4 text-oba-500" />
				oba-mcp Server
			</h2>

			<div class="space-y-4">
				<div>
					<div class="flex items-center gap-2">
						<p class="flex-1 text-sm text-zinc-600 dark:text-zinc-300">
							Configured by the UI server. MCP credentials never enter this browser.
						</p>
						<button
							type="button"
							onclick={testConnection}
							disabled={testing}
							class="flex items-center gap-1.5 rounded-lg border border-zinc-200 px-3 py-2 text-sm font-medium text-zinc-600 transition hover:bg-zinc-50 disabled:opacity-60 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
						>
							{#if testing}
								<Icon name="loader" cls="h-3.5 w-3.5 animate-spin" />
							{:else}
								<Icon name="activity" cls="h-3.5 w-3.5" />
							{/if}
							Test
						</button>
					</div>
					{#if testResult}
						<p class="mt-1.5 flex items-center gap-1.5 text-xs {testResult === 'success' ? 'text-oba-600 dark:text-oba-400' : 'text-red-500'}">
							<Icon name={testResult === 'success' ? 'check' : 'alert-triangle'} cls="h-3.5 w-3.5 shrink-0" />
							{testResult === 'success' ? 'Connected!' : testResult}
						</p>
					{:else}
						<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
							Ask the deployment administrator to configure the MCP connection if this test fails.
						</p>
					{/if}
				</div>
			</div>
		</section>

		<!-- Map -->
		<section class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
			<h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-zinc-900 dark:text-zinc-100">
				<Icon name="map" cls="h-4 w-4 text-oba-500" />
				Map
			</h2>

			<div class="space-y-2">
				{#each MAP_STYLES as s}
					<label class="flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition
						{mapStyle === s.id ? 'border-oba-400 bg-oba-50 dark:border-oba-600 dark:bg-oba-500/10' : 'border-zinc-200 hover:bg-zinc-50 dark:border-zinc-700 dark:hover:bg-zinc-800'}">
						<input type="radio" bind:group={mapStyle} value={s.id} class="accent-oba-500" />
						<span class="text-sm text-zinc-700 dark:text-zinc-300">{s.label}</span>
					</label>
				{/each}
				<div>
					<p class="mb-1 text-xs text-zinc-400 dark:text-zinc-500">Or paste a custom MapLibre style URL:</p>
					<input
						type="url"
						bind:value={mapStyle}
						placeholder="https://…/style.json"
						class="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 font-mono text-sm focus:border-oba-400 focus:outline-none focus:ring-2 focus:ring-oba-400/20 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100"
					/>
				</div>
			</div>
		</section>

		<!-- Save -->
		<div class="flex items-center justify-end gap-3">
			<button
				type="submit"
				class="flex items-center gap-2 rounded-xl bg-oba-500 px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-oba-600 active:scale-95"
			>
				{#if saved}
					<Icon name="check" cls="h-4 w-4" />
					Saved!
				{:else}
					Save settings
				{/if}
			</button>
		</div>
	</form>

	<!-- Quick start -->
	<div class="mt-8 rounded-xl border border-zinc-200 bg-zinc-50 p-5 dark:border-zinc-800 dark:bg-zinc-900/50">
		<h3 class="mb-3 text-sm font-semibold text-zinc-700 dark:text-zinc-300">Quick start</h3>
		<ol class="space-y-2.5 text-sm text-zinc-600 dark:text-zinc-400">
			<li class="flex gap-3">
				<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-oba-500 text-xs font-bold text-white">1</span>
				Start maglev: <code class="rounded bg-zinc-200 px-1.5 py-0.5 text-xs dark:bg-zinc-800">cd maglev && make run</code>
			</li>
			<li class="flex gap-3">
				<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-oba-500 text-xs font-bold text-white">2</span>
				Start oba-mcp: <code class="rounded bg-zinc-200 px-1.5 py-0.5 text-xs dark:bg-zinc-800">cd oba-mcp && OBA_HTTP_AUTH_TOKEN=local-dev make serve-http</code>
			</li>
			<li class="flex gap-3">
				<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-oba-500 text-xs font-bold text-white">3</span>
				No API key? Pick <strong>Groq</strong> above — free account at <code class="rounded bg-zinc-200 px-1.5 py-0.5 text-xs dark:bg-zinc-800">console.groq.com</code>
			</li>
			<li class="flex gap-3">
				<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-oba-500 text-xs font-bold text-white">4</span>
				Go to <a href="/chat" class="font-medium text-oba-600 underline dark:text-oba-400">Chat</a> and ask about buses!
			</li>
		</ol>
	</div>
</div>

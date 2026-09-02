<script>
	import '../app.css';
	import Header from '$lib/components/Header.svelte';
	import TrackingWidget from '$lib/components/TrackingWidget.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<title>OBA Transit Dashboard</title>
</svelte:head>

<div
	class="flex min-h-screen flex-col bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100"
>
	<Header />
	<main class="flex flex-1 flex-col">
		{@render children()}
	</main>
	<TrackingWidget />
</div>

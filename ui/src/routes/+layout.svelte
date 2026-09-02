<script>
	import '../app.css';
	import Header from '$lib/components/Header.svelte';
	import TrackingWidget from '$lib/components/TrackingWidget.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<title>OneBusAway Assistant</title>
</svelte:head>

<div
	class="flex min-h-screen flex-col bg-[#f3f7f9] text-[#172126] dark:bg-[#11130f] dark:text-[#f0f2ed]"
>
	<Header />
	<main class="flex flex-1 flex-col">
		{@render children()}
	</main>
	<TrackingWidget />
</div>

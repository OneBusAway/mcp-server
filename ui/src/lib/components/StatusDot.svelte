<script>
	/** @type {{ live?: boolean, color?: string }} */
	let { live = false, color = '#4caf50' } = $props();
</script>

<span class="relative flex h-2.5 w-2.5 shrink-0">
	{#if live}
		<span
			class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
			style="background-color: {color}"
		></span>
	{/if}
	<span class="relative inline-flex h-2.5 w-2.5 rounded-full" style="background-color: {color}"></span>
</span>

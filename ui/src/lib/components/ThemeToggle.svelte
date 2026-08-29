<script>
	import { theme } from '$lib/theme.svelte.js';
	import Icon from './Icon.svelte';
</script>

<button
	type="button"
	onclick={() => theme.toggle()}
	class="rounded-full p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
	title="Toggle theme"
>
	{#if theme.dark}
		<Icon name="sun" cls="h-4 w-4" />
	{:else}
		<Icon name="moon" cls="h-4 w-4" />
	{/if}
</button>

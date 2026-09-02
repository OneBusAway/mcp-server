<script>
	import { theme } from '$lib/theme.svelte.js';
	import Icon from './Icon.svelte';
</script>

<button
	type="button"
	onclick={() => theme.toggle()}
	aria-label={theme.dark ? 'Switch to light theme' : 'Switch to dark theme'}
	class="flex min-h-[38px] items-center gap-2 rounded-lg border border-white/30 px-3 text-[13px] font-semibold text-white transition hover:bg-white/10"
	title="Toggle theme"
>
	{#if theme.dark}
		<Icon name="sun" cls="h-4 w-4" />
	{:else}
		<Icon name="moon" cls="h-4 w-4" />
	{/if}
	<span class="hidden md:inline">{theme.dark ? 'Light' : 'Dark'}</span>
</button>

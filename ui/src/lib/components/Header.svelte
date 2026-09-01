<script>
	import Icon from './Icon.svelte';
	import ThemeToggle from './ThemeToggle.svelte';
	import { page } from '$app/stores';

	const NAV = [
		{ href: '/chat',     label: 'Chat',     icon: 'message-circle' },
		{ href: '/settings', label: 'Settings', icon: 'settings' },
	];
</script>

<header class="sticky top-0 border-b border-zinc-200/80 bg-white/80 backdrop-blur-md dark:border-zinc-800/80 dark:bg-zinc-950/80" style="z-index: 9999; pointer-events: auto;">
	<div class="mx-auto grid max-w-7xl grid-cols-3 items-center px-4 py-3 sm:px-6">
		<!-- Left: Brand -->
		<a href="/chat" class="flex items-center gap-2.5 justify-self-start">
			<img src="/oba-mcp.png" alt="OneBusAway" class="h-9 w-9 rounded-xl shadow-sm" width="36" height="36" />
			<div class="hidden sm:block">
				<p class="text-sm font-bold leading-tight text-zinc-900 dark:text-zinc-100">OBA Transit</p>
				<p class="text-xs text-zinc-500 dark:text-zinc-400">MCP Dashboard</p>
			</div>
		</a>

		<!-- Center: Nav (desktop) / Mobile icon nav -->
		<nav class="flex items-center justify-center gap-1">
			{#each NAV as item}
				{@const active = $page.url.pathname === item.href || ($page.url.pathname.startsWith(item.href) && item.href !== '/')}
				<!-- Desktop: icon + label -->
				<a
					href={item.href}
					class="hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition sm:flex
						{active
							? 'bg-oba-50 text-oba-700 dark:bg-oba-500/10 dark:text-oba-400'
							: 'text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'}"
				>
					<Icon name={item.icon} cls="h-3.5 w-3.5" />
					{item.label}
				</a>
				<!-- Mobile: icon only -->
				<a
					href={item.href}
					class="rounded-lg p-2 transition sm:hidden
						{active
							? 'text-oba-600 dark:text-oba-400'
							: 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}"
					title={item.label}
				>
					<Icon name={item.icon} cls="h-4 w-4" />
				</a>
			{/each}
		</nav>

		<!-- Right: Theme toggle -->
		<div class="flex items-center justify-end">
			<ThemeToggle />
		</div>
	</div>
</header>

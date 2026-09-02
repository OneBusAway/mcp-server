<script>
	import Icon from './Icon.svelte';
	import ThemeToggle from './ThemeToggle.svelte';
	import { page } from '$app/stores';

	const NAV = [
		{ href: '/chat', label: 'Chat', icon: 'message-circle' },
		{ href: '/settings', label: 'Settings', icon: 'settings' },
	];
</script>

<header
	class="sticky top-0 h-[58px] border-b border-black/40 bg-[#1b2017] text-white dark:bg-[#0b0d09]"
	style="z-index: 9999; pointer-events: auto;"
>
	<div class="mx-auto flex h-full max-w-7xl items-center gap-5 px-4 sm:px-6">
		<!-- Left: Brand -->
		<a href="/chat" class="mr-auto flex items-center gap-2.5 text-white no-underline">
			<img
				src="/oba-mcp.png"
				alt="OneBusAway"
				class="h-[32px] w-[32px] rounded-lg"
				width="32"
				height="32"
			/>
			<span class="oba-heading hidden text-[21px] font-medium tracking-tight sm:block"
				>OneBusAway Assistant</span
			>
		</a>

		<!-- Center: Nav (desktop) / Mobile icon nav -->
		<nav class="flex items-center justify-center gap-1" aria-label="Sections">
			{#each NAV as item}
				{@const active =
					$page.url.pathname === item.href ||
					($page.url.pathname.startsWith(item.href) && item.href !== '/')}
				<!-- Desktop: icon + label -->
				<a
					href={item.href}
					aria-current={active ? 'page' : undefined}
					class="hidden min-h-[38px] items-center gap-1.5 rounded-lg px-3.5 text-sm transition sm:flex
						{active
						? 'bg-white/15 font-semibold text-white'
						: 'font-medium text-white/75 hover:bg-white/10 hover:text-white'}"
				>
					<Icon name={item.icon} cls="h-3.5 w-3.5" />
					{item.label}
				</a>
				<!-- Mobile: icon only -->
				<a
					href={item.href}
					aria-current={active ? 'page' : undefined}
					class="rounded-lg p-2 transition sm:hidden
						{active ? 'bg-white/15 text-white' : 'text-white/70 hover:bg-white/10 hover:text-white'}"
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

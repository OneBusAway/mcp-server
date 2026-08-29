<script>
	import { onMount, onDestroy } from 'svelte';

	/** @type {{ label?: string, size?: 'sm' | 'md' | 'lg' }} */
	let { label = null, size = 'md' } = $props();

	const MESSAGES = [
		'Checking the schedule…',
		'Fetching real-time data…',
		'Finding your bus…',
		'Convincing the buses to cooperate…',
		'Almost at the stop…',
		'Counting stops away…',
		'Untangling the transit spaghetti…',
		'Herding buses into formation…',
		'Running the route…',
		'Hold on, bus incoming…',
		'Scanning the network…',
		'Checking if the bus remembered its keys…',
		'Polishing the imaginary express lane…',
		'Asking the timetable nicely…',
		'Warming up the bus-shaped calculator…',
		'Putting the wheels in motion…',
		'Negotiating with the next stop…',
		'Locating the bus in the wild…',
		'Giving the route a tiny pep talk…',
		'Counting buses like sheep…',
		'Untying a particularly stubborn traffic knot…',
		'Checking the bus’s tiny clipboard…',
		'Looking under the transit couch cushions…',
		'Consulting the sacred map fold…',
		'Telling the GPS to keep calm…',
		'Calculating bus vibes…',
		'Launching the extremely small search party…',
		'Making sure the bus is wearing its seatbelt…',
		'Bribing the ETA with punctuality points…',
		'Assembling the answer one stop at a time…',
		'Checking whether traffic has had its coffee…',
	];

	let msgIndex = $state(0);
	let timer;

	onMount(() => {
		msgIndex = Math.floor(Math.random() * MESSAGES.length);
		timer = setInterval(() => {
			msgIndex = (msgIndex + 1) % MESSAGES.length;
		}, 2000);
	});

	onDestroy(() => clearInterval(timer));

	const displayLabel = $derived(label ?? MESSAGES[msgIndex]);
</script>

<div class="flex flex-col items-center gap-3 py-4">
	<!-- Track + bus scene -->
	<div class="loader-scene relative overflow-hidden rounded-xl border border-oba-100/70 bg-gradient-to-b from-oba-50/80 via-white to-white shadow-sm dark:border-oba-900/40 dark:from-oba-900/20 dark:via-zinc-900 dark:to-zinc-900" style="width: 184px; height: 48px;">
		<!-- Soft motion streaks make the scene feel alive while the bus travels. -->
		<div class="motion-streak absolute left-2 top-3 h-px w-7 rounded-full bg-oba-300/50"></div>
		<div class="motion-streak motion-streak-delayed absolute left-12 top-5 h-px w-4 rounded-full bg-oba-200/60"></div>

		<!-- Road -->
		<div class="absolute bottom-2 left-0 right-0 h-2 bg-gradient-to-b from-zinc-200/70 to-transparent dark:from-zinc-700/60"></div>
		<div class="absolute bottom-1 left-0 right-0 h-px bg-zinc-300 dark:bg-zinc-600"></div>

		<!-- Dashed line -->
		<div class="road-dashes absolute bottom-[5px] left-0 h-px w-[400%]"
			style="background: repeating-linear-gradient(to right, transparent 0, transparent 8px, #a1a1aa 8px, #a1a1aa 16px)">
		</div>

		<!-- Bus SVG — small, flat, clean -->
		<div class="bus absolute bottom-1">
			<svg width="52" height="28" viewBox="0 0 52 28" fill="none">
				<!-- Shadow -->
				<ellipse class="bus-shadow" cx="26" cy="27" rx="22" ry="2" fill="#000" opacity="0.12"/>
				<!-- Body -->
				<rect x="1" y="3" width="46" height="20" rx="4" fill="#4caf50"/>
				<!-- Roof shine -->
				<rect x="4" y="3" width="40" height="6" rx="3" fill="#66bb6a" opacity="0.45"/>
				<!-- Windows -->
				<rect x="6"  y="7" width="8" height="7" rx="2" fill="white" opacity="0.9"/>
				<rect x="17" y="7" width="8" height="7" rx="2" fill="white" opacity="0.9"/>
				<rect x="28" y="7" width="8" height="7" rx="2" fill="white" opacity="0.9"/>
				<!-- Front glass -->
				<rect x="39" y="7" width="6" height="7" rx="2" fill="white" opacity="0.85"/>
				<!-- Front cap -->
				<rect x="45" y="8" width="5" height="10" rx="2.5" fill="#388e3c"/>
				<!-- Headlight -->
				<rect class="headlight" x="47" y="10" width="2.5" height="3.5" rx="1" fill="#fff9c4"/>
				<!-- Bottom stripe -->
				<rect x="1" y="19" width="46" height="3" fill="#388e3c"/>
				<!-- Wheels -->
				<g class="wheel"><circle cx="12" cy="24" r="4.5" fill="#1c1c1e"/><circle cx="12" cy="24" r="2.5" fill="#333"/>
				<circle cx="12" cy="24" r="1"   fill="#555"/>
				</g><g class="wheel"><circle cx="38" cy="24" r="4.5" fill="#1c1c1e"/><circle cx="38" cy="24" r="2.5" fill="#333"/>
				<circle cx="38" cy="24" r="1"   fill="#555"/>
				</g>
			</svg>
		</div>

		<!-- Bus stop pole -->
		<div class="stop-pole absolute bottom-1 right-7 h-8 w-px bg-zinc-300 dark:bg-zinc-600"></div>
		<div class="stop-sign absolute right-5 top-2 flex h-3 w-5 items-center justify-center rounded-sm bg-oba-500 text-[7px] font-bold text-white shadow-sm">+</div>
	</div>

	<!-- Cycling label -->
	<p class="min-h-[1.2rem] text-xs font-medium text-zinc-400 transition-opacity duration-500 dark:text-zinc-500">
		{displayLabel}
	</p>
</div>

<style>
	/* Keyframes live in app.css (global). Class names here are Svelte-scoped
	   but the referenced @keyframes names are global — that's intentional. */
	.bus {
		animation: oba-bus-slide 4.8s cubic-bezier(0.4, 0, 0.2, 1) infinite;
		filter: drop-shadow(0 3px 2px rgb(0 0 0 / 0.12));
		will-change: transform;
	}
	.road-dashes {
		animation: oba-road-scroll 0.65s linear infinite;
	}
	@keyframes oba-wheel-spin {
		to { transform: rotate(360deg); }
	}
	@keyframes oba-shadow-pulse {
		0%, 100% { transform: scaleX(0.9); opacity: 0.1; }
		50% { transform: scaleX(1.05); opacity: 0.16; }
	}
	@keyframes oba-headlight {
		0%, 100% { opacity: 0.55; }
		50% { opacity: 1; filter: drop-shadow(0 0 3px #fff9c4); }
	}
	@keyframes oba-stop-pulse {
		0%, 100% { transform: scale(1); box-shadow: 0 0 0 0 rgb(76 175 80 / 0.35); }
		50% { transform: scale(1.08); box-shadow: 0 0 0 4px rgb(76 175 80 / 0); }
	}
	@keyframes oba-streak {
		0%, 100% { opacity: 0.15; transform: translateX(0); }
		45% { opacity: 0.65; transform: translateX(18px); }
		70% { opacity: 0; transform: translateX(30px); }
	}
	.wheel {
		transform-box: fill-box;
		transform-origin: center;
		animation: oba-wheel-spin 0.55s linear infinite;
	}
	.bus-shadow {
		animation: oba-shadow-pulse 1.2s ease-in-out infinite;
	}
	.headlight {
		animation: oba-headlight 1.4s ease-in-out infinite;
	}
	.stop-sign {
		animation: oba-stop-pulse 1.8s ease-in-out infinite;
	}
	.motion-streak {
		animation: oba-streak 2.4s ease-in-out infinite;
	}
	.motion-streak-delayed {
		animation-delay: 0.8s;
	}
	@media (prefers-reduced-motion: reduce) {
		.bus, .road-dashes, .wheel, .bus-shadow, .headlight, .stop-sign, .motion-streak {
			animation-play-state: paused;
		}
	}
</style>

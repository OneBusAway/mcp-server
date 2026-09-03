/**
 * Shared reactive clock. Ticks every 15s so ETA countdowns stay current
 * between arrival refreshes without every row spinning up its own timer.
 */

import { browser } from '$app/environment';

const TICK_MS = 15_000;

let nowMs = $state(Date.now());
let intervalId = null;
let subscribers = 0;

function start() {
	if (!browser || intervalId != null) return;
	intervalId = setInterval(() => {
		nowMs = Date.now();
	}, TICK_MS);
}

function stop() {
	if (intervalId != null) {
		clearInterval(intervalId);
		intervalId = null;
	}
}

/** Subscribe to the shared clock. Call the returned function to unsubscribe. */
export function useNow() {
	subscribers += 1;
	start();
	return () => {
		subscribers -= 1;
		if (subscribers <= 0) {
			subscribers = 0;
			stop();
		}
	};
}

export function now() {
	return nowMs;
}

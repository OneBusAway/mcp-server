/**
 * Global chat store — lives for the lifetime of the browser tab, not the component.
 * Messages persist to localStorage. In-flight requests survive tab navigation.
 */

const STORAGE_KEY = 'oba_chat_history';
const MAX_HISTORY = 80;

const COMBINABLE_MAP_TYPES = new Set(['map', 'route_map']);

function mergeMarkers(cards) {
	const seen = new Set();
	return cards
		.flatMap((card) => card.markers ?? [])
		.filter((marker) => {
			const key = marker.id ?? `${marker.lat},${marker.lon}`;
			if (seen.has(key)) return false;
			seen.add(key);
			return true;
		});
}

function coalesceMapCards(cards, incoming) {
	const mapCards = [...cards.filter((card) => COMBINABLE_MAP_TYPES.has(card.type)), incoming];
	const routeCards = mapCards.filter((card) => card.type === 'route_map');
	const directions = routeCards.flatMap((card) => card.directions ?? []);
	const markers = mergeMarkers(mapCards);

	if (!directions.length) return { type: 'map', markers };

	// MapCard supports both route geometry and markers, so keep all details together.
	return {
		...routeCards.at(-1),
		type: 'route_map',
		directions,
		markers
	};
}

function loadFromStorage() {
	try {
		return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
	} catch {
		return [];
	}
}

function saveToStorage(msgs) {
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(msgs.slice(-MAX_HISTORY)));
	} catch {}
}

function createChatStore() {
	let messages  = $state(typeof localStorage !== 'undefined' ? loadFromStorage() : []);
	let loading   = $state(false);  // true while waiting for first byte (shows BusLoader)
	let streaming = $state(false);  // true while text is flowing in (input disabled)
	let error     = $state('');
	let draft     = $state('');
	let _abort    = null;

	function abort() {
		_abort?.abort();
		_abort = null;
	}

	async function send(text, { provider, apiKey, model }) {
		if (!text?.trim() || loading || streaming) return;

		error    = '';
		messages = [...messages, { role: 'user', text: text.trim() }];
		saveToStorage(messages);
		loading  = true;
		_abort   = new AbortController();

		try {
			const res = await fetch('/api/chat', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				signal: _abort.signal,
				body: JSON.stringify({
					messages: messages.map((m) => ({ role: m.role, content: m.text })),
					provider,
					apiKey,
					model
				})
			});

			if (!res.ok) {
				const e = await res.json().catch(() => ({}));
				throw new Error(e.error ?? `Server error ${res.status}`);
			}

			// Add the assistant shell — stream will fill it in
			messages  = [...messages, { role: 'assistant', text: '', toolCards: [] }];
			const lastMsg = messages[messages.length - 1]; // Svelte 5 deep-reactive proxy
			loading   = false;
			streaming = true;

			const reader  = res.body.getReader();
			const decoder = new TextDecoder();
			let buf  = '';
			let done = false;

			while (!done) {
				const { done: eof, value } = await reader.read();
				if (eof) break;

				buf += decoder.decode(value, { stream: true });
				const parts = buf.split('\n\n');
				buf = parts.pop() ?? '';

				for (const part of parts) {
					const line = part.trim();
					if (!line.startsWith('data: ')) continue;
					let evt;
					try { evt = JSON.parse(line.slice(6)); } catch { continue; }

						if (evt.t === 'text') {
							lastMsg.text += evt.v;
						} else if (evt.t === 'card') {
							const cards = lastMsg.toolCards ?? [];
							if (evt.v.type === 'combined_map') {
								// The server emits exactly one normalized map state per reply.
								lastMsg.toolCards = [...cards.filter((card) => !['map', 'route_map', 'vehicle_map', 'arrivals_route_map', 'combined_map'].includes(card.type)), evt.v];
							} else if (evt.v.type === 'arrivals_route_map') {
							// This map already contains the stop, routes, and incoming vehicles.
							// Replace all other maps emitted by overlapping model tool calls.
							lastMsg.toolCards = [...cards.filter((card) => !['map', 'route_map', 'vehicle_map'].includes(card.type)), evt.v];
							} else if (['map', 'route_map', 'vehicle_map'].includes(evt.v.type) && cards.some((card) => card.type === 'arrivals_route_map')) {
								// Ignore a generic map that arrives after the focused arrivals map.
								continue;
							} else if (COMBINABLE_MAP_TYPES.has(evt.v.type)) {
								// A location lookup and route lookup may arrive separately. Render their
								// markers and geometry on one map instead of stacking map cards.
								lastMsg.toolCards = [
									...cards.filter((card) => !COMBINABLE_MAP_TYPES.has(card.type)),
									coalesceMapCards(cards, evt.v)
								];
							} else {
							lastMsg.toolCards = [...cards, evt.v];
						}
					} else if (evt.t === 'map_suggestion') {
						lastMsg.mapSuggestion = evt.v;
					} else if (evt.t === 'error') {
						throw new Error(evt.v);
					} else if (evt.t === 'done') {
						done = true;
						break;
					}
				}
			}

			saveToStorage(messages);
		} catch (e) {
			if (e.name === 'AbortError') {
				// User stopped the response — keep whatever text arrived, don't show error
				saveToStorage(messages);
			} else {
				error = e.message;
				// Roll back: remove assistant shell (if added) and the user message
				const lastRole = messages.at(-1)?.role;
				messages = lastRole === 'assistant' ? messages.slice(0, -2) : messages.slice(0, -1);
				saveToStorage(messages);
				draft = text; // restore so the user can retry without retyping
			}
		} finally {
			loading   = false;
			streaming = false;
			_abort    = null;
		}
	}

	function clear() {
		messages  = [];
		error     = '';
		loading   = false;
		streaming = false;
		draft     = '';
		saveToStorage([]);
	}

	return {
		get messages()  { return messages;  },
		get loading()   { return loading;   },
		get streaming() { return streaming; },
		get error()     { return error;     },
		set error(v)    { error = v;        },
		get draft()     { return draft;     },
		set draft(v)    { draft = v;        },
		send,
		abort,
		clear
	};
}

export const chat = createChatStore();

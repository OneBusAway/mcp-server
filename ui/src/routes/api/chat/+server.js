import { error as httpError } from '@sveltejs/kit';
import Anthropic from '@anthropic-ai/sdk';
import { callTool as mcpCall, listTools } from '$lib/mcp.js';
import { settings, PROVIDERS } from '$lib/settings.svelte.js';

const SYSTEM = `You are a transit assistant with access to live OneBusAway transit data via tools.

RULES — follow these exactly:
1. ALWAYS call a tool to answer transit questions. Never guess or say "I'll check" without calling a tool first.
2. NEVER say "there was an issue" or "I couldn't find" without actually calling the tool and getting an error back.
3. If a tool returns an error or empty result, report exactly what it returned. Do not apologize or repeat the call.
4. Use stop IDs and route IDs exactly as given — do not modify them (e.g. "1_1" stays "1_1").
5. Format times clearly: "3:42 PM (in 8 min)". Be concise.
6. Map cards and arrival panels are shown automatically by the UI whenever a tool returns location or arrival data — never say you "cannot show on a map" or tell the user to use Google Maps.
7. When listing multiple stops, call get_stop for each one — the UI will offer to show them all on a single map after your response. Do not describe coordinates or tell the user to look at a map manually.
8. For a question about a specific stop (including “show incoming buses moving toward my stop”), call get_arrivals_for_stop first. Do NOT call get_vehicles_for_agency: it returns the entire fleet, is unnecessarily broad, and makes the answer less useful. The UI will show one map with only the arriving vehicles and their routes.
9. For current vehicles on named routes, call get_trips_for_route once per route. Do not call get_vehicles_for_agency unless the user explicitly asks for the agency's entire fleet.`;

const LOCAL_SYSTEM = `You are a transit assistant with access to live OneBusAway transit data via tools.

CRITICAL RULES:
- You MUST call a tool for every transit question. No exceptions.
- Call the tool NOW. Do not say you will call it. Do not explain what you are about to do.
- IDs from previous messages are real — use them directly. If the user said agency_id is "1", pass agency_id="1" to the tool.
- Use IDs exactly as returned by tools. "1_1" stays "1_1". "1" stays "1". Never add or remove prefixes.
- To list stops for an agency use get_stops_for_agency with the agency_id. Do NOT use search_stops for this.
- To list routes for an agency use get_routes_for_agency with the agency_id. Do NOT use search_routes for this.
- If the tool returns an error, quote the error. Do not retry or apologize.
- After getting tool results, answer directly. Do not ask follow-up questions.
- Map cards are shown automatically by the UI — NEVER say you cannot show a map or tell the user to use another app.
- When listing multiple stops, call get_stop for each one — the UI will offer to show them all on a map after your response.
- For a specific stop or its incoming buses, call get_arrivals_for_stop first. Never call get_vehicles_for_agency unless the user explicitly asks for all active vehicles in an agency.
- For current vehicles on one or more named routes, call get_trips_for_route for each requested route. Do not call get_vehicles_for_agency for this.
- Do not call a second arrival or overview tool for a stop after you already received its arrivals in this reply.
- Be short. One or two sentences max after the data.
/no_think`;

function getProviderCfg(id) {
	return PROVIDERS.find((p) => p.id === id) ?? PROVIDERS[0];
}

// Cache tool definitions for 5 minutes — they never change while oba-mcp is running
let _toolsCache = null;
let _toolsCacheAt = 0;
async function getToolDefs() {
	if (_toolsCache && Date.now() - _toolsCacheAt < 5 * 60_000) return _toolsCache;
	_toolsCache = await listTools();
	_toolsCacheAt = Date.now();
	return _toolsCache;
}

const enc = new TextEncoder();
function sse(controller, obj) {
	controller.enqueue(enc.encode(`data: ${JSON.stringify(obj)}\n\n`));
}

function isStopFocusedRequest(messages) {
	const latestUserText = [...messages]
		.reverse()
		.find((message) => message.role === 'user')?.content ?? '';
	// OBA stop IDs such as "1_1013" identify a single-stop arrival request.
	return /\b[\w-]+_\d+\b/.test(latestUserText);
}

// OBA exposes route geometry as a Google encoded polyline. MapLibre expects
// longitude/latitude coordinate pairs.
function decodePolyline(encoded) {
	const coordinates = [];
	let index = 0, lat = 0, lon = 0;
	while (index < encoded.length) {
		let result = 0, shift = 0, byte;
		do { byte = encoded.charCodeAt(index++) - 63; result |= (byte & 0x1f) << shift; shift += 5; } while (byte >= 0x20 && index < encoded.length);
		lat += result & 1 ? ~(result >> 1) : result >> 1;
		result = 0; shift = 0;
		do { byte = encoded.charCodeAt(index++) - 63; result |= (byte & 0x1f) << shift; shift += 5; } while (byte >= 0x20 && index < encoded.length);
		lon += result & 1 ? ~(result >> 1) : result >> 1;
		coordinates.push([lon / 1e5, lat / 1e5]);
	}
	return coordinates;
}

/** @type {import('./$types').RequestHandler} */
export async function POST({ request }) {
	const body = await request.json();
	const { messages } = body;

	const provider = body.provider ?? settings.provider ?? 'anthropic';
	const apiKey   = body.apiKey   ?? settings.apiKey    ?? '';
	const model    = body.model    ?? settings.model     ?? 'claude-haiku-4-5-20251001';
	const provCfg  = getProviderCfg(provider);

	if (!apiKey && provider !== 'ollama' && provider !== 'llama-server') {
		throw httpError(400, { error: `No API key set for ${provCfg.label}. Go to Settings.` });
	}

	// Tools a typical rider needs. Local models have limited context so we skip
	// low-frequency tools (get_block, get_shape, get_metadata, get_trip, get_schedule_for_route).
	const LOCAL_TOOLS = new Set([
		'get_agencies', 'get_agency',
		'get_stop', 'search_stops', 'find_stops_near_location', 'get_stop_overview',
		'get_stops_for_agency',
		'get_arrivals_for_stop', 'get_arrivals_for_location', 'get_arrival_and_departure_for_stop',
		'get_stop_schedule',
		'get_route', 'search_routes', 'get_routes_for_agency', 'get_routes_for_location',
		'get_stops_for_route',
		'get_trip_details', 'get_trip_for_vehicle', 'get_trips_for_route', 'get_vehicles_for_agency',
		'get_current_time',
	]);

	const isLocal = provCfg.id === 'ollama' || provCfg.id === 'llama-server';

	let tools;
	try {
		const defs = await getToolDefs();
		let filtered = isLocal ? defs.filter((t) => LOCAL_TOOLS.has(t.name)) : defs;
		if (isStopFocusedRequest(messages)) {
			// The arrival tool is the accurate, compact source for a named stop.
			// Do not let a model waste a tool round and context on an agency-wide fleet.
			filtered = filtered.filter((t) => t.name !== 'get_vehicles_for_agency');
		}
		tools = filtered.map((t) => ({
			name: t.name,
			description: t.description,
			input_schema: t.inputSchema ?? { type: 'object', properties: {} }
		}));
	} catch (e) {
		throw httpError(502, { error: `Cannot reach oba-mcp: ${e.message}` });
	}

	const stream = new ReadableStream({
		async start(controller) {
			try {
				const msgs = messages.map((m) => ({ role: m.role, content: m.content ?? m.text }));

				if (provider === 'anthropic') {
					await streamAnthropic({ apiKey, model, msgs, tools, controller });
				} else {
					await streamOpenAI({ provCfg, apiKey, model, msgs, tools, controller, isLocal });
				}

				sse(controller, { t: 'done' });
			} catch (e) {
				sse(controller, { t: 'error', v: e.message });
			} finally {
				controller.close();
			}
		}
	});

	return new Response(stream, {
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			'X-Accel-Buffering': 'no'
		}
	});
}

async function streamAnthropic({ apiKey, model, msgs, tools, controller }) {
	const client = new Anthropic({ apiKey });
	let currentMsgs = msgs;
	const mapState = createMapState();
	// Tracks what's already rendered this turn — prevents duplicate map cards
	const emitted = { routeIds: new Set(), stopIds: new Set(), arrivalStopIds: new Set() };

	for (let round = 0; round < 8; round++) {
		const stream = client.messages.stream({
			model,
			max_tokens: 1024,
			system: SYSTEM,
			tools,
			messages: currentMsgs
		});

		stream.on('text', (text) => { sse(controller, { t: 'text', v: text }); });

		const finalMsg = await stream.finalMessage();
		currentMsgs = [...currentMsgs, { role: 'assistant', content: finalMsg.content }];

		if (finalMsg.stop_reason === 'end_turn') break;

		if (finalMsg.stop_reason === 'tool_use') {
			const toolResults = [];
			for (const block of finalMsg.content) {
				if (block.type !== 'tool_use') continue;
				const result = await dispatchTool(block.name, block.input, controller, mapState, emitted);
				toolResults.push({ type: 'tool_result', tool_use_id: block.id, content: JSON.stringify(result) });
			}
			currentMsgs = [...currentMsgs, { role: 'user', content: toolResults }];
		} else {
			break;
		}
	}

	flushMapState(controller, mapState);
}

async function streamOpenAI({ provCfg, apiKey, model, msgs, tools, controller, isLocal }) {
	const baseURL     = provCfg.baseUrl ?? 'https://api.openai.com/v1';
	const openaiTools = tools.map((t) => ({
		type: 'function',
		function: { name: t.name, description: t.description, parameters: t.input_schema }
	}));

	const system = isLocal ? LOCAL_SYSTEM : SYSTEM;
	let oaiMsgs = [{ role: 'system', content: system }, ...msgs];
	const mapState = createMapState();
	const emitted = { routeIds: new Set(), stopIds: new Set(), arrivalStopIds: new Set() };

	for (let round = 0; round < 8; round++) {
		const res = await fetch(`${baseURL}/chat/completions`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				Authorization: `Bearer ${apiKey}`,
				'HTTP-Referer': 'http://localhost:5173',
				'X-Title': 'OBA Transit UI'
			},
			body: JSON.stringify({ model, messages: oaiMsgs, tools: openaiTools, max_tokens: 2048, stream: true })
		});

		if (!res.ok) {
			const err = await res.json().catch(() => ({}));
			throw new Error(err.error?.message ?? `Provider error: HTTP ${res.status}`);
		}

		const reader   = res.body.getReader();
		const decoder  = new TextDecoder();
		let buf          = '';
		let finishReason = null;
		const tcAccum    = {}; // index → { id, name, args }
		let assistantContent = '';
		let ended = false;

		while (!ended) {
			const { done, value } = await reader.read();
			if (done) break;

			buf += decoder.decode(value, { stream: true });
			const lines = buf.split('\n');
			buf = lines.pop() ?? '';

			for (const line of lines) {
				if (!line.startsWith('data: ')) continue;
				const raw = line.slice(6).trim();
				if (raw === '[DONE]') { ended = true; break; }

				let chunk;
				try { chunk = JSON.parse(raw); } catch { continue; }

				const delta = chunk.choices?.[0]?.delta;
				const fr    = chunk.choices?.[0]?.finish_reason;
				if (fr) finishReason = fr;

				if (delta?.content) {
					assistantContent += delta.content;
					sse(controller, { t: 'text', v: delta.content });
				}

				if (delta?.tool_calls) {
					for (const tc of delta.tool_calls) {
						if (!tcAccum[tc.index]) tcAccum[tc.index] = { id: '', name: '', args: '' };
						if (tc.id)                  tcAccum[tc.index].id   = tc.id;
						if (tc.function?.name)      tcAccum[tc.index].name += tc.function.name;
						if (tc.function?.arguments) tcAccum[tc.index].args += tc.function.arguments;
					}
				}
			}
		}

		const toolCallsList = Object.values(tcAccum);
		if (!toolCallsList.length || finishReason === 'stop') break;

		const formattedTCs = toolCallsList.map((tc) => ({
			id: tc.id, type: 'function',
			function: { name: tc.name, arguments: tc.args }
		}));

		oaiMsgs = [...oaiMsgs, {
			role: 'assistant',
			content: assistantContent || null,
			tool_calls: formattedTCs
		}];

		const toolMsgs = [];
		for (const tc of formattedTCs) {
			let args;
			try { args = JSON.parse(tc.function.arguments); } catch { args = {}; }
			const result = await dispatchTool(tc.function.name, args, controller, mapState, emitted);
			toolMsgs.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
		}
		oaiMsgs = [...oaiMsgs, ...toolMsgs];
	}

	flushMapState(controller, mapState);
}

function toMapMarkers(stops) {
	return (Array.isArray(stops) ? stops : [])
		.filter((s) => s.lat && s.lon)
		.map((s) => ({ lat: s.lat, lon: s.lon, name: s.name, id: s.id }));
}

function routeStopMarkers(route) {
	return (route?.directions ?? [])
		.flatMap((direction) => direction.stops ?? [])
		.map((stop) => ({ id: stop.id, lat: stop.lat, lon: stop.lon, name: stop.name }));
}

function markCurrentStop(markers) {
	if (markers.length === 1) markers[0].is_current = true;
	return markers;
}

function createMapState() {
	return {
		markers: [], directions: [], vehicles: [], routeIds: [], vehicleTripIds: [], tripInfo: {},
		markerKeys: new Set(), markerIndexes: new Map(), currentMarkerKey: null, directionKeys: new Set(), vehicleKeys: new Set(), routeIdSet: new Set(), tripIds: new Set()
	};
}

function addMarkers(mapState, markers) {
	for (const marker of markers) {
		if (marker?.lat == null || marker?.lon == null) continue;
		const key = marker.id ?? `${marker.lat},${marker.lon}`;
		const isCurrent = marker.is_current && (!mapState.currentMarkerKey || mapState.currentMarkerKey === key);
		if (mapState.markerKeys.has(key)) {
			const index = mapState.markerIndexes.get(key);
			if (isCurrent && !mapState.markers[index].is_current) {
				mapState.markers[index] = { ...mapState.markers[index], ...marker, is_current: true };
				mapState.currentMarkerKey = key;
			}
			continue;
		}
		mapState.markerKeys.add(key);
		mapState.markerIndexes.set(key, mapState.markers.length);
		mapState.markers.push({ ...marker, is_current: isCurrent });
		if (isCurrent) mapState.currentMarkerKey = key;
	}
}

function addDirections(mapState, directions) {
	for (const direction of directions) {
		const coordinates = direction?.coordinates ?? [];
		if (coordinates.length < 2) continue;
		const key = `${direction.direction ?? ''}:${JSON.stringify(coordinates)}`;
		if (mapState.directionKeys.has(key)) continue;
		mapState.directionKeys.add(key);
		mapState.directions.push(direction);
	}
}

function addVehicles(mapState, vehicles) {
	for (const vehicle of vehicles) {
		if (vehicle?.lat == null || vehicle?.lon == null) continue;
		const key = vehicle.vehicle_id ?? vehicle.trip_id ?? `${vehicle.lat},${vehicle.lon}`;
		if (mapState.vehicleKeys.has(key)) continue;
		mapState.vehicleKeys.add(key);
		mapState.vehicles.push(vehicle);
	}
}

function addRouteId(mapState, routeId) {
	if (!routeId || mapState.routeIdSet.has(routeId)) return;
	mapState.routeIdSet.add(routeId);
	mapState.routeIds.push(routeId);
}

function addRouteData(mapState, routeId, route) {
	const routeStops = routeStopMarkers(route);
	addMarkers(mapState, routeStops);
	const directions = (route?.polylines ?? []).flatMap((polyline) => {
		const coordinates = decodePolyline(polyline.encoded_polyline ?? '');
		return coordinates.length >= 2 ? [{ direction: routeId, coordinates }] : [];
	});
	addDirections(mapState, directions);
	if (routeStops.length || directions.length) addRouteId(mapState, routeId);
	return routeStops.length > 0 || directions.length > 0;
}

async function addRouteToMap(mapState, routeId, emitted) {
	if (!routeId || emitted.routeIds.has(routeId)) return;
	const route = await mcpCall('get_stops_for_route', { route_id: routeId });
	if (addRouteData(mapState, routeId, route)) emitted.routeIds.add(routeId);
}

function flushMapState(controller, mapState) {
	if (!mapState.markers.length && !mapState.directions.length && !mapState.vehicles.length) return;
	sse(controller, { t: 'card', v: {
		type: 'combined_map',
		markers: mapState.markers,
		directions: mapState.directions,
		vehicles: mapState.vehicles,
		route_ids: mapState.routeIds,
		trip_ids: mapState.vehicleTripIds,
		trip_info: mapState.tripInfo,
	} });
}

async function dispatchTool(name, input, controller, mapState = createMapState(), emitted = null) {
	const _e = emitted ?? { routeIds: new Set(), stopIds: new Set(), arrivalStopIds: new Set() };
	let result;
	try {
		result = await mcpCall(name, input);

		if (name === 'get_arrivals_for_stop' && (Array.isArray(result) || result?.arrivals)) {
			// Tool returns a plain array; result.arrivals is a legacy fallback
			const arrivals = Array.isArray(result) ? result : result.arrivals;
			const stopName = result?.stop_name ?? input.stop_id;
			if (!_e.arrivalStopIds.has(input.stop_id)) {
				sse(controller, { t: 'card', v: { type: 'arrivals', stop_id: input.stop_id, stop_name: stopName, arrivals } });
				_e.arrivalStopIds.add(input.stop_id);
			}

			// Emit one focused map for the live arrivals at this stop. GPS is optional:
			// MapCard fetches the selected trips' positions when the arrival result omits it.
			const predictedArrivals = arrivals.filter(a => a.predicted && a.trip_id);
			const vehicledArrivals  = predictedArrivals.filter(a => a.vehicle_lat && a.vehicle_lon);
			if (predictedArrivals.length) {
				// WayFinder draws entry.polylines from stops-for-route. They are the
				// canonical OBA route shapes; never rebuild a route by joining stops.
				const routeIds = [...new Set(predictedArrivals.map((a) => a.route_id).filter(Boolean))].slice(0, 3);
				const [stopRes, ...routeResults] = await Promise.allSettled([
					mcpCall('get_stop', { stop_id: input.stop_id }),
					...routeIds.map((route_id) => mcpCall('get_stops_for_route', { route_id }))
				]);
				let stopLat = null, stopLon = null;
				if (stopRes.status === 'fulfilled' && stopRes.value?.lat) {
					stopLat = stopRes.value.lat;
					stopLon = stopRes.value.lon;
				}
				const routeNames = new Map(predictedArrivals.map((a) => [a.route_id, a.route_name ?? a.route_id]));
				const directions = routeResults.flatMap((routeRes, index) => {
					const route = routeRes.status === 'fulfilled' ? routeRes.value : null;
					addMarkers(mapState, routeStopMarkers(route));
					const polylines = route?.polylines ?? [];
					const routeName = routeNames.get(routeIds[index]) ?? routeIds[index];
					return polylines.flatMap((polyline) => {
						const coordinates = decodePolyline(polyline.encoded_polyline ?? '');
						return coordinates.length >= 2 ? [{ direction: routeName, coordinates }] : [];
					});
				});
				// trip_info lets MapCard build proper route badges when fetching trip_details for GPS
				const trip_info = {};
				for (const a of predictedArrivals) {
					if (a.trip_id) trip_info[a.trip_id] = { route_short_name: a.route_name, headsign: a.headsign };
				}
				if (stopLat != null) {
					addMarkers(mapState, [{ id: input.stop_id, lat: stopLat, lon: stopLon, name: stopName, is_current: true }]);
				}
				addDirections(mapState, directions);
				addVehicles(mapState, vehicledArrivals.map(a => ({
					lat: a.vehicle_lat,
					lon: a.vehicle_lon,
					vehicle_id: a.vehicle_id,
					trip_id: a.trip_id,
					route_id: a.route_id,
					route_short_name: a.route_name,
					headsign: a.headsign,
					stops_away: a.number_of_stops_away,
					bearing: a.vehicle_bearing ?? null,
				})));
				for (const tripId of predictedArrivals.map(a => a.trip_id).filter(Boolean)) {
					if (!mapState.tripIds.has(tripId)) {
						mapState.tripIds.add(tripId);
						mapState.vehicleTripIds.push(tripId);
					}
				}
				Object.assign(mapState.tripInfo, trip_info);
				// Register so subsequent tool calls don't add duplicate geometry or markers.
				for (const arrival of predictedArrivals) {
					addRouteId(mapState, arrival.route_id);
					_e.routeIds.add(arrival.route_id);
				}
				_e.stopIds.add(input.stop_id);
			}

		} else if (name === 'find_stops_near_location' && Array.isArray(result) && result.length) {
			addMarkers(mapState, markCurrentStop(toMapMarkers(result)));

		} else if (name === 'search_stops' && Array.isArray(result) && result.length) {
			addMarkers(mapState, markCurrentStop(toMapMarkers(result)));

		} else if (name === 'get_stop' && result?.lat) {
			addMarkers(mapState, [{ id: input.stop_id, lat: result.lat, lon: result.lon, name: result.name ?? input.stop_id, is_current: true }]);

		} else if (name === 'get_stop_overview') {
			const stopName = result?.stop_name ?? input.stop_id;
			if (result?.lat) addMarkers(mapState, [{ id: input.stop_id, lat: result.lat, lon: result.lon, name: stopName, is_current: true }]);
			if (result?.next_arrivals?.length && !_e.arrivalStopIds.has(input.stop_id)) {
				sse(controller, { t: 'card', v: { type: 'arrivals', stop_id: input.stop_id, stop_name: stopName, arrivals: result.next_arrivals } });
				_e.arrivalStopIds.add(input.stop_id);
			}

		} else if (name === 'get_stops_for_route' && (Array.isArray(result) || result?.directions)) {
			if (!_e.routeIds.has(input.route_id)) {
				if (addRouteData(mapState, input.route_id, result)) _e.routeIds.add(input.route_id);
			}

		} else if (name === 'get_trips_for_route') {
			if (Array.isArray(result)) {
				addVehicles(mapState, result.map((trip) => ({
					...trip,
					route_id: input.route_id,
					route_short_name: String(input.route_id).split('_').at(-1),
				})));
			}
			await addRouteToMap(mapState, input.route_id, _e);

		} else if (name === 'get_trip_details' && result?.lat != null && result?.lon != null) {
			addVehicles(mapState, [{ ...result, trip_id: result.trip_id ?? input.trip_id }]);
			await addRouteToMap(mapState, result.route_id, _e);

		} else if (name === 'get_vehicles_for_agency' && Array.isArray(result) && result.length) {
			// Agency-wide fleet data is a discovery result, not a map result. The model
			// must confirm a vehicle's requested route via get_trips_for_route or trip details.

		} else if (name === 'get_trips_for_location' && Array.isArray(result) && result.length) {
			addVehicles(mapState, result.filter(v => v.lat && v.lon));

		} else if (
			(name === 'search_routes' || name === 'get_routes_for_agency' || name === 'get_routes_for_location') &&
			Array.isArray(result) && result.length
		) {
			const routes = result.slice(0, 6).map((r) => ({
				id: r.id,
				name: r.short_name ?? r.id,
				description: r.long_name ?? '',
				color: r.color || null
			}));
			sse(controller, { t: 'card', v: { type: 'route_suggestions', routes } });
		}
	} catch (e) {
		result = { error: e.message };
	}
	return result;
}

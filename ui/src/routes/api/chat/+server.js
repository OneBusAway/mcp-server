import { error as httpError } from '@sveltejs/kit';
import Anthropic from '@anthropic-ai/sdk';
import { createMCPClient } from '$lib/mcp.js';
import { settings, PROVIDERS } from '$lib/settings.svelte.js';
import { sse } from './stream.js';
import { dispatchTool, createMapState } from './dispatch.js';
import { flushMapState } from './map.js';

// ---------------------------------------------------------------------------
// System prompts
// ---------------------------------------------------------------------------

const SYSTEM = `CRITICAL — OUTPUT FORMAT: You output ONLY the direct answer to the user. No reasoning. No planning. No "Step 1:". No "Following the rules:". No "I must call". No "The tool returned". No numbered steps. No explanation of what you are doing. If your response contains any of those patterns it is wrong. Respond as if you already know the answer and are just telling the user.

You are onebusaway-transit-asssit with access to live OneBusAway transit data via tools.

RULES — follow these exactly:
1. ALWAYS call a tool to answer transit questions. Never guess or say "I'll check" without calling a tool first.
2. NEVER say "there was an issue" or "I couldn't find" without actually calling the tool and getting an error back.
3. If a tool returns an error or empty result, report exactly what it returned. Do not apologize or repeat the call.
4. Use stop IDs and route IDs exactly as given — do not modify them (e.g. "1_1" stays "1_1").
5. Format times clearly: "3:42 PM (in 8 min)". Be concise.
6. Start with the answer; never narrate actions such as "I'll check" or "I'll get the details."
7. In normal rider answers, never use the words UI, map, card, tool, display, render, or automatic. Do not explain internal behavior or capabilities.
8. Do not include latitude or longitude unless the user explicitly asks for coordinates.
9. For a question about a specific stop (including "show incoming buses moving toward my stop"), call get_arrivals_for_stop first. Do NOT call get_vehicles_for_agency: it returns the entire fleet and makes the answer less useful.
10. For current vehicles on named routes, call get_trips_for_route once per route. Do not call get_vehicles_for_agency unless the user explicitly asks for the agency's entire fleet.
11. When the user names a stop by TEXT (e.g. "E Harrison St @ Hank Ballard WB", "3rd & Main", "downtown transit center") and NOT by an ID like "1_1", call search_stops with query=<that name> first. If it returns exactly one stop, use that stop_id with get_arrivals_for_stop. If it returns multiple stops, DO NOT choose one and DO NOT call another tool: ask one short clarification question listing the matching stop names and IDs so the user can choose. Do NOT call find_stops_near_location, get_arrivals_for_location, get_stops_for_agency, or get_routes_for_agency for a named stop — the first two need coordinates you don't have, and the others return oversized lists.`;

const LOCAL_SYSTEM = `You are onebusaway-transit-asssit with access to live OneBusAway transit data via tools.

CRITICAL RULES:
- You MUST call a tool for every transit question. No exceptions.
- Call the tool NOW. Do not say you will call it. Do not explain what you are about to do.
- IDs from previous messages are real — use them directly. If the user said agency_id is "1", pass agency_id="1" to the tool.
- Use IDs exactly as returned by tools. "1_1" stays "1_1". "1" stays "1". Never add or remove prefixes.
- To list stops for an agency use get_stops_for_agency with the agency_id. Do NOT use search_stops for this.
- To list routes for an agency use get_routes_for_agency with the agency_id. Do NOT use search_routes for this.
- If the tool returns an error, quote the error. Do not retry or apologize.
- After getting tool results, answer directly. Do not ask follow-up questions.
- Start with the answer; never narrate actions such as "I'll check" or "I'll get the details."
- In normal rider answers, never use the words UI, map, card, tool, display, render, or automatic. Do not explain internal behavior or capabilities.
- Do not include latitude or longitude unless the user explicitly asks for coordinates.
- For a specific stop or its incoming buses, call get_arrivals_for_stop first. Never call get_vehicles_for_agency unless the user explicitly asks for all active vehicles in an agency.
- For current vehicles on one or more named routes, call get_trips_for_route for each requested route. Do not call get_vehicles_for_agency for this.
- If the user names a stop by TEXT like "E Harrison St @ Hank Ballard WB" or "3rd & Main" (no "_" in it, so not an ID), first call search_stops with query=<the exact stop text>. If there is exactly one result, call get_arrivals_for_stop with its stop_id. If there are multiple results, STOP: do not select the first result and do not call another tool. Ask one short question listing each matching stop name and ID and let the user choose. NEVER call find_stops_near_location, get_arrivals_for_location, get_stops_for_agency, or get_routes_for_agency for a named stop — they require coordinates you do not have or return oversized lists.
- Do not call a second arrival or overview tool for a stop after you already received its arrivals in this reply.
- Be short. One or two sentences max after the data.
/no_think`;

// ---------------------------------------------------------------------------
// Tool list cache (5-minute TTL — tools never change while oba-mcp is running)
// ---------------------------------------------------------------------------

// Rider-facing tool allowlist. Applied to all providers — the MCP server may
// expose more tools (get_block, get_shape, get_metadata, etc.) but the
// passenger UI has no use for them and their presence increases token cost
// and tool-selection ambiguity.
const RIDER_TOOLS = new Set([
	'get_agencies',
	'get_agency',
	'get_stop',
	'search_stops',
	'find_stops_near_location',
	'get_stop_overview',
	'get_stops_for_agency',
	'get_arrivals_for_stop',
	'get_arrivals_for_location',
	'get_arrival_and_departure_for_stop',
	'get_stop_schedule',
	'get_route',
	'search_routes',
	'get_routes_for_agency',
	'get_routes_for_location',
	'get_stops_for_route',
	'get_trip_details',
	'get_trip_for_vehicle',
	'get_trips_for_route',
	'get_vehicles_for_agency',
	'get_current_time',
]);

let _toolsCache = null;
let _toolsCacheAt = 0;

const ARRIVAL_FOCUSED_TOOLS = new Set(['search_stops', 'get_arrivals_for_stop']);

async function getToolDefs(mcp, arrivalFocused, allTools = false) {
	if (!_toolsCache || Date.now() - _toolsCacheAt >= 5 * 60_000) {
		_toolsCache = await mcp.listTools();
		_toolsCacheAt = Date.now();
	}
	let filtered = allTools ? _toolsCache : _toolsCache.filter((t) => RIDER_TOOLS.has(t.name));
	// A named-stop arrival request has one deterministic flow: resolve the stop,
	// then fetch its arrivals. Restricting this small turn-specific tool set is
	// especially important for local models, which otherwise confuse a trip
	// detail lookup with the complete arrivals list.
	if (arrivalFocused) filtered = filtered.filter((t) => ARRIVAL_FOCUSED_TOOLS.has(t.name));
	return filtered.map((t) => ({
		name: t.name,
		description: t.description,
		input_schema: t.inputSchema ?? { type: 'object', properties: {} },
	}));
}

// ---------------------------------------------------------------------------
// Request handler
// ---------------------------------------------------------------------------

function getProviderCfg(id) {
	return PROVIDERS.find((p) => p.id === id) ?? PROVIDERS[0];
}

function isArrivalFocusedRequest(messages) {
	const latestUserText = [...messages].reverse().find((m) => m.role === 'user')?.content ?? '';
	const asksForArrivals =
		/\b(arrivals?|arriv(?:e|ing)|upcoming|next\s+(?:bus|buses)|incoming)\b/i.test(latestUserText);
	const identifiesStop = /\b[\w-]+_\d+\b/.test(latestUserText) || /\s[@&]\s/.test(latestUserText);
	return asksForArrivals && identifiesStop;
}

/** @type {import('./$types').RequestHandler} */
export async function POST({ request, fetch }) {
	const body = await request.json();
	const { messages } = body;

	const provider = body.provider ?? settings.provider ?? 'anthropic';
	const apiKey = body.apiKey ?? settings.apiKey ?? '';
	const model = body.model ?? settings.model ?? 'claude-haiku-4-5-20251001';
	const allTools = (body.toolMode ?? 'rider') === 'all';
	const provCfg = getProviderCfg(provider);

	if (!apiKey && !provCfg.local) {
		throw httpError(
			400,
			/** @type {any} */ ({ error: `No API key set for ${provCfg.label}. Go to Settings.` }),
		);
	}

	const isLocal = provCfg.local === true;
	const mcp = createMCPClient(fetch);
	let tools;
	try {
		tools = await getToolDefs(mcp, isArrivalFocusedRequest(messages), allTools);
	} catch (e) {
		throw httpError(502, /** @type {any} */ ({ error: `Cannot reach oba-mcp: ${e.message}` }));
	}

	const stream = new ReadableStream({
		async start(controller) {
			try {
				const msgs = messages.map((m) => ({ role: m.role, content: m.content ?? m.text }));
				if (provider === 'anthropic') {
					await streamAnthropic({ apiKey, model, msgs, tools, controller, mcp });
				} else {
					await streamOpenAI({ provCfg, apiKey, model, msgs, tools, controller, isLocal, mcp });
				}
				sse(controller, { t: 'done' });
			} catch (e) {
				sse(controller, { t: 'error', v: e.message });
			} finally {
				controller.close();
			}
		},
	});

	return new Response(stream, {
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			'X-Accel-Buffering': 'no',
		},
	});
}

// ---------------------------------------------------------------------------
// Anthropic streaming
// ---------------------------------------------------------------------------

async function streamAnthropic({ apiKey, model, msgs, tools, controller, mcp }) {
	const client = new Anthropic({ apiKey });
	let currentMsgs = msgs;
	const mapState = createMapState();
	const emitted = { routeIds: new Set(), stopIds: new Set(), arrivalStopIds: new Set() };

	for (let round = 0; round < 8; round++) {
		const stream = client.messages.stream({
			model,
			max_tokens: 1024,
			system: SYSTEM,
			tools,
			messages: currentMsgs,
		});

		stream.on('text', (text) => sse(controller, { t: 'text', v: text }));

		const finalMsg = await stream.finalMessage();
		currentMsgs = [...currentMsgs, { role: 'assistant', content: finalMsg.content }];

		if (finalMsg.stop_reason === 'end_turn') break;

		if (finalMsg.stop_reason === 'tool_use') {
			const toolResults = [];
			for (const block of finalMsg.content) {
				if (block.type !== 'tool_use') continue;
				const result = await dispatchTool(
					block.name,
					block.input,
					controller,
					sse,
					mapState,
					emitted,
					mcp,
				);
				toolResults.push({
					type: 'tool_result',
					tool_use_id: block.id,
					content: JSON.stringify(result),
				});
			}
			currentMsgs = [...currentMsgs, { role: 'user', content: toolResults }];
		} else {
			break;
		}
	}

	flushMapState(controller, mapState, sse);
}

// ---------------------------------------------------------------------------
// OpenAI-compatible streaming
// ---------------------------------------------------------------------------
const _GOOGLE_SCHEMA_ALLOWED = new Set([
	'type',
	'description',
	'properties',
	'required',
	'items',
	'enum',
	'nullable',
	'format',
]);

function _sanitizeSchemaForGoogle(schema) {
	if (!schema || typeof schema !== 'object') return schema;
	if (Array.isArray(schema)) return schema.map(_sanitizeSchemaForGoogle);
	const result = {};
	for (const [k, v] of Object.entries(schema)) {
		if (!_GOOGLE_SCHEMA_ALLOWED.has(k)) continue;
		// `properties` maps property NAMES → sub-schemas; preserve names, sanitize values only.
		if (k === 'properties' && v && typeof v === 'object' && !Array.isArray(v)) {
			result.properties = Object.fromEntries(
				Object.entries(v).map(([name, sub]) => [name, _sanitizeSchemaForGoogle(sub)]),
			);
		} else {
			result[k] = _sanitizeSchemaForGoogle(v);
		}
	}
	return result;
}

const _DEFAULT_ADAPTER = {
	sanitizeSchema: (s) => s,
	extraBody: () => ({}),
	extractToolSig: (/** @type {any} */ _tc) => null,
	embedToolSig: (/** @type {any} */ _sig) => ({}),
};

const PROVIDER_ADAPTERS = {
	'google-ai-studio': {
		sanitizeSchema: _sanitizeSchemaForGoogle,
		extraBody: () => ({}),
		// Gemini thinking models return thought_signature under extra_content.google
		extractToolSig: (tc) => tc.extra_content?.google?.thought_signature ?? null,
		embedToolSig: (sig) => ({ extra_content: { google: { thought_signature: sig } } }),
	},
	openrouter: {
		..._DEFAULT_ADAPTER,
		// Ask OpenRouter to strip reasoning tokens from the stream
		extraBody: () => ({ reasoning: { exclude: true } }),
	},
};

function getProviderAdapter(id) {
	return PROVIDER_ADAPTERS[id] ?? _DEFAULT_ADAPTER;
}

async function streamOpenAI({ provCfg, apiKey, model, msgs, tools, controller, isLocal, mcp }) {
	const baseURL = provCfg.baseUrl ?? 'https://api.openai.com/v1';
	const adapter = getProviderAdapter(provCfg.id);
	const openaiTools = tools.map((t) => ({
		type: 'function',
		function: {
			name: t.name,
			description: t.description,
			parameters: adapter.sanitizeSchema(t.input_schema),
		},
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
				'X-Title': 'OBA Transit UI',
			},
			body: JSON.stringify({
				model,
				messages: oaiMsgs,
				tools: openaiTools,
				max_tokens: 2048,
				stream: true,
				...adapter.extraBody(),
			}),
		});

		if (!res.ok) {
			const body = await res.text().catch(() => '');
			let errMsg;
			try {
				errMsg = JSON.parse(body)?.error?.message;
			} catch {}
			console.error(`[${provCfg.id}] HTTP ${res.status}:`, errMsg ?? body.slice(0, 400));
			throw new Error(errMsg ?? `${provCfg.label} error: HTTP ${res.status}`);
		}

		// Some providers return a JSON error body with HTTP 200 when they can't stream.
		const ct = res.headers.get('content-type') ?? '';
		if (!ct.includes('event-stream') && !ct.includes('octet-stream')) {
			const body = await res.text().catch(() => '');
			let errMsg;
			try {
				errMsg = JSON.parse(body)?.error?.message;
			} catch {}
			console.error(`[${provCfg.id}] non-stream 200 body:`, body.slice(0, 400));
			throw new Error(
				errMsg ??
					`${provCfg.label} returned a non-streaming response — check your API key and model name.`,
			);
		}

		const { assistantContent, toolCalls } = await readOpenAIStream(res, controller, adapter);

		if (!toolCalls.length) break;

		oaiMsgs = [
			...oaiMsgs,
			{
				role: 'assistant',
				content: assistantContent || null,
				tool_calls: toolCalls,
			},
		];

		const toolMsgs = [];
		for (const tc of toolCalls) {
			let args;
			try {
				args = JSON.parse(tc.function.arguments);
			} catch {
				args = {};
			}
			const result = await dispatchTool(
				tc.function.name,
				args,
				controller,
				sse,
				mapState,
				emitted,
				mcp,
			);
			toolMsgs.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
		}
		oaiMsgs = [...oaiMsgs, ...toolMsgs];
	}

	flushMapState(controller, mapState, sse);
}

async function readOpenAIStream(res, controller, adapter = _DEFAULT_ADAPTER) {
	const reader = res.body.getReader();
	const decoder = new TextDecoder();
	let buf = '';
	let finishReason = null;
	let assistantContent = '';
	let ended = false;
	let inThink = false;
	const tcParts = {}; // index → { id, name, args }

	while (!ended) {
		const { done, value } = await reader.read();
		if (done) break;

		buf += decoder.decode(value, { stream: true });
		const lines = buf.split('\n');
		buf = lines.pop() ?? '';

		for (const line of lines) {
			if (!line.startsWith('data: ')) continue;
			const raw = line.slice(6).trim();
			if (raw === '[DONE]') {
				ended = true;
				break;
			}

			let chunk;
			try {
				chunk = JSON.parse(raw);
			} catch {
				continue;
			}

			// Some providers embed errors inside the SSE stream.
			if (chunk.error) throw new Error(chunk.error.message ?? JSON.stringify(chunk.error));

			const delta = chunk.choices?.[0]?.delta;
			const fr = chunk.choices?.[0]?.finish_reason;
			if (fr) finishReason = fr;

			if (delta?.content) {
				let text = delta.content;
				if (inThink) {
					const end = text.indexOf('</think>');
					if (end >= 0) {
						inThink = false;
						text = text.slice(end + 8);
					} else {
						text = '';
					}
				}
				if (text) {
					const start = text.indexOf('<think>');
					if (start >= 0) {
						const end = text.indexOf('</think>', start);
						if (end >= 0) {
							text = text.slice(0, start) + text.slice(end + 8);
						} else {
							inThink = true;
							text = text.slice(0, start);
						}
					}
				}
				if (text) {
					assistantContent += text;
					sse(controller, { t: 'text', v: text });
				}
			}

			if (delta?.tool_calls) {
				for (const tc of delta.tool_calls) {
					if (!tcParts[tc.index]) tcParts[tc.index] = { id: '', name: '', args: '', sig: null };
					if (tc.id) tcParts[tc.index].id = tc.id;
					if (tc.function?.name) tcParts[tc.index].name += tc.function.name;
					if (tc.function?.arguments) tcParts[tc.index].args += tc.function.arguments;
					const sig = adapter.extractToolSig(tc);
					if (sig) tcParts[tc.index].sig = sig;
				}
			}
		}
	}

	const toolCalls = Object.values(tcParts).map((tc) => ({
		id: tc.id,
		type: 'function',
		function: { name: tc.name, arguments: tc.args },
		...(tc.sig && adapter.embedToolSig(tc.sig)),
	}));

	return { assistantContent, finishReason, toolCalls };
}

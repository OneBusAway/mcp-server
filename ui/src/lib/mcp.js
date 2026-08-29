import { settings } from './settings.svelte.js';

let sessionId  = null;
let initPromise = null; // ensures we only initialize once at a time

function mcpHeaders() {
	const h = {
		'Content-Type': 'application/json',
		Accept: 'application/json, text/event-stream'
	};
	if (sessionId) h['Mcp-Session-Id'] = sessionId;
	return h;
}

function captureSession(res) {
	const sid = res.headers.get('Mcp-Session-Id');
	if (sid) sessionId = sid;
}

function parseResponse(text) {
	if (!text?.trim()) throw new Error('Empty response from oba-mcp');

	// SSE: lines starting with "data: "
	if (text.trimStart().startsWith('data:')) {
		const dataLines = text
			.split('\n')
			.filter((l) => l.startsWith('data:'))
			.map((l) => l.slice(5).trim())
			.filter(Boolean);
		if (!dataLines.length) throw new Error('Empty SSE stream');
		// The last non-empty data line is the result
		return JSON.parse(dataLines[dataLines.length - 1]);
	}

	return JSON.parse(text);
}

async function doInit() {
	const res = await fetch(`${settings.mcpUrl}/mcp`, {
		method: 'POST',
		headers: mcpHeaders(),
		body: JSON.stringify({
			jsonrpc: '2.0',
			id: 'init-0',
			method: 'initialize',
			params: {
				protocolVersion: '2024-11-05',
				capabilities: {},
				clientInfo: { name: 'transit-ui', version: '0.1.0' }
			}
		})
	});
	captureSession(res);
	// Consume body (needed for keep-alive)
	await res.text();
}

async function ensureInit() {
	if (sessionId) return;
	if (!initPromise) initPromise = doInit().catch((e) => { initPromise = null; throw e; });
	await initPromise;
}

/**
 * Call any oba-mcp tool over the StreamableHTTP transport.
 * @param {string} name
 * @param {Record<string, unknown>} args
 * @returns {Promise<any>}
 */
export async function callTool(name, args = {}) {
	await ensureInit();

	const res = await fetch(`${settings.mcpUrl}/mcp`, {
		method: 'POST',
		headers: mcpHeaders(),
		body: JSON.stringify({
			jsonrpc: '2.0',
			id: crypto.randomUUID(),
			method: 'tools/call',
			params: { name, arguments: args }
		})
	});
	captureSession(res);

	const text = await res.text();
	const json = parseResponse(text);

	if (json.error) throw new Error(json.error.message ?? JSON.stringify(json.error));

	const raw = json.result?.content?.[0]?.text ?? 'null';

	// Tool results are "Header text:\n{...}" or "Header text:\n[...]"
	const jsonStart = raw.search(/\n[{\[]/);
	if (jsonStart !== -1) {
		try { return JSON.parse(raw.slice(jsonStart + 1)); } catch {}
	}
	try { return JSON.parse(raw); } catch { return raw; }
}

/**
 * Fetch the tool list from oba-mcp.
 * @returns {Promise<Array>}
 */
export async function listTools() {
	await ensureInit();

	const res = await fetch(`${settings.mcpUrl}/mcp`, {
		method: 'POST',
		headers: mcpHeaders(),
		body: JSON.stringify({ jsonrpc: '2.0', id: 'list-tools', method: 'tools/list', params: {} })
	});
	captureSession(res);

	const text = await res.text();
	const json = parseResponse(text);
	return json.result?.tools ?? [];
}

/** Reset session (call when server URL changes in settings). */
export function resetSession() {
	sessionId   = null;
	initPromise = null;
}

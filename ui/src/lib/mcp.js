import { parseStructured } from './mcp/schemas.js';

const mcpProxyPath = '/api/mcp';

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

	try {
		return JSON.parse(text);
	} catch {
		throw new Error(`Unexpected non-JSON response from oba-mcp: ${text.slice(0, 120)}`);
	}
}

/** Create an MCP client bound to one browser or server request context. */
export function createMCPClient(fetchImpl) {
	let sessionId = null;
	let initPromise = null;

	function mcpHeaders() {
		const headers = {
			'Content-Type': 'application/json',
			Accept: 'application/json, text/event-stream',
		};
		if (sessionId) headers['Mcp-Session-Id'] = sessionId;
		return headers;
	}

	function captureSession(response) {
		const sid = response.headers.get('Mcp-Session-Id');
		if (sid) sessionId = sid;
	}

	async function doInit() {
		const response = await fetchImpl(mcpProxyPath, {
			method: 'POST',
			headers: mcpHeaders(),
			body: JSON.stringify({
				jsonrpc: '2.0',
				id: 'init-0',
				method: 'initialize',
				params: {
					protocolVersion: '2024-11-05',
					capabilities: {},
					clientInfo: { name: 'transit-ui', version: '0.1.0' },
				},
			}),
		});
		captureSession(response);
		await response.text();
	}

	async function ensureInit() {
		if (sessionId) return;
		if (!initPromise)
			initPromise = doInit().catch((error) => {
				initPromise = null;
				throw error;
			});
		await initPromise;
	}

	async function callTool(name, args = {}, { retryOnStaleSession = true } = {}) {
		await ensureInit();
		const response = await fetchImpl(mcpProxyPath, {
			method: 'POST',
			headers: mcpHeaders(),
			body: JSON.stringify({
				jsonrpc: '2.0',
				id: crypto.randomUUID(),
				method: 'tools/call',
				params: { name, arguments: args },
			}),
		});
		captureSession(response);

		// A server restart or session TTL invalidates our cached session id.
		// Upstream MCP replies "Invalid session ID" as plain text with HTTP 404,
		// which would otherwise crash parseResponse.
		if (response.status === 404 && retryOnStaleSession) {
			sessionId = null;
			initPromise = null;
			return callTool(name, args, { retryOnStaleSession: false });
		}

		const result = parseResponse(await response.text());
		if (result.error) throw new Error(result.error.message ?? JSON.stringify(result.error));

		const structured = result.result?.structuredContent;
		if (structured == null) {
			throw new Error(`Tool "${name}" returned no structuredContent`);
		}
		return parseStructured(structured, result.result?.isError === true);
	}

	async function listTools() {
		await ensureInit();
		const response = await fetchImpl(mcpProxyPath, {
			method: 'POST',
			headers: mcpHeaders(),
			body: JSON.stringify({ jsonrpc: '2.0', id: 'list-tools', method: 'tools/list', params: {} }),
		});
		captureSession(response);
		return parseResponse(await response.text()).result?.tools ?? [];
	}

	return {
		callTool,
		listTools,
		resetSession: () => {
			sessionId = null;
			initPromise = null;
		},
	};
}

const browserClient = createMCPClient((input, init) => fetch(input, init));
export const callTool = (...args) => browserClient.callTool(...args);
export const listTools = () => browserClient.listTools();
export const resetSession = () => browserClient.resetSession();

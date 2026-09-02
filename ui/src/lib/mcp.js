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

	return JSON.parse(text);
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

	async function callTool(name, args = {}) {
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

		const result = parseResponse(await response.text());
		if (result.error) throw new Error(result.error.message ?? JSON.stringify(result.error));

		const structured = result.result?.structuredContent;
		if (structured != null) {
			if (result.result?.isError) {
				const error =
					/** @type {Error & { code?: string, retryable?: boolean, retryAfterMs?: number|null, requestId?: string }} */ (
						new Error(structured.message ?? structured.code ?? 'Tool error')
					);
				error.code = structured.code;
				error.retryable = structured.retryable ?? false;
				error.retryAfterMs = structured.retry_after_ms ?? null;
				error.requestId = structured.request_id ?? null;
				throw error;
			}
			return structured;
		}

		if (result.result?.isError) throw new Error(result.result?.content?.[0]?.text ?? 'Tool error');
		const raw = result.result?.content?.[0]?.text ?? 'null';
		const jsonStart = raw.search(/\n[{[]/);
		let legacyData;
		if (jsonStart !== -1) {
			try {
				legacyData = JSON.parse(raw.slice(jsonStart + 1));
			} catch {}
		}
		if (legacyData === undefined) {
			try {
				legacyData = JSON.parse(raw);
			} catch {
				legacyData = raw;
			}
		}
		return { data: legacyData, meta: null, warnings: null };
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

import { env } from '$env/dynamic/private';
import { json } from '@sveltejs/kit';

const forwardedRequestHeaders = ['accept', 'content-type', 'last-event-id', 'mcp-session-id'];
const forwardedResponseHeaders = ['cache-control', 'content-type', 'mcp-session-id'];

function mcpEndpoint() {
	const value = env.OBA_MCP_URL;
	if (!value || !env.OBA_MCP_AUTH_TOKEN) return null;

	try {
		const url = new URL(value);
		if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
		url.pathname = `${url.pathname.replace(/\/$/, '')}/mcp`;
		url.search = '';
		url.hash = '';
		return url;
	} catch {
		return null;
	}
}

/** Proxy browser MCP requests with a server-held bearer token. */
export async function POST({ request }) {
	const endpoint = mcpEndpoint();
	if (!endpoint) {
		return json({ error: { message: 'The MCP proxy is not configured.' } }, { status: 503 });
	}

	// Accept X-Request-ID from the caller (browser MCP client); generate one if absent.
	// The ID is forwarded upstream so the MCP can attach it to structuredContent for
	// end-to-end correlation (phase-6 contract, see results.go AttachRequestID).
	const requestId = request.headers.get('x-request-id') ?? crypto.randomUUID();

	const headers = new Headers({
		Authorization: `Bearer ${env.OBA_MCP_AUTH_TOKEN}`,
		'X-Request-ID': requestId,
	});
	for (const name of forwardedRequestHeaders) {
		const value = request.headers.get(name);
		if (value) headers.set(name, value);
	}

	let upstream;
	try {
		upstream = await globalThis.fetch(endpoint, {
			method: 'POST',
			headers,
			body: await request.arrayBuffer(),
		});
	} catch {
		return json({ error: { message: 'The MCP service is unavailable.' } }, { status: 502 });
	}

	const responseHeaders = new Headers({ 'X-Request-ID': requestId });
	for (const name of forwardedResponseHeaders) {
		const value = upstream.headers.get(name);
		if (value) responseHeaders.set(name, value);
	}
	return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
}

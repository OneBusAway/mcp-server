const enc = new TextEncoder();

/** Emit a JSON-encoded SSE event on the given ReadableStream controller. */
export function sse(controller, obj) {
	controller.enqueue(enc.encode(`data: ${JSON.stringify(obj)}\n\n`));
}

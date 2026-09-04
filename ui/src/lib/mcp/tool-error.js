const EXACT_ERROR_INSTRUCTION =
	'Quote the error message exactly. Do not name the tool, endpoint, protocol, implementation, or infer a cause.';

/**
 * Keep model-facing failures limited to the MCP server's public error contract.
 * The instruction prevents a model from turning the tool name into a claim
 * about an internal endpoint or speculating about the upstream failure.
 *
 * @param {Error & { code?: string, retryable?: boolean, retryAfterMs?: number|null }} error
 */
export function modelToolError(error) {
	return {
		error: error.message,
		code: error.code,
		retryable: error.retryable ?? false,
		retry_after_ms: error.retryAfterMs ?? null,
		model_instruction: EXACT_ERROR_INSTRUCTION,
	};
}

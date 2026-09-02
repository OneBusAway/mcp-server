/**
 * Hand-rolled validators for the phase-6 MCP envelope contract.
 *
 * SuccessEnvelope: { data, meta: { generated_at_ms, truncated, cache?, request_id? }, warnings? }
 * ErrorEnvelope:   { code, message, retryable, retry_after_ms?, request_id? }
 *
 * Validators are forward-compatible: unknown extra fields are silently ignored.
 * Callers receive a normalized object with defaults for all optional fields.
 */

/**
 * @typedef {{
 *   data: unknown,
 *   meta: { generated_at_ms: number, truncated: boolean, cache: string, request_id: string },
 *   warnings: string[]
 * }} ValidatedSuccess
 */

/**
 * @typedef {{
 *   code: string,
 *   message: string,
 *   retryable: boolean,
 *   retry_after_ms: number | null,
 *   request_id: string
 * }} ValidatedError
 */

/**
 * Validate and normalize a SuccessEnvelope from structuredContent.
 * Throws TypeError when the envelope shape is fundamentally invalid.
 *
 * @param {unknown} raw
 * @returns {ValidatedSuccess}
 */
export function validateSuccess(raw) {
	if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) {
		throw new TypeError('MCP success envelope must be a non-null object');
	}
	const obj = /** @type {Record<string, unknown>} */ (raw);
	const meta = obj.meta;
	if (meta == null || typeof meta !== 'object' || Array.isArray(meta)) {
		throw new TypeError('MCP success envelope missing required meta block');
	}
	const m = /** @type {Record<string, unknown>} */ (meta);
	return {
		data: obj.data ?? null,
		meta: {
			generated_at_ms: typeof m.generated_at_ms === 'number' ? m.generated_at_ms : 0,
			truncated: m.truncated === true,
			cache: typeof m.cache === 'string' ? m.cache : '',
			request_id: typeof m.request_id === 'string' ? m.request_id : '',
		},
		warnings: Array.isArray(obj.warnings) ? /** @type {string[]} */ (obj.warnings) : [],
	};
}

/**
 * Validate and normalize an ErrorEnvelope from structuredContent.
 * Throws TypeError when the envelope shape is fundamentally invalid.
 *
 * @param {unknown} raw
 * @returns {ValidatedError}
 */
export function validateError(raw) {
	if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) {
		throw new TypeError('MCP error envelope must be a non-null object');
	}
	const obj = /** @type {Record<string, unknown>} */ (raw);
	return {
		code: typeof obj.code === 'string' ? obj.code : 'UNKNOWN',
		message: typeof obj.message === 'string' ? obj.message : 'Unknown error',
		retryable: obj.retryable === true,
		retry_after_ms: typeof obj.retry_after_ms === 'number' ? obj.retry_after_ms : null,
		request_id: typeof obj.request_id === 'string' ? obj.request_id : '',
	};
}

/**
 * Parse structuredContent from a tools/call result into a ValidatedSuccess,
 * or throw a typed Error when the tool reported an error envelope.
 *
 * The thrown Error carries camelCase properties matching the convention used
 * by dispatch.js: .code, .retryable, .retryAfterMs, .requestId.
 *
 * @param {unknown} structured - result.result.structuredContent
 * @param {boolean} isError    - result.result.isError
 * @returns {ValidatedSuccess}
 */
export function parseStructured(structured, isError) {
	if (isError) {
		const env = validateError(structured);
		const error =
			/** @type {Error & { code?: string, retryable?: boolean, retryAfterMs?: number|null, requestId?: string }} */ (
				new Error(env.message)
			);
		error.code = env.code;
		error.retryable = env.retryable;
		error.retryAfterMs = env.retry_after_ms;
		error.requestId = env.request_id;
		throw error;
	}
	return validateSuccess(structured);
}

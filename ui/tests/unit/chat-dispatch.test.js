import { describe, expect, it } from 'vitest';
import { modelToolError } from '../../src/lib/mcp/tool-error.js';

describe('chat tool error handling', () => {
	it('gives the model only the public error and forbids endpoint inference', async () => {
		const upstreamError = Object.assign(
			new Error('The transit service returned an unusable response.'),
			{ code: 'UPSTREAM_BAD_RESPONSE', retryable: false },
		);
		const result = modelToolError(upstreamError);

		expect(result).toEqual({
			error: 'The transit service returned an unusable response.',
			code: 'UPSTREAM_BAD_RESPONSE',
			retryable: false,
			retry_after_ms: null,
			model_instruction:
				'Quote the error message exactly. Do not name the tool, endpoint, protocol, implementation, or infer a cause.',
		});
	});
});

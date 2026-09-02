import { describe, it, expect } from 'vitest';
import { validateSuccess, validateError, parseStructured } from '../../src/lib/mcp/schemas.js';
import successFixture from '../fixtures/mcp/success.json';
import errorFixture from '../fixtures/mcp/error.json';
import futureFieldsFixture from '../fixtures/mcp/future-fields.json';

describe('validateSuccess', () => {
	it('parses a phase-6 success envelope', () => {
		const result = validateSuccess(successFixture);
		expect(result.meta.request_id).toBe('test-req-id-001');
		expect(result.meta.truncated).toBe(false);
		expect(result.meta.generated_at_ms).toBe(1700000000000);
		expect(/** @type {any} */ (result.data).items).toHaveLength(1);
		expect(result.warnings).toEqual([]);
	});

	it('silently ignores unknown future fields in meta and top-level', () => {
		expect(() => validateSuccess(futureFieldsFixture)).not.toThrow();
		const result = validateSuccess(futureFieldsFixture);
		expect(result.meta.request_id).toBe('test-req-id-003');
		expect(result.meta.cache).toBe('miss');
		expect(result.warnings).toHaveLength(1);
		expect(/** @type {any} */ (result.data).items).toEqual([]);
	});

	it('defaults optional meta fields when absent', () => {
		const result = validateSuccess({ data: null, meta: { generated_at_ms: 1 } });
		expect(result.meta.cache).toBe('');
		expect(result.meta.request_id).toBe('');
		expect(result.meta.truncated).toBe(false);
		expect(result.warnings).toEqual([]);
	});

	it('throws TypeError on non-object input', () => {
		expect(() => validateSuccess('not an object')).toThrow(TypeError);
		expect(() => validateSuccess(null)).toThrow(TypeError);
		expect(() => validateSuccess([1, 2])).toThrow(TypeError);
	});

	it('throws TypeError when meta is missing', () => {
		expect(() => validateSuccess({ data: {} })).toThrow(TypeError);
	});

	it('throws TypeError when meta is not an object', () => {
		expect(() => validateSuccess({ data: {}, meta: 'bad' })).toThrow(TypeError);
	});
});

describe('validateError', () => {
	it('parses a phase-6 error envelope', () => {
		const result = validateError(errorFixture);
		expect(result.code).toBe('UPSTREAM_TIMEOUT');
		expect(result.message).toBe('The transit service timed out.');
		expect(result.retryable).toBe(true);
		expect(result.retry_after_ms).toBe(5000);
		expect(result.request_id).toBe('test-req-id-002');
	});

	it('defaults missing optional fields', () => {
		const result = validateError({ code: 'INVALID_ARGUMENT', message: 'Bad arg', retryable: false });
		expect(result.retry_after_ms).toBeNull();
		expect(result.request_id).toBe('');
	});

	it('defaults code and message when absent', () => {
		const result = validateError({});
		expect(result.code).toBe('UNKNOWN');
		expect(result.message).toBe('Unknown error');
		expect(result.retryable).toBe(false);
	});

	it('throws TypeError on non-object input', () => {
		expect(() => validateError(null)).toThrow(TypeError);
		expect(() => validateError('err')).toThrow(TypeError);
	});
});

describe('parseStructured', () => {
	it('returns ValidatedSuccess for a success envelope', () => {
		const result = parseStructured(successFixture, false);
		expect(result.data).toBeDefined();
		expect(result.meta.request_id).toBe('test-req-id-001');
	});

	it('throws a typed Error for an error envelope', () => {
		expect(() => parseStructured(errorFixture, true)).toThrow('The transit service timed out.');
	});

	it('attaches camelCase error properties matching dispatch.js expectations', () => {
		try {
			parseStructured(errorFixture, true);
		} catch (e) {
			expect(e.code).toBe('UPSTREAM_TIMEOUT');
			expect(e.retryable).toBe(true);
			expect(e.retryAfterMs).toBe(5000);
			expect(e.requestId).toBe('test-req-id-002');
		}
	});

	it('does not throw on future-field success envelopes', () => {
		expect(() => parseStructured(futureFieldsFixture, false)).not.toThrow();
	});
});

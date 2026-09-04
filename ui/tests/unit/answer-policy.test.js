import { describe, expect, it } from 'vitest';
import {
	filterModelAnswer,
	PRIVATE_REASONING_FALLBACK,
	visibleModelAnswer,
} from '../../src/lib/mcp/answer-policy.js';

describe('filterModelAnswer', () => {
	it('passes through a direct rider answer', () => {
		expect(filterModelAnswer('Route 30 arrives at 4:18 PM (in 35 min).')).toEqual({
			text: 'Route 30 arrives at 4:18 PM (in 35 min).',
			rejected: false,
		});
	});

	it('rejects an explicit thinking-process response', () => {
		const result = filterModelAnswer(
			`Here's a thinking process:\n1. Analyze User Input:\n- User asks for airport arrivals.\n- I need to call a tool.`,
		);
		expect(result).toEqual({ text: '', rejected: true });
	});

	it('rejects untagged intermediate calculations', () => {
		const result = filterModelAnswer(
			'We have the arrivals data. We need to format the time. Let us compute predicted_ms and convert it to a date.',
		);
		expect(result).toEqual({ text: '', rejected: true });
	});

	it('removes tagged reasoning and keeps the final answer', () => {
		const result = filterModelAnswer(
			'<think>I need to inspect the result.</think>Route 30 arrives at 4:18 PM.',
		);
		expect(result).toEqual({ text: 'Route 30 arrives at 4:18 PM.', rejected: false });
	});

	it('extracts an explicit final answer after leaked reasoning', () => {
		const result = filterModelAnswer(
			'Analysis: I need to compute predicted_ms.\nFinal answer: Route 30 arrives at 4:18 PM.',
		);
		expect(result).toEqual({ text: 'Route 30 arrives at 4:18 PM.', rejected: false });
	});

	it('does not reject a direct quote of a tool error (rule 3)', () => {
		const quoted = 'The tool returned: The transit service returned an unusable response.';
		expect(filterModelAnswer(quoted)).toEqual({ text: quoted, rejected: false });
	});
});

describe('visibleModelAnswer', () => {
	it('suppresses a failure message when an arrivals card already answered the request', () => {
		expect(visibleModelAnswer({ text: '', rejected: true }, true)).toBe('');
	});

	it('uses the safe fallback when no structured answer is visible', () => {
		expect(visibleModelAnswer({ text: '', rejected: true }, false)).toBe(
			PRIVATE_REASONING_FALLBACK,
		);
	});
});

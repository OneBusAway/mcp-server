import { describe, expect, it } from 'vitest';
import { MULTIPLE_STOP_SEARCH_INSTRUCTION, RIDER_TOOLS } from '../../src/lib/mcp/tools.js';

describe('rider tool registry', () => {
	it('includes active trips near a location', () => {
		expect(RIDER_TOOLS.has('get_trips_for_location')).toBe(true);
	});

	it('allows landmark matches to anchor a vicinity search', () => {
		expect(MULTIPLE_STOP_SEARCH_INSTRUCTION).toContain('continue with a location-based tool');
		expect(MULTIPLE_STOP_SEARCH_INSTRUCTION).toContain('coordinate anchor');
	});
});

import { describe, expect, it } from 'vitest';
import {
	shouldPollArrivalsPanel,
	shouldPollMapVehicles,
	trackedArrivalRequest,
} from '../../src/lib/tracking-request.js';

describe('arrival tracking requests', () => {
	it('targets one precise arrival using every available identifier', () => {
		expect(
			trackedArrivalRequest({
				stop_id: 'test_stop',
				trip_id: 'trip_1',
				service_date: 1_770_000_000_000,
				vehicle_id: 'bus_1',
			}),
		).toEqual({
			name: 'get_arrival_and_departure_for_stop',
			input: {
				stop_id: 'test_stop',
				trip_id: 'trip_1',
				service_date: 1_770_000_000_000,
				vehicle_id: 'bus_1',
			},
		});
	});

	it('omits vehicle_id when the arrivals response did not provide one', () => {
		const request = trackedArrivalRequest({
			stop_id: 'test_stop',
			trip_id: 'trip_2',
			service_date: 1_770_000_000_000,
		});

		expect(request.name).toBe('get_arrival_and_departure_for_stop');
		expect(request.input).toEqual({
			stop_id: 'test_stop',
			trip_id: 'trip_2',
			service_date: 1_770_000_000_000,
		});
	});

	it('stops trip-detail polling when any tracker belongs to the map stop', () => {
		expect(
			shouldPollMapVehicles({
				agencyId: null,
				vehicleTripIds: ['different_trip'],
				hasStopTracker: true,
			}),
		).toBe(false);
		expect(
			shouldPollMapVehicles({
				agencyId: null,
				vehicleTripIds: ['different_trip'],
				hasStopTracker: false,
			}),
		).toBe(true);
	});

	it('never polls trip details for a stop-arrivals map', () => {
		expect(
			shouldPollMapVehicles({
				agencyId: null,
				vehicleTripIds: ['arrival-trip'],
				hasStopTracker: false,
				stopId: '1_1156',
			}),
		).toBe(false);
	});

	it('refreshes initialized arrival panels until precise tracking takes ownership', () => {
		expect(shouldPollArrivalsPanel({ stopId: 'test_stop', hasStopTracker: false })).toBe(true);
		expect(shouldPollArrivalsPanel({ stopId: 'test_stop', hasStopTracker: true })).toBe(false);
		expect(shouldPollArrivalsPanel({ stopId: null, hasStopTracker: false })).toBe(false);
	});
});

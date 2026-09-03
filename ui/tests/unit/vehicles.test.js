import { describe, expect, it } from 'vitest';
import {
	deduplicateVehicles,
	hasVehiclePosition,
	replaceRefreshedVehicles,
	vehicleFromArrival,
	vehicleFromTripDetails,
	vehicleKey,
	vehicleMatchesRoute,
	vehicleRequiresRouteGeometry,
} from '../../src/lib/vehicles.js';

describe('vehicle identity', () => {
	it('uses active trip ID before vehicle ID, like WayFinder', () => {
		expect(
			vehicleKey({
				active_trip_id: 'active-trip',
				vehicle_id: 'bus-7',
				trip_id: 'arrival-trip',
				lat: 1,
				lon: 2,
			}),
		).toBe('active-trip:active-trip');
	});

	it('keeps valid anonymous and zero-coordinate vehicles renderable', () => {
		expect(vehicleKey({ route_id: 'route-1', lat: 0, lon: 0 })).toBe(
			'anonymous:route-1:0.000000,0.000000',
		);
		expect(hasVehiclePosition({ lat: 0, lon: 0 })).toBe(true);
		expect(hasVehiclePosition({ lat: null, lon: null })).toBe(false);
	});

	it('merges aliases and retains metadata omitted by a position poll', () => {
		const vehicles = deduplicateVehicles([
			{ trip_id: 'trip-2', lat: 10, lon: 20, route_short_name: '44' },
			{ trip_id: 'trip-2', vehicle_id: 'bus-7', lat: 11, lon: 21 },
			{ vehicle_id: 'bus-7', trip_id: 'trip-2', lat: 12, lon: 22 },
		]);

		expect(vehicles).toHaveLength(1);
		expect(vehicles[0]).toMatchObject({
			vehicle_id: 'bus-7',
			trip_id: 'trip-2',
			lat: 12,
			lon: 22,
			route_short_name: '44',
		});
	});

	it('collapses an interlined bus while retaining its current active trip', () => {
		const vehicles = deduplicateVehicles([
			{
				active_trip_id: 'current-trip',
				trip_id: 'first-arrival',
				vehicle_id: 'bus-7',
				lat: 10,
				lon: 20,
			},
			{
				active_trip_id: 'current-trip',
				trip_id: 'next-arrival',
				vehicle_id: 'bus-7',
				lat: 10,
				lon: 20,
			},
		]);

		expect(vehicles).toHaveLength(1);
		expect(vehicleKey(vehicles[0])).toBe('active-trip:current-trip');
		expect(vehicles[0].trip_id).toBe('first-arrival');
	});

	it('maps an interlined arrival position onto its active route', () => {
		const vehicle = vehicleFromArrival({
			trip_id: 'future-arrival',
			route_id: 'agency_5',
			route_name: '5',
			active_trip_id: 'current-trip',
			active_route_id: 'agency_30',
			active_shape_id: 'shape-30',
			active_headsign: 'Current Route',
			vehicle_id: 'bus-7',
			vehicle_lat: 27.9,
			vehicle_lon: -82.4,
		});

		expect(vehicle).toMatchObject({
			trip_id: 'future-arrival',
			arrival_route_id: 'agency_5',
			active_trip_id: 'current-trip',
			route_id: 'agency_30',
			route_short_name: '30',
		});
		expect(vehicleKey(vehicle)).toBe('active-trip:current-trip');
	});

	it('does not attach active-trip GPS to an unresolved arrival route', () => {
		expect(
			vehicleFromArrival({
				trip_id: 'future-arrival',
				route_id: 'agency_5',
				active_trip_id: 'current-trip',
				vehicle_lat: 27.9,
				vehicle_lon: -82.4,
			}),
		).toBeNull();
	});

	it('uses active-trip route metadata from trip details', () => {
		expect(
			vehicleFromTripDetails(
				{
					trip_id: 'arrival-trip',
					route_id: 'agency_5',
					active_trip_id: 'current-trip',
					active_route_id: 'agency_30',
					lat: 27.9,
					lon: -82.4,
				},
				{ route_short_name: '5' },
			),
		).toMatchObject({
			arrival_route_id: 'agency_5',
			active_trip_id: 'current-trip',
			route_id: 'agency_30',
			route_short_name: '30',
		});
	});

	it('requires full active-route geometry instead of matching route numbers', () => {
		const vehicle = { active_route_id: 'agency-a_45', route_short_name: '45' };
		expect(vehicleRequiresRouteGeometry(vehicle)).toBe(true);
		expect(vehicleMatchesRoute(vehicle, 'agency-a_45')).toBe(true);
		expect(vehicleMatchesRoute(vehicle, 'agency-b_45')).toBe(false);
	});

	it('removes a refreshed trip when its latest response has no vehicle position', () => {
		const vehicles = replaceRefreshedVehicles(
			[
				{ trip_id: 'ended-trip', active_trip_id: 'ended-trip', lat: 10, lon: 20 },
				{ trip_id: 'untouched-trip', active_trip_id: 'untouched-trip', lat: 30, lon: 40 },
			],
			[],
			new Set(['ended-trip']),
		);

		expect(vehicles.map((vehicle) => vehicle.trip_id)).toEqual(['untouched-trip']);
	});
});

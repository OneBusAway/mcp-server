<script>
	// @ts-nocheck
	// MapCard.svelte carries pre-existing MapLibre + augmented-marker typing debt.
	// Phase U2.1 (see mcp-features/PRODUCTION_UI_TODO.md and PRODUCTION_UI_AUDIT.md)
	// splits this file into engine.js, geometry.js, markers.js, and vehicle-animation.js
	// with proper types; re-enable checkJs then.
	import { onMount, onDestroy, tick, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import { settings } from '$lib/settings.svelte.js';
	import { callTool } from '$lib/mcp.js';
	import { items, unwrap } from '$lib/result.js';
	import { tracking } from '$lib/tracking.svelte.js';
	import { shouldPollMapVehicles } from '$lib/tracking-request.js';
	import {
		deduplicateVehicles,
		hasVehiclePosition,
		mergeTrackedVehicleUpdates,
		replaceRefreshedVehicles,
		vehicleFromArrival,
		vehicleFromTripDetails,
		vehicleKey,
	} from '$lib/vehicles.js';
	import ArrivalsPanel from './ArrivalsPanel.svelte';

	/**
	 * @typedef {{ lat: number, lon: number, name?: string, id?: string, is_current?: boolean }} Marker
	 * @typedef {{ direction: string, route_id?: string, stops?: Marker[], coordinates?: [number, number][], encodedPolyline?: string }} RouteDir
	 * @typedef {{ lat: number, lon: number, vehicle_id?: string, trip_id?: string, route_id?: string, route_short_name?: string, headsign?: string, stops_away?: number, phase?: string, deviation_mins?: number }} Vehicle
	 */

	/** @type {{ markers?: Marker[], routes?: RouteDir[], zoom?: number, vehicles?: Vehicle[], agencyId?: string | null, vehicleTripIds?: string[], stopId?: string | null, tripInfo?: Record<string, { route_short_name: string, headsign: string }>, autoScroll?: boolean }} */
	let {
		markers = [],
		routes = [],
		zoom = 14,
		vehicles: vehiclesProp = [],
		agencyId = null,
		vehicleTripIds = [],
		stopId = null,
		tripInfo = {},
		autoScroll = false,
	} = $props();

	const THEMES = [
		{ id: 'https://tiles.openfreemap.org/styles/bright', label: 'Bright' },
		{ id: 'https://tiles.openfreemap.org/styles/liberty', label: 'Liberty' },
		{ id: 'https://demotiles.maplibre.org/style.json', label: 'Minimal' },
	];

	// Route colors stay distinct from the OneBusAway brand color. These are used
	// consistently by linework, legends, stop outlines, and vehicle badges.
	const LINE_COLORS = ['#e46f5b', '#9278d0', '#377fba', '#d18a2e', '#318a78', '#b5548d'];

	function routeColorForVehicle(vehicle) {
		const routeName = String(vehicle.route_short_name ?? vehicle.route_id ?? '');
		const matchIndex = routeName
			? routeLegend.findIndex(({ label }) =>
					String(label).toLowerCase().includes(routeName.toLowerCase()),
				)
			: -1;
		if (matchIndex >= 0) return routeLegend[matchIndex].color;
		let hash = 0;
		for (const char of routeName) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
		return LINE_COLORS[hash % LINE_COLORS.length];
	}

	function escapeHTML(value) {
		return String(value ?? '')
			.replaceAll('&', '&amp;')
			.replaceAll('<', '&lt;')
			.replaceAll('>', '&gt;')
			.replaceAll('"', '&quot;')
			.replaceAll("'", '&#039;');
	}

	function decodePolyline(encoded) {
		const coordinates = [];
		let index = 0,
			lat = 0,
			lon = 0;
		while (index < encoded.length) {
			let result = 0,
				shift = 0,
				byte;
			do {
				byte = encoded.charCodeAt(index++) - 63;
				result |= (byte & 0x1f) << shift;
				shift += 5;
			} while (byte >= 0x20 && index < encoded.length);
			lat += result & 1 ? ~(result >> 1) : result >> 1;
			result = 0;
			shift = 0;
			do {
				byte = encoded.charCodeAt(index++) - 63;
				result |= (byte & 0x1f) << shift;
				shift += 5;
			} while (byte >= 0x20 && index < encoded.length);
			lon += result & 1 ? ~(result >> 1) : result >> 1;
			coordinates.push([lon / 1e5, lat / 1e5]);
		}
		return coordinates;
	}

	function routeCoordinates(dir) {
		if (Array.isArray(dir.coordinates) && dir.coordinates.length >= 2) {
			return dir.coordinates.filter(([lon, lat]) => Number.isFinite(lon) && Number.isFinite(lat));
		}
		if (dir.encodedPolyline) return decodePolyline(dir.encodedPolyline);
		return (dir.stops ?? [])
			.filter((s) => s.lat && (s.lon || s.lng))
			.map((s) => [s.lon ?? s.lng, s.lat]);
	}

	let containerEl = $state();
	let mapEl = $state();
	let map;
	let maplibregl;
	let markerInstances = [];
	let popupInstances = [];
	let routeLayerIds = [];
	/** @type {Map<string, { marker: any, popup: any }>} */
	let vehicleMarkerMap = new Map();
	let localVehicles = $state([]);
	let isFullscreen = $state(false);
	let showThemes = $state(false);
	let activeTheme = $state(settings.mapStyle);
	let selectedStop = $state(null);
	let mapLoading = $state(true);
	let vehicleRefreshId = null; // plain var — not reactive
	let vehicleRefreshRequest = 0;
	let vehicleRebuildRaf = null;
	let lastVehiclesProp = null;

	// True when any of this map's tracked trips is being watched in the tracking store
	const isTracked = $derived(
		vehicleTripIds.length > 0 &&
			vehicleTripIds.some((id) => tracking.trackers.some((t) => t.trip_id === id)),
	);
	const hasStopTracker = $derived(
		!!stopId && tracking.trackers.some((tracker) => tracker.stop_id === stopId),
	);
	const routeLegend = $derived.by(() => {
		const uniqueRoutes = new Map();
		for (const [index, dir] of routes.entries()) {
			const key = dir.direction || `Route ${index + 1}`;
			if (!uniqueRoutes.has(key)) {
				uniqueRoutes.set(key, {
					label: key,
					color: LINE_COLORS[uniqueRoutes.size % LINE_COLORS.length],
				});
			}
		}
		return [...uniqueRoutes.values()];
	});
	const primaryStop = $derived(
		markers.find((marker) => stopId && marker.id === stopId) ??
			markers.find((marker) => marker.is_current) ??
			null,
	);

	function onFullscreenChange() {
		isFullscreen = !!document.fullscreenElement;
		tick().then(() => map?.resize());
	}

	async function toggleFullscreen() {
		if (!document.fullscreenElement) {
			await containerEl?.requestFullscreen?.();
		} else {
			await document.exitFullscreen?.();
		}
	}

	function applyTheme(url) {
		activeTheme = url;
		showThemes = false;
		mapLoading = true;
		map?.setStyle(url);
	}

	function zoomIn() {
		map?.zoomIn({ duration: 220 });
	}

	function zoomOut() {
		map?.zoomOut({ duration: 220 });
	}

	function clearRouteLayers() {
		for (const id of routeLayerIds) {
			if (map.getLayer(id)) map.removeLayer(id);
			if (map.getSource(id)) map.removeSource(id);
		}
		routeLayerIds = [];
	}

	function busInnerHTML(vehicle, color, isTracked) {
		const route = escapeHTML(vehicle.route_short_name ?? 'BUS');
		const bearing = Number.isFinite(vehicle.bearing) ? vehicle.bearing : 0;
		return `<span class="oba-vehicle-arrow" style="--vehicle-color:${color};transform:translateX(-50%) rotate(${bearing}deg)"></span>
			<span class="oba-vehicle-body" style="--vehicle-color:${color}">
				<span class="oba-vehicle-route">${route}</span>
			</span>
			${isTracked ? '<span class="oba-vehicle-tracking">Tracked</span>' : ''}`;
	}

	function makeBusElement(v, isTracked = false) {
		const color = routeColorForVehicle(v);
		const el = document.createElement('div');
		el.className = `oba-vehicle-marker${isTracked ? ' is-tracked' : ''}`;
		el.innerHTML = busInnerHTML(v, color, isTracked);
		return el;
	}

	function animateTo(entry, toLat, toLng, duration = 700) {
		if (entry._animRaf) cancelAnimationFrame(entry._animRaf);
		const from = entry.marker.getLngLat();
		const fromLng = from.lng,
			fromLat = from.lat;
		const start = performance.now();
		function step(now) {
			const t = Math.min((now - start) / duration, 1);
			const ease = 1 - Math.pow(1 - t, 3);
			entry.marker.setLngLat([
				fromLng + (toLng - fromLng) * ease,
				fromLat + (toLat - fromLat) * ease,
			]);
			if (t < 1) {
				entry._animRaf = requestAnimationFrame(step);
			} else {
				entry._animRaf = null;
			}
		}
		entry._animRaf = requestAnimationFrame(step);
	}

	function pointDistance(a, b) {
		return Math.hypot(a[0] - b[0], a[1] - b[1]);
	}

	// Metres per degree of longitude at a given latitude
	const METRES_PER_DEG = 111320;
	function mPerDeg(lat) {
		return METRES_PER_DEG * Math.cos((lat * Math.PI) / 180);
	}

	function sameStopLocation(a, b, thresholdM = 6) {
		if (!a || !b) return false;
		const aLon = a.lon ?? a.lng;
		const bLon = b.lon ?? b.lng;
		if (![a.lat, aLon, b.lat, bLon].every(Number.isFinite)) return false;
		const refLat = (a.lat + b.lat) / 2;
		return (
			Math.hypot((aLon - bLon) * mPerDeg(refLat), (a.lat - b.lat) * METRES_PER_DEG) <= thresholdM
		);
	}

	// Returns nearest point on polyline with distM in metres (latitude-corrected)
	function nearestPointOnRoute(point, coordinates) {
		const lonScale = mPerDeg(point[1]);
		let nearest = null;
		for (let index = 0; index < coordinates.length - 1; index++) {
			const start = coordinates[index];
			const end = coordinates[index + 1];
			const dx = (end[0] - start[0]) * lonScale;
			const dy = (end[1] - start[1]) * METRES_PER_DEG;
			const px = (point[0] - start[0]) * lonScale;
			const py = (point[1] - start[1]) * METRES_PER_DEG;
			const lengthSq = dx * dx + dy * dy;
			const fraction = lengthSq ? Math.max(0, Math.min(1, (px * dx + py * dy) / lengthSq)) : 0;
			const snapped = [
				start[0] + (end[0] - start[0]) * fraction,
				start[1] + (end[1] - start[1]) * fraction,
			];
			const ex = (point[0] - snapped[0]) * lonScale;
			const ey = (point[1] - snapped[1]) * METRES_PER_DEG;
			const distM = Math.hypot(ex, ey);
			if (!nearest || distM < nearest.distM) nearest = { point: snapped, index, fraction, distM };
		}
		return nearest;
	}

	const SNAP_DISTANCE_M = 400;

	function routeAliases(value) {
		if (value == null) return [];
		const id = String(value).trim().toLowerCase();
		if (!id) return [];
		return [...new Set([id, id.split('_').at(-1)])];
	}

	function directionMatchesVehicle(direction, vehicleIds) {
		return [direction.route_id, direction.direction]
			.flatMap(routeAliases)
			.some((id) => vehicleIds.has(id));
	}

	// Snap only to the vehicle's route. Once matching geometry exists, keep the
	// marker on it even when an upstream GPS update is unusually far away.
	function snapVehicleToRoute(vehicle, lngLat) {
		const coords = routeCoordinatesForVehicle(vehicle, lngLat);
		if (coords) {
			const nearest = nearestPointOnRoute(lngLat, coords);
			if (nearest) return nearest.point;
		}
		return null;
	}

	function routeCoordinatesForVehicle(vehicle, target, from = null) {
		const ids = new Set(
			[vehicle.active_route_id, vehicle.route_id, vehicle.route_short_name].flatMap(routeAliases),
		);
		const candidates = routes
			.map((direction) => ({ direction, coordinates: routeCoordinates(direction) }))
			.filter(({ coordinates }) => coordinates.length >= 2);
		const matching = candidates.filter(({ direction }) => directionMatchesVehicle(direction, ids));
		// Old saved single-route cards have no route_id metadata. With only one
		// route group there is no unrelated line to choose, so they remain usable.
		const routeGroups = new Set(
			candidates.map(({ direction }) => direction.route_id ?? direction.direction ?? ''),
		);
		const eligible = matching.length ? matching : routeGroups.size <= 1 ? candidates : [];
		let best = null;
		for (const { coordinates } of eligible) {
			const targetProjection = nearestPointOnRoute(target, coordinates);
			const fromProjection = from ? nearestPointOnRoute(from, coordinates) : null;
			// WayFinder chooses the shape minimizing the combined distance from both
			// endpoints. Using only the new GPS point can jump to a parallel/opposite
			// shape and becomes especially obvious after zooming in.
			const score = targetProjection.distM + (fromProjection?.distM ?? 0);
			if (!best || score < best.score) best = { coordinates, score };
		}
		return best?.coordinates ?? null;
	}

	function routePathForVehicle(vehicle, from, target) {
		const coordinates = routeCoordinatesForVehicle(vehicle, target, from);
		if (!coordinates) return null;
		const start = nearestPointOnRoute(from, coordinates);
		const end = nearestPointOnRoute(target, coordinates);
		// 400 m threshold — generous enough for OBA GPS noise, still rejects vehicles
		// that are genuinely on a different road
		if (!start || !end || start.distM > SNAP_DISTANCE_M || end.distM > SNAP_DISTANCE_M) return null;

		const startProgress = start.index + start.fraction;
		const endProgress = end.index + end.fraction;
		const middle =
			startProgress <= endProgress
				? coordinates.slice(start.index + 1, end.index + 1)
				: coordinates.slice(end.index + 1, start.index + 1).reverse();
		// Path ends at the on-route projection, NOT at raw GPS. Ending at `target`
		// (raw GPS) makes the marker rest on a nearby house/parking lot ~10-30 m off
		// the road — visible immediately when the user zooms in.
		const path = [start.point, ...middle, end.point];
		return path.filter(
			(point, index) => index === 0 || pointDistance(point, path[index - 1]) > 0.000001,
		);
	}

	function animateAlongRoute(entry, path, duration = 2800) {
		if (entry._animRaf) cancelAnimationFrame(entry._animRaf);
		// Use metre-based segment lengths for proportional speed across lat/lon
		const segmentLengths = path.slice(1).map((point, index) => {
			const from = path[index];
			const refLat = (from[1] + point[1]) / 2;
			return Math.hypot(
				(point[0] - from[0]) * mPerDeg(refLat),
				(point[1] - from[1]) * METRES_PER_DEG,
			);
		});
		const totalLength = segmentLengths.reduce((total, length) => total + length, 0);
		if (!totalLength) return;
		const start = performance.now();
		function step(now) {
			const elapsed = Math.min((now - start) / duration, 1);
			const targetDistance = (1 - Math.pow(1 - elapsed, 3)) * totalLength;
			let traversed = 0;
			for (let index = 0; index < segmentLengths.length; index++) {
				const length = segmentLengths[index];
				if (traversed + length >= targetDistance || index === segmentLengths.length - 1) {
					const fraction = length ? (targetDistance - traversed) / length : 1;
					const from = path[index],
						to = path[index + 1];
					entry.marker.setLngLat([
						from[0] + (to[0] - from[0]) * fraction,
						from[1] + (to[1] - from[1]) * fraction,
					]);
					break;
				}
				traversed += length;
			}
			if (elapsed < 1) entry._animRaf = requestAnimationFrame(step);
			else entry._animRaf = null;
		}
		entry._animRaf = requestAnimationFrame(step);
	}

	function vehiclePopupText(v) {
		const parts = [];
		if (v.route_short_name) parts.push(v.route_short_name);
		if (v.headsign) parts.push(v.headsign);
		if (v.stops_away > 0) parts.push(`${v.stops_away} stop${v.stops_away === 1 ? '' : 's'} away`);
		if (v.phase && v.phase !== 'in_progress') parts.push(v.phase.replace(/_/g, ' '));
		return parts.join(' · ') || v.vehicle_id || 'Vehicle';
	}

	// Public entry: coalesces multiple invocations that hit within the same frame.
	// When Track fires, `tracking.trackers`, `stopArrivals[stopId]`, and `localVehicles`
	// can all reassign in the same microtask batch — without coalescing this ran the
	// expensive DOM/snap loop N times back-to-back and blocked the main thread long
	// enough that clicks stopped registering.
	function buildVehicleMarkers() {
		if (!map || !maplibregl) return;
		if (vehicleRebuildRaf != null) return;
		vehicleRebuildRaf = requestAnimationFrame(() => {
			vehicleRebuildRaf = null;
			// Re-read tracked IDs at fire time so we reflect the latest tracker state,
			// not the state captured when the first cascading effect scheduled us.
			_buildVehicleMarkersNow([...tracking.trackers]);
		});
	}

	function _buildVehicleMarkersNow(activeTrackers) {
		if (!map || !maplibregl) return;
		const seen = new Set();

		for (const v of deduplicateVehicles(localVehicles)) {
			const key = vehicleKey(v);
			if (!key || !hasVehiclePosition(v)) continue;
			seen.add(key);
			const lngLat = [v.lon, v.lat];
			const popupText = vehiclePopupText(v);

			const isTracked = activeTrackers.some(
				(tracker) =>
					(v.active_trip_id && tracker.active_trip_id === v.active_trip_id) ||
					(v.vehicle_id && tracker.vehicle_id === v.vehicle_id) ||
					tracker.trip_id === v.trip_id,
			);
			const color = routeColorForVehicle(v);
			const visualKey = `${v.route_short_name ?? ''}|${v.bearing ?? ''}|${color}|${isTracked}`;
			const targetSnapped = snapVehicleToRoute(v, lngLat) ?? lngLat;
			if (vehicleMarkerMap.has(key)) {
				const entry = vehicleMarkerMap.get(key);
				const { marker, popup } = entry;
				const trackingChanged = entry.isTracked !== isTracked;
				// Update visual in-place if tracked state changed
				if (entry.visualKey !== visualKey) {
					entry.el.className = `oba-vehicle-marker${isTracked ? ' is-tracked' : ''}`;
					entry.el.innerHTML = busInnerHTML(v, color, isTracked);
					entry.isTracked = isTracked;
					entry.visualKey = visualKey;
				}
				// Compare the incoming GPS position, not its snapped projection. Several GPS
				// reports can project to the same point on a coarse route shape; comparing
				// projections made those vehicles look frozen. Keeping the raw source also
				// prevents unrelated tracker renders from restarting an animation.
				const prev = entry.sourceLngLat;
				const destChanged =
					!prev || Math.abs(prev[0] - lngLat[0]) > 1e-7 || Math.abs(prev[1] - lngLat[1]) > 1e-7;
				if (destChanged || trackingChanged) {
					entry.sourceLngLat = lngLat;
					entry.targetLngLat = targetSnapped;
					const curr = marker.getLngLat();
					const path = routePathForVehicle(v, [curr.lng, curr.lat], lngLat);
					if (path?.length > 1) animateAlongRoute(entry, path);
					// Never draw a straight diagonal when a valid route projection exists.
					else if (targetSnapped !== lngLat) marker.setLngLat(targetSnapped);
					else animateTo(entry, targetSnapped[1], targetSnapped[0], 1800);
				}
				if (popup) popup.setLngLat(targetSnapped).setText(popupText);
			} else {
				const el = makeBusElement(v, isTracked);
				const popup = new maplibregl.Popup({
					offset: 20,
					closeButton: false,
					className: 'oba-hover-popup',
				})
					.setLngLat(targetSnapped)
					.setText(popupText);
				const marker = new maplibregl.Marker({
					element: el,
					anchor: 'center',
					subpixelPositioning: true,
				})
					.setLngLat(targetSnapped)
					.addTo(map);
				el.addEventListener('mouseenter', () => popup.addTo(map));
				el.addEventListener('mouseleave', () => popup.remove());
				vehicleMarkerMap.set(key, {
					marker,
					popup,
					el,
					isTracked,
					visualKey,
					sourceLngLat: lngLat,
					targetLngLat: targetSnapped,
					_animRaf: null,
				});
			}
		}

		// Remove markers for vehicles no longer in the list
		for (const [key, entry] of vehicleMarkerMap) {
			if (!seen.has(key)) {
				if (entry._animRaf) cancelAnimationFrame(entry._animRaf);
				entry.popup?.remove();
				entry.marker.remove();
				vehicleMarkerMap.delete(key);
			}
		}
	}

	async function refreshVehiclePositions(tripIds = vehicleTripIds) {
		const request = ++vehicleRefreshRequest;
		if (agencyId) {
			try {
				const result = await callTool('get_vehicles_for_agency', { agency_id: agencyId });
				const fleet = items(result);
				if (request === vehicleRefreshRequest) {
					localVehicles = deduplicateVehicles(fleet);
				}
			} catch {}
		} else if (tripIds?.length) {
			const ids = tripIds.slice(0, 6);
			const settled = await Promise.allSettled(
				ids.map((id) => callTool('get_trip_details', { trip_id: id, include_schedule: false })),
			);
			const refreshedTripIds = new Set();
			const updates = settled.flatMap((r, index) => {
				if (r.status !== 'fulfilled') return [];
				refreshedTripIds.add(ids[index]);
				const d = unwrap(r.value);
				const vehicle = vehicleFromTripDetails(
					{ ...d, trip_id: d?.trip_id ?? ids[index] },
					tripInfo[d?.trip_id ?? ids[index]],
				);
				return vehicle ? [vehicle] : [];
			});
			if (request === vehicleRefreshRequest) {
				const projectedUpdates = updates.filter((vehicle) =>
					routeCoordinatesForVehicle(vehicle, [vehicle.lon, vehicle.lat]),
				);
				localVehicles = replaceRefreshedVehicles(localVehicles, projectedUpdates, refreshedTripIds);
			}
		}
	}

	function buildLayers() {
		markerInstances.forEach((m) => m.remove());
		markerInstances = [];
		popupInstances.forEach((p) => p.remove());
		popupInstances = [];
		clearRouteLayers();

		const allRouteStops = [];
		const currentStopMarker =
			markers.find((marker) => stopId && marker.id === stopId) ??
			markers.find((marker) => marker.is_current);
		const colorByRoute = new Map();
		let colorIndex = 0;
		routes.forEach((dir, i) => {
			// Skip the current stop here — it's drawn as the highlighted blue marker
			// below from `markers`, so drawing the route dot too would stack them.
			const valid = (dir.stops ?? []).filter(
				(s) =>
					s.lat &&
					(s.lon || s.lng) &&
					(!stopId || s.id !== stopId) &&
					!sameStopLocation(s, currentStopMarker),
			);
			const coordinates = routeCoordinates(dir);
			if (coordinates.length < 2) return;

			allRouteStops.push(...coordinates.map(([lon, lat]) => ({ lon, lat })));
			// OBA returns one route as several shape segments. Keep every segment
			// of that route the same color instead of cycling per segment.
			const colorKey = dir.direction || `route-${i}`;
			if (!colorByRoute.has(colorKey)) {
				colorByRoute.set(colorKey, LINE_COLORS[colorIndex % LINE_COLORS.length]);
				colorIndex += 1;
			}
			const color = colorByRoute.get(colorKey);
			const srcId = `route-${i}`;
			const casingId = `route-casing-${i}`;
			const layerId = `route-line-${i}`;
			const routeCasing = document.documentElement.classList.contains('dark')
				? '#11130f'
				: '#ffffff';

			map.addSource(srcId, {
				type: 'geojson',
				data: {
					type: 'Feature',
					geometry: {
						type: 'LineString',
						coordinates,
					},
				},
			});
			map.addLayer({
				id: casingId,
				type: 'line',
				source: srcId,
				layout: { 'line-join': 'round', 'line-cap': 'round' },
				paint: { 'line-color': routeCasing, 'line-width': 9, 'line-opacity': 0.92 },
			});
			map.addLayer({
				id: layerId,
				type: 'line',
				source: srcId,
				layout: { 'line-join': 'round', 'line-cap': 'round' },
				paint: { 'line-color': color, 'line-width': 5, 'line-opacity': 1 },
			});
			routeLayerIds.push(casingId, layerId, srcId);

			for (const s of valid) {
				const el = document.createElement('div');
				Object.assign(el.style, {
					width: '11px',
					height: '11px',
					borderRadius: '50%',
					background: '#26302a',
					border: `2px solid ${color}`,
					boxShadow: '0 0 0 1px rgba(255,255,255,.75), 0 1px 3px rgba(0,0,0,.35)',
					cursor: s.id ? 'pointer' : 'default',
				});
				const popup = s.name
					? new maplibregl.Popup({ offset: 10, closeButton: false, className: 'oba-hover-popup' })
							.setLngLat([s.lon ?? s.lng, s.lat])
							.setText(s.name)
					: null;
				const marker = new maplibregl.Marker({ element: el, anchor: 'center' }).setLngLat([
					s.lon ?? s.lng,
					s.lat,
				]);
				el.addEventListener('mouseenter', () => {
					el.style.boxShadow = `0 0 0 4px rgba(255,255,255,.75), 0 2px 6px rgba(0,0,0,.35)`;
					popup?.addTo(map);
				});
				el.addEventListener('mouseleave', () => {
					el.style.boxShadow = '0 0 0 1px rgba(255,255,255,.75), 0 1px 3px rgba(0,0,0,.35)';
					popup?.remove();
				});
				if (s.id) {
					el.addEventListener('click', () => {
						if (s.id === stopId) return;
						selectedStop = selectedStop?.id === s.id ? null : { id: s.id, name: s.name ?? s.id };
					});
				}
				marker.addTo(map);
				markerInstances.push(marker);
				if (popup) popupInstances.push(popup);
			}
		});

		// Prefer the blue/current marker, then discard any green search marker at
		// the same physical location even if the upstream IDs differ.
		const markerCandidates = markers
			.filter((m) => m.lat && (m.lon || m.lng))
			.sort(
				(a, b) =>
					Number(Boolean(b.is_current || (stopId && b.id === stopId))) -
					Number(Boolean(a.is_current || (stopId && a.id === stopId))),
			);
		const validMarkers = [];
		for (const marker of markerCandidates) {
			if (validMarkers.some((existing) => sameStopLocation(existing, marker))) continue;
			validMarkers.push(marker);
		}
		for (const m of validMarkers) {
			const el = document.createElement('div');
			const hasId = !!m.id;
			const isCurrent = !!m.is_current || (!!stopId && m.id === stopId);
			if (isCurrent) el.className = 'oba-current-stop-marker';
			Object.assign(el.style, {
				width: isCurrent ? '28px' : '16px',
				height: isCurrent ? '28px' : '16px',
				borderRadius: isCurrent ? '8px' : '50%',
				background: isCurrent ? '#f9faf7' : '#78aa37',
				border: isCurrent ? '3px solid #26302a' : '3px solid white',
				boxShadow: isCurrent
					? '0 0 0 2px rgba(255,255,255,.85), 0 2px 8px rgba(0,0,0,.45)'
					: '0 1px 5px rgba(0,0,0,0.4)',
				cursor: hasId ? 'pointer' : 'default',
			});
			if (isCurrent) {
				el.innerHTML =
					'<span style="display:block;width:12px;height:12px;margin:5px;border-radius:3px;background:#377fba"></span>';
			}
			const popup = m.name
				? new maplibregl.Popup({ offset: 14, closeButton: false, className: 'oba-hover-popup' })
						.setLngLat([m.lon ?? m.lng, m.lat])
						.setText(isCurrent ? `${m.name} · Your stop` : m.name)
				: null;
			const marker = new maplibregl.Marker({ element: el, anchor: 'center' }).setLngLat([
				m.lon ?? m.lng,
				m.lat,
			]);
			el.addEventListener('mouseenter', () => {
				el.style.boxShadow = isCurrent
					? '0 0 0 4px rgba(55,127,186,.35), 0 2px 8px rgba(0,0,0,.45)'
					: '0 0 0 4px rgba(120,170,55,.3), 0 2px 8px rgba(0,0,0,.4)';
				popup?.addTo(map);
			});
			el.addEventListener('mouseleave', () => {
				el.style.boxShadow = isCurrent
					? '0 0 0 2px rgba(255,255,255,.85), 0 2px 8px rgba(0,0,0,.45)'
					: '0 1px 5px rgba(0,0,0,0.4)';
				popup?.remove();
			});
			if (hasId) {
				el.addEventListener('click', () => {
					if (m.id === stopId) return;
					selectedStop = selectedStop?.id === m.id ? null : { id: m.id, name: m.name ?? m.id };
				});
			}
			marker.addTo(map);
			markerInstances.push(marker);
			if (popup) popupInstances.push(popup);
		}

		const allPoints = [
			...allRouteStops,
			...validMarkers,
			...localVehicles.filter((v) => v.lat && v.lon),
		].filter((p) => p.lat && (p.lon || p.lng));

		if (allPoints.length > 1) {
			const bounds = new maplibregl.LngLatBounds();
			for (const p of allPoints) bounds.extend([p.lon ?? p.lng, p.lat]);
			map.fitBounds(bounds, { padding: 40, maxZoom: 16 });
		} else if (allPoints.length === 1) {
			map.setCenter([allPoints[0].lon ?? allPoints[0].lng, allPoints[0].lat]);
		}
	}

	onMount(async () => {
		if (!browser) return;
		if (autoScroll) {
			await tick();
			requestAnimationFrame(() =>
				containerEl?.scrollIntoView({ behavior: 'smooth', block: 'center' }),
			);
		}

		document.addEventListener('fullscreenchange', onFullscreenChange);

		const mod = await import('maplibre-gl');
		maplibregl = mod.default ?? mod;
		await new Promise((r) => requestAnimationFrame(r));

		const allPoints = [
			...routes.flatMap((d) => [
				...(d.stops ?? []),
				...routeCoordinates(d).map(([lon, lat]) => ({ lon, lat })),
			]),
			...markers,
			...vehiclesProp,
		].filter((p) => p.lat && (p.lon || p.lng));
		const first = allPoints[0];
		const center = first ? [first.lon ?? first.lng, first.lat] : [-121.74, 38.54];

		map = new maplibregl.Map({
			container: mapEl,
			style: activeTheme,
			center,
			zoom,
			attributionControl: false,
			pitchWithRotate: false,
			dragRotate: false,
		});

		map.addControl(new maplibregl.AttributionControl({ compact: true }), 'bottom-right');
		map.on('style.load', () => {
			buildLayers();
			buildVehicleMarkers();
			mapLoading = false;
		});

		const ro = new ResizeObserver(() => requestAnimationFrame(() => map?.resize()));
		ro.observe(containerEl);

		return () => {
			ro.disconnect();
			document.removeEventListener('fullscreenchange', onFullscreenChange);
		};
	});

	// Rebuild route lines + stop markers when those props change
	$effect(() => {
		const _r = routes;
		const _m = markers;
		if (map && map.isStyleLoaded()) {
			buildLayers();
			// A vehicle can arrive before its route geometry during streaming.
			// Re-project existing markers as soon as the shapes become available.
			buildVehicleMarkers();
		}
	});

	// Tool cards may receive their vehicle list after this component has mounted.
	// Keep the local, animated list in sync with that prop without treating every
	// tracker update as a new GPS report.
	$effect(() => {
		const incoming = vehiclesProp;
		if (incoming === lastVehiclesProp) return;
		lastVehiclesProp = incoming;
		localVehicles = deduplicateVehicles(
			incoming.filter((vehicle) => !vehicle.trip_id || vehicle.active_trip_id),
		);
	});

	// Update vehicle markers whenever localVehicles or tracking state changes.
	// The Set is (re)computed inside buildVehicleMarkers at rAF fire time,
	// so cascading tracker/stopArrivals/localVehicles updates within one frame
	// only trigger one DOM rebuild.
	$effect(() => {
		localVehicles;
		tracking.trackers;
		if (map && map.isStyleLoaded()) buildVehicleMarkers();
	});

	// Non-tracked/restored maps use trip details. Once a stop arrival is tracked,
	// its precise arrival-for-stop poll becomes the only live-position source.
	$effect(() => {
		const canPoll = shouldPollMapVehicles({ agencyId, vehicleTripIds, hasStopTracker });
		if (!canPoll) return;
		const refresh = () => refreshVehiclePositions(vehicleTripIds);
		const ms = isTracked ? 30_000 : 60_000;
		refresh();
		vehicleRefreshId = setInterval(refresh, ms);
		return () => {
			clearInterval(vehicleRefreshId);
			vehicleRefreshId = null;
			vehicleRefreshRequest++;
		};
	});

	// Update vehicle positions from tracking store's live stop arrivals
	$effect(() => {
		if (!stopId) return;
		const liveArrivals = tracking.stopArrivals[stopId];
		if (!Array.isArray(liveArrivals) || !liveArrivals.length) return;
		const trackedTripIds = new Set(
			tracking.trackers.filter((t) => t.stop_id === stopId).map((t) => t.trip_id),
		);
		const vehicled = liveArrivals.filter(
			(a) =>
				a.trip_id &&
				trackedTripIds.has(a.trip_id) &&
				a.vehicle_lat != null &&
				a.vehicle_lon != null,
		);
		if (!vehicled.length) return;

		// This effect writes localVehicles, so reading it reactively here would make
		// the effect trigger itself forever and freeze all page interactions.
		const currentVehicles = untrack(() => localVehicles);
		const projectedUpdates = vehicled
			.map(vehicleFromArrival)
			.filter(Boolean)
			.filter((vehicle) => routeCoordinatesForVehicle(vehicle, [vehicle.lon, vehicle.lat]));
		if (!projectedUpdates.length) return;
		localVehicles = mergeTrackedVehicleUpdates(currentVehicles, projectedUpdates, trackedTripIds);
	});

	onDestroy(() => {
		if (vehicleRefreshId) clearInterval(vehicleRefreshId);
		if (vehicleRebuildRaf != null) {
			cancelAnimationFrame(vehicleRebuildRaf);
			vehicleRebuildRaf = null;
		}
		for (const entry of vehicleMarkerMap.values()) {
			if (entry._animRaf) cancelAnimationFrame(entry._animRaf);
			entry.popup?.remove();
			entry.marker.remove();
		}
		vehicleMarkerMap.clear();
		popupInstances.forEach((p) => p.remove());
		map?.remove();
	});

	const hasContent = $derived(markers.length > 0 || routes.length > 0 || vehiclesProp.length > 0);
</script>

{#if browser && hasContent}
	<div
		bind:this={containerEl}
		class="overflow-hidden rounded-[10px] border border-[#cedadf] bg-white shadow-[0_1px_2px_rgba(23,33,38,.06),0_6px_18px_rgba(23,33,38,.06)] dark:border-[#34382f] dark:bg-[#1a1d17] {isFullscreen
			? 'flex flex-col'
			: 'rounded-[10px]'}"
	>
		<!-- Map viewport -->
		<div
			class="relative"
			style={isFullscreen ? 'flex: 1 1 0; min-height: 0;' : 'height: clamp(300px, 46vw, 420px)'}
		>
			<div bind:this={mapEl} style="position: absolute; inset: 0;"></div>
			{#if mapLoading}
				<div
					class="absolute inset-0 z-20 flex items-center justify-center bg-white/80 text-sm text-[#5f6659] backdrop-blur-sm dark:bg-[#11130f]/80 dark:text-[#a5ab9f]"
					aria-live="polite"
				>
					<svg
						class="mr-2 h-4 w-4 animate-spin text-oba-600"
						viewBox="0 0 24 24"
						fill="none"
						aria-hidden="true"
					>
						<circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="3"
						></circle>
						<path class="opacity-75" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-3a6 6 0 0 0-6-6V3z"
						></path>
					</svg>
					Loading map…
				</div>
			{/if}

			<!-- Controls -->
			<div
				class="absolute right-3 top-3 z-10 flex flex-col overflow-visible rounded-[7px] border border-[#cedadf] bg-white/95 shadow-sm backdrop-blur-sm dark:border-[#34382f] dark:bg-[#1a1d17]/95"
			>
				<button
					type="button"
					onclick={zoomIn}
					aria-label="Zoom in"
					class="flex h-9 w-9 items-center justify-center border-b border-[#cedadf] text-lg text-[#64737a] transition hover:bg-[#eaf0f2] hover:text-[#172126] dark:border-[#34382f] dark:text-[#a5ab9f] dark:hover:bg-[#22261e] dark:hover:text-[#f0f2ed]"
				>
					+
				</button>
				<button
					type="button"
					onclick={zoomOut}
					aria-label="Zoom out"
					class="flex h-9 w-9 items-center justify-center border-b border-[#cedadf] text-lg text-[#64737a] transition hover:bg-[#eaf0f2] hover:text-[#172126] dark:border-[#34382f] dark:text-[#a5ab9f] dark:hover:bg-[#22261e] dark:hover:text-[#f0f2ed]"
				>
					−
				</button>
				<!-- Theme picker -->
				<div class="relative">
					<button
						type="button"
						onclick={() => (showThemes = !showThemes)}
						title="Change map theme"
						class="flex h-9 w-9 items-center justify-center border-b border-[#cedadf] text-[#64737a] transition hover:bg-[#eaf0f2] hover:text-[#172126] dark:border-[#34382f] dark:text-[#a5ab9f] dark:hover:bg-[#22261e] dark:hover:text-[#f0f2ed]"
					>
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<polygon points="12 2 2 7 12 12 22 7 12 2" />
							<polyline points="2 17 12 22 22 17" />
							<polyline points="2 12 12 17 22 12" />
						</svg>
					</button>

					{#if showThemes}
						<div
							class="absolute right-11 top-0 z-20 min-w-[130px] overflow-hidden rounded-[7px] border border-[#cedadf] bg-white shadow-lg dark:border-[#34382f] dark:bg-[#1a1d17]"
						>
							{#each THEMES as t}
								<button
									type="button"
									onclick={() => applyTheme(t.id)}
									class="flex min-h-9 w-full items-center gap-2 px-3 py-2 text-left text-xs transition hover:bg-[#f7fafb] dark:hover:bg-[#22261e]
										{activeTheme === t.id
										? 'font-semibold text-oba-600 dark:text-oba-400'
										: 'text-[#5f6659] dark:text-[#a5ab9f]'}"
								>
									{#if activeTheme === t.id}
										<svg
											xmlns="http://www.w3.org/2000/svg"
											width="10"
											height="10"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="3"
											stroke-linecap="round"
											stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg
										>
									{:else}
										<span class="w-[10px]"></span>
									{/if}
									{t.label}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Fullscreen -->
				<button
					type="button"
					onclick={toggleFullscreen}
					title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
					class="flex h-9 w-9 items-center justify-center text-[#64737a] transition hover:bg-[#eaf0f2] hover:text-[#172126] dark:text-[#a5ab9f] dark:hover:bg-[#22261e] dark:hover:text-[#f0f2ed]"
				>
					{#if isFullscreen}
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path d="M8 3v3a2 2 0 0 1-2 2H3" /><path d="M21 8h-3a2 2 0 0 1-2-2V3" />
							<path d="M3 16h3a2 2 0 0 1 2 2v3" /><path d="M16 21v-3a2 2 0 0 1 2-2h3" />
						</svg>
					{:else}
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path d="M3 7V3h4" /><path d="M17 3h4v4" />
							<path d="M21 17v4h-4" /><path d="M7 21H3v-4" />
						</svg>
					{/if}
				</button>
			</div>

			<!-- Route direction legend -->
			{#if routeLegend.length > 0}
				<div class="absolute left-3 top-3 z-10 flex max-w-[calc(100%-92px)] flex-col gap-1.5">
					{#each routeLegend as route}
						<div
							class="flex min-h-9 items-center gap-2 rounded-full border border-[#cedadf] bg-white/95 px-3.5 text-xs shadow-sm backdrop-blur-sm dark:border-[#34382f] dark:bg-[#1a1d17]/95"
						>
							<span class="h-3 w-3 rounded-[4px]" style="background: {route.color};"></span>
							<span class="max-w-[160px] truncate font-medium text-[#1d211a] dark:text-[#f0f2ed]"
								>{route.label}</span
							>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Vehicle count badge (shown on vehicle_map cards with no route legend) -->
			{#if localVehicles.length > 0 && routes.length === 0}
				<div class="absolute bottom-6 left-2 z-10">
					<div
						class="flex items-center gap-1.5 rounded-[5px] border border-[#cedadf] bg-white/95 px-2.5 py-1.5 text-xs shadow-sm backdrop-blur-sm dark:border-[#34382f] dark:bg-[#1a1d17]/95"
					>
						<svg
							width="12"
							height="10"
							viewBox="0 0 13 10"
							fill="none"
							xmlns="http://www.w3.org/2000/svg"
						>
							<rect x="0.5" y="0.5" width="12" height="7" rx="1.5" fill="#4caf50" />
							<rect x="1" y="1" width="5" height="3.5" rx="0.5" fill="white" fill-opacity="0.5" />
							<rect x="7" y="1" width="4.5" height="3.5" rx="0.5" fill="white" fill-opacity="0.5" />
							<circle cx="2.5" cy="9.2" r="1" fill="#4caf50" />
							<circle cx="10.5" cy="9.2" r="1" fill="#4caf50" />
						</svg>
						<span class="font-medium text-[#1d211a] dark:text-[#f0f2ed]"
							>{localVehicles.length} active vehicle{localVehicles.length === 1 ? '' : 's'}</span
						>
					</div>
				</div>
			{/if}
		</div>

		{#if primaryStop && !selectedStop}
			<div
				class="flex items-center gap-3 border-t border-[#cedadf] bg-white px-4 py-3 dark:border-[#34382f] dark:bg-[#1a1d17]"
			>
				<span
					class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 border-[#377fba]"
				>
					<span class="h-3 w-3 rounded-full bg-[#377fba]"></span>
				</span>
				<div class="min-w-0 flex-1">
					<p
						class="oba-heading truncate text-[22px] leading-tight text-[#1d211a] dark:text-[#f0f2ed]"
					>
						{primaryStop.name ?? 'Selected stop'}
					</p>
					<p class="mt-0.5 truncate text-xs text-[#7d8377] dark:text-[#858b80]">
						{primaryStop.id ? `Stop #${primaryStop.id}` : 'Selected stop'}
					</p>
				</div>
				<span
					class="hidden text-[10.5px] font-bold uppercase tracking-[0.1em] text-[#7d8377] dark:text-[#858b80] sm:inline"
				>
					Live map
				</span>
			</div>
		{/if}

		<!-- Arrivals pane: shown when a clickable stop is selected -->
		{#if selectedStop}
			<div
				class="border-t border-[#cedadf] dark:border-[#34382f] {isFullscreen
					? 'flex-shrink-0'
					: ''}"
				style={isFullscreen
					? 'height: 320px; overflow-y: auto;'
					: 'max-height: 220px; overflow-y: auto;'}
			>
				<ArrivalsPanel
					stopId={selectedStop.id}
					stopName={selectedStop.name}
					onClose={() => (selectedStop = null)}
				/>
			</div>
		{/if}
	</div>
{/if}

<style>
	:global(.oba-vehicle-marker) {
		position: relative;
		display: flex;
		width: 44px;
		height: 52px;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		user-select: none;
		filter: drop-shadow(0 2px 3px rgb(0 0 0 / 0.35));
	}
	:global(.oba-vehicle-body) {
		position: relative;
		z-index: 2;
		display: flex;
		width: 32px;
		height: 32px;
		align-items: center;
		justify-content: center;
		overflow: hidden;
		border: 2px solid white;
		border-radius: 50%;
		background: var(--vehicle-color);
		box-shadow: 0 0 0 1px rgb(23 33 38 / 0.55);
	}
	:global(.oba-vehicle-route) {
		display: grid;
		width: 100%;
		height: 100%;
		place-items: center;
		background: transparent;
		color: white;
		font-family: 'Public Sans', sans-serif;
		font-size: 11.5px;
		font-weight: 700;
		line-height: 1;
	}
	:global(.oba-vehicle-arrow) {
		position: absolute;
		z-index: 1;
		left: 50%;
		top: 0;
		width: 0;
		height: 0;
		transform-origin: 50% 26px;
		border-right: 5px solid transparent;
		border-bottom: 10px solid var(--vehicle-color);
		border-left: 5px solid transparent;
	}
	:global(.oba-vehicle-marker.is-tracked) {
		filter: drop-shadow(0 2px 4px rgb(98 142 41 / 0.35));
	}
	:global(.oba-vehicle-marker.is-tracked .oba-vehicle-body) {
		box-shadow:
			0 0 0 2px white,
			0 0 0 5px #78aa37;
	}
	:global(.oba-vehicle-tracking) {
		position: absolute;
		z-index: 3;
		left: 50%;
		bottom: 0;
		transform: translateX(-50%);
		padding: 2px 5px;
		border-radius: 999px;
		background: #405c1e;
		color: white;
		font-family: 'Public Sans', sans-serif;
		font-size: 7px;
		font-weight: 800;
		letter-spacing: 0.08em;
		line-height: 1;
		text-transform: uppercase;
		white-space: nowrap;
	}
	:global(.maplibregl-ctrl-top-right) {
		top: 8px;
		right: 8px;
	}
	:global(.maplibregl-ctrl-group) {
		overflow: hidden;
		border: 1px solid #cedadf;
		border-radius: 7px;
		box-shadow: 0 1px 3px rgb(29 33 26 / 0.16);
	}
	:global(.maplibregl-ctrl-group button) {
		width: 36px;
		height: 36px;
	}
	:global(.dark .maplibregl-ctrl-group) {
		border-color: #34382f;
		background: #1a1d17;
	}
	:global(.dark .maplibregl-ctrl-group button + button) {
		border-color: #34382f;
	}
	:global(.dark .maplibregl-ctrl-icon) {
		filter: invert(1);
	}
	:global(:fullscreen .maplibregl-map),
	:global(:-webkit-full-screen .maplibregl-map) {
		width: 100% !important;
		height: 100% !important;
	}
	/* Hover-only popups: no pointer events (prevents flicker loop), no animation (prevents flying). */
	:global(.oba-hover-popup),
	:global(.oba-hover-popup .maplibregl-popup-content),
	:global(.oba-hover-popup .maplibregl-popup-tip) {
		pointer-events: none;
		animation: none !important;
		transition: none !important;
	}
	:global(.oba-hover-popup .maplibregl-popup-content) {
		border: 1px solid #cedadf;
		border-radius: 7px;
		color: #1d211a;
		font-family: 'Public Sans', sans-serif;
		font-size: 12px;
		font-weight: 600;
		box-shadow: 0 4px 14px rgb(29 33 26 / 0.15);
	}
	:global(.dark .oba-hover-popup .maplibregl-popup-content) {
		border-color: #34382f;
		background: #1a1d17;
		color: #f0f2ed;
	}
	:global(.dark .oba-hover-popup .maplibregl-popup-tip) {
		border-color: #1a1d17;
	}
</style>

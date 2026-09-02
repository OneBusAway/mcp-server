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
	import ArrivalsPanel from './ArrivalsPanel.svelte';

	/**
	 * @typedef {{ lat: number, lon: number, name?: string, id?: string, is_current?: boolean }} Marker
	 * @typedef {{ direction: string, stops?: Marker[], coordinates?: [number, number][], encodedPolyline?: string }} RouteDir
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

	// Route line colors — cycles through these for multi-direction routes
	const LINE_COLORS = ['#4caf50', '#2196f3', '#ff9800', '#e91e63', '#9c27b0'];

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
	let vehicleRebuildRaf = null;
	let lastVehiclesProp = null;

	// True when any of this map's tracked trips is being watched in the tracking store
	const isTracked = $derived(
		vehicleTripIds.length > 0 &&
			vehicleTripIds.some((id) => tracking.trackers.some((t) => t.trip_id === id)),
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

	function clearRouteLayers() {
		for (const id of routeLayerIds) {
			if (map.getLayer(id)) map.removeLayer(id);
			if (map.getSource(id)) map.removeSource(id);
		}
		routeLayerIds = [];
	}

	function busInnerHTML(color, isTracked) {
		let html = '';
		if (isTracked) {
			html += `<svg style="position:absolute;inset:-6px;width:46px;height:46px;pointer-events:none;" viewBox="0 0 46 46" fill="none"><circle cx="23" cy="23" r="18" stroke="${color}" stroke-width="2.5" opacity="0.7"><animate attributeName="r" values="15;19;15" dur="1.5s" repeatCount="indefinite"/><animate attributeName="opacity" values="0.7;0.15;0.7" dur="1.5s" repeatCount="indefinite"/></circle></svg>`;
		}
		html += `<svg width="34" height="34" viewBox="0 0 34 34" fill="none" xmlns="http://www.w3.org/2000/svg" aria-label="Vehicle">
			<circle cx="17" cy="17" r="11.5" fill="${color}" stroke="white" stroke-width="1.5"/>
			<path d="M12.25 23.5v-8.1c0-2.55 1.65-4.15 4.75-4.15s4.75 1.6 4.75 4.15v8.1h-1v1.15c0 .4-.32.7-.7.7h-.6a.7.7 0 0 1-.7-.7V23.5h-3.5v1.15c0 .4-.32.7-.7.7h-.6a.7.7 0 0 1-.7-.7V23.5h-1Z" fill="white"/>
			<rect x="15.05" y="12.55" width="3.9" height="1.15" rx=".5" fill="${color}"/>
			<path d="M14.1 15h5.8c.4 0 .7.3.75.7l.35 3.15H13l.35-3.15c.05-.4.35-.7.75-.7Z" fill="${color}"/>
			<circle cx="14.35" cy="21.25" r=".85" fill="${color}"/><circle cx="19.65" cy="21.25" r=".85" fill="${color}"/>
		</svg>`;
		return html;
	}

	function makeBusElement(v, isTracked = false) {
		const color = isTracked ? '#f59e0b' : '#16a34a';
		const el = document.createElement('div');
		el.style.cssText = `cursor:pointer;user-select:none;line-height:0;position:relative;width:34px;height:34px;filter:drop-shadow(0 ${isTracked ? '2px 5px rgba(245,158,11,0.55)' : '1px 3px rgba(0,0,0,0.32)'});`;
		el.innerHTML = busInnerHTML(color, isTracked);
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

	// Snap a [lon, lat] position to the nearest route line within 400 m.
	// OBA GPS reports can drift 150-300 m from the shape; 400 m catches real
	// noise while still rejecting vehicles that are genuinely on a different road.
	function snapToRoute(lngLat) {
		let best = null;
		for (const dir of routes) {
			const coords = routeCoordinates(dir);
			if (coords.length < 2) continue;
			const nearest = nearestPointOnRoute(lngLat, coords);
			if (!nearest) continue;
			if (!best || nearest.distM < best.distM) best = nearest;
		}
		return best && best.distM <= 400 ? best.point : null;
	}

	// Snap using the vehicle's own route when we can identify it; falls back
	// to nearest-of-any-route. Prevents a bus getting glued to a parallel line
	// (e.g. two routes on the same street) it doesn't actually serve.
	function snapVehicleToRoute(vehicle, lngLat) {
		const coords = routeCoordinatesForVehicle(vehicle, lngLat);
		if (coords) {
			const nearest = nearestPointOnRoute(lngLat, coords);
			if (nearest && nearest.distM <= 400) return nearest.point;
		}
		return snapToRoute(lngLat);
	}

	function routeCoordinatesForVehicle(vehicle, target, from = null) {
		const shortId = String(vehicle.route_id ?? '')
			.split('_')
			.at(-1);
		const ids = new Set(
			[vehicle.route_id, vehicle.route_short_name, shortId].filter(Boolean).map(String),
		);
		let best = null;
		for (const direction of routes) {
			const coordinates = routeCoordinates(direction);
			if (coordinates.length < 2) continue;
			const match = ids.has(String(direction.direction));
			const targetProjection = nearestPointOnRoute(target, coordinates);
			const fromProjection = from ? nearestPointOnRoute(from, coordinates) : null;
			// WayFinder chooses the shape minimizing the combined distance from both
			// endpoints. Using only the new GPS point can jump to a parallel/opposite
			// shape and becomes especially obvious after zooming in.
			const score = targetProjection.distM + (fromProjection?.distM ?? 0) + (match ? 0 : 500);
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
		if (!start || !end || start.distM > 400 || end.distM > 400) return null;

		const startProgress = start.index + start.fraction;
		const endProgress = end.index + end.fraction;
		const middle =
			startProgress <= endProgress
				? coordinates.slice(start.index + 1, end.index + 1)
				: coordinates.slice(end.index + 1, start.index + 1).reverse();
		// Path ends at the on-route projection, NOT at raw GPS. Ending at `target`
		// (raw GPS) makes the marker rest on a nearby house/parking lot ~10-30 m off
		// the road — visible immediately when the user zooms in.
		const path = [from, start.point, ...middle, end.point];
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

	// A physical bus can appear on multiple trips (interlined service, OBA quirks).
	// Deduplicate by vehicle_id: prefer the entry that has GPS coordinates.
	function deduplicateVehicles(vehicles) {
		const byVehicleId = new Map();
		const noId = [];
		for (const v of vehicles) {
			if (!v.vehicle_id) {
				noId.push(v);
				continue;
			}
			const existing = byVehicleId.get(v.vehicle_id);
			if (!existing || (!existing.lat && v.lat)) byVehicleId.set(v.vehicle_id, v);
		}
		return [...byVehicleId.values(), ...noId];
	}

	// Public entry: coalesces multiple invocations that hit within the same frame.
	// When Track fires, `tracking.trackers`, `stopArrivals[stopId]`, and `localVehicles`
	// can all reassign in the same microtask batch — without coalescing this ran the
	// expensive DOM/snap loop N times back-to-back and blocked the main thread long
	// enough that clicks stopped registering.
	function buildVehicleMarkers(_trackedIds = new Set()) {
		if (!map || !maplibregl) return;
		if (vehicleRebuildRaf != null) return;
		vehicleRebuildRaf = requestAnimationFrame(() => {
			vehicleRebuildRaf = null;
			// Re-read tracked IDs at fire time so we reflect the latest tracker state,
			// not the state captured when the first cascading effect scheduled us.
			const latest = new Set(tracking.trackers.map((t) => t.trip_id));
			_buildVehicleMarkersNow(latest);
		});
	}

	function _buildVehicleMarkersNow(trackedIds) {
		if (!map || !maplibregl) return;
		const seen = new Set();

		for (const v of deduplicateVehicles(localVehicles)) {
			const key = v.trip_id || v.vehicle_id;
			if (!key || !v.lat || !v.lon) continue;
			seen.add(key);
			const lngLat = [v.lon, v.lat];
			const popupText = vehiclePopupText(v);

			const isTracked = trackedIds.has(key);
			const targetSnapped = snapVehicleToRoute(v, lngLat) ?? lngLat;
			if (vehicleMarkerMap.has(key)) {
				const entry = vehicleMarkerMap.get(key);
				const { marker, popup } = entry;
				// Update visual in-place if tracked state changed
				if (entry.isTracked !== isTracked) {
					const color = isTracked ? '#f59e0b' : '#16a34a';
					entry.el.innerHTML = busInnerHTML(color, isTracked);
					entry.el.style.filter = `drop-shadow(0 ${isTracked ? '2px 5px rgba(245,158,11,0.55)' : '1px 3px rgba(0,0,0,0.32)'})`;
					entry.isTracked = isTracked;
				}
				// Compare the incoming GPS position, not its snapped projection. Several GPS
				// reports can project to the same point on a coarse route shape; comparing
				// projections made those vehicles look frozen. Keeping the raw source also
				// prevents unrelated tracker renders from restarting an animation.
				const prev = entry.sourceLngLat;
				const destChanged =
					!prev || Math.abs(prev[0] - lngLat[0]) > 1e-7 || Math.abs(prev[1] - lngLat[1]) > 1e-7;
				if (destChanged) {
					entry.sourceLngLat = lngLat;
					entry.targetLngLat = targetSnapped;
					const curr = marker.getLngLat();
					const path = routePathForVehicle(v, [curr.lng, curr.lat], lngLat);
					if (path?.length > 1) animateAlongRoute(entry, path);
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
					sourceLngLat: lngLat,
					targetLngLat: targetSnapped,
					_animRaf: null,
				});
			}
		}

		// Remove markers for vehicles no longer in the list
		for (const [key, { marker, popup }] of vehicleMarkerMap) {
			if (!seen.has(key)) {
				popup?.remove();
				marker.remove();
				vehicleMarkerMap.delete(key);
			}
		}
	}

	async function refreshVehiclePositions(tripIds = vehicleTripIds) {
		if (agencyId) {
			try {
				const result = await callTool('get_vehicles_for_agency', { agency_id: agencyId });
				const fleet = items(result);
				if (fleet.length) {
					localVehicles = fleet.filter((v) => v.lat && v.lon);
				}
			} catch {}
		} else if (tripIds?.length) {
			const ids = tripIds.slice(0, 5);
			const settled = await Promise.allSettled(
				ids.map((id) => callTool('get_trip_details', { trip_id: id, include_schedule: false })),
			);
			const updates = settled.flatMap((r) => {
				if (r.status !== 'fulfilled') return [];
				const d = unwrap(r.value);
				return d?.lat != null && d?.lon != null ? [d] : [];
			});
			if (updates.length) {
				// Merge: update existing vehicles' positions OR add vehicles that weren't in arrivals GPS
				const merged = [...localVehicles];
				for (const upd of updates) {
					const idx = merged.findIndex(
						(v) => v.trip_id === upd.trip_id || v.vehicle_id === upd.vehicle_id,
					);
					if (idx >= 0) {
						merged[idx] = {
							...merged[idx],
							lat: upd.lat,
							lon: upd.lon,
							bearing: upd.bearing ?? merged[idx].bearing,
						};
					} else {
						// Use trip_info for route badge label when GPS wasn't in arrivals data
						const info = tripInfo[upd.trip_id] ?? {};
						merged.push({
							lat: upd.lat,
							lon: upd.lon,
							trip_id: upd.trip_id,
							vehicle_id: upd.vehicle_id,
							route_id: upd.route_id,
							bearing: upd.bearing ?? null,
							route_short_name: info.route_short_name ?? null,
							headsign: info.headsign ?? null,
							stops_away: null,
						});
					}
				}
				localVehicles = merged;
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
			const layerId = `route-line-${i}`;

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
				id: layerId,
				type: 'line',
				source: srcId,
				layout: { 'line-join': 'round', 'line-cap': 'round' },
				paint: { 'line-color': color, 'line-width': 3, 'line-opacity': 0.9 },
			});
			routeLayerIds.push(srcId, layerId);

			for (const s of valid) {
				const el = document.createElement('div');
				Object.assign(el.style, {
					width: '10px',
					height: '10px',
					borderRadius: '50%',
					background: color,
					border: '2px solid white',
					boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
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
					el.style.boxShadow = `0 0 0 3px rgba(255,255,255,0.7), 0 2px 6px rgba(0,0,0,0.35)`;
					popup?.addTo(map);
				});
				el.addEventListener('mouseleave', () => {
					el.style.boxShadow = '0 1px 3px rgba(0,0,0,0.3)';
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
				width: isCurrent ? '20px' : '14px',
				height: isCurrent ? '20px' : '14px',
				borderRadius: '50%',
				background: isCurrent ? '#2563eb' : '#4caf50',
				border: isCurrent ? '3px solid white' : '2.5px solid white',
				boxShadow: isCurrent
					? '0 0 0 4px rgba(37,99,235,0.28), 0 2px 8px rgba(0,0,0,0.45)'
					: '0 1px 5px rgba(0,0,0,0.4)',
				cursor: hasId ? 'pointer' : 'default',
			});
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
					? '0 0 0 6px rgba(37,99,235,0.24), 0 2px 8px rgba(0,0,0,0.45)'
					: '0 0 0 4px rgba(76,175,80,0.3), 0 2px 8px rgba(0,0,0,0.4)';
				popup?.addTo(map);
			});
			el.addEventListener('mouseleave', () => {
				el.style.boxShadow = isCurrent
					? '0 0 0 4px rgba(37,99,235,0.28), 0 2px 8px rgba(0,0,0,0.45)'
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
		map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-left');

		map.on('style.load', () => {
			buildLayers();
			buildVehicleMarkers();
			mapLoading = false;
			// Stop-focused maps are refreshed by tracking.stopArrivals. Only generic
			// trip/agency maps need the separate vehicle-position lookup.
			if (agencyId || (!stopId && vehicleTripIds?.length > 0)) {
				refreshVehiclePositions();
			}
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
		}
	});

	// Tool cards may receive their vehicle list after this component has mounted.
	// Keep the local, animated list in sync with that prop without treating every
	// tracker update as a new GPS report.
	$effect(() => {
		const incoming = vehiclesProp;
		if (incoming === lastVehiclesProp) return;
		lastVehiclesProp = incoming;
		localVehicles = [...incoming];
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

	// Poll generic trip/agency maps directly. Stop-focused maps get their live
	// positions from tracking.stopArrivals, including recently departed trips
	// retained by the arrivals endpoint's minutes_before window.
	$effect(() => {
		const canPoll = !!agencyId || (!stopId && (vehicleTripIds?.length ?? 0) > 0);
		if (!canPoll) return;
		const refresh = () => refreshVehiclePositions(vehicleTripIds);
		const ms = isTracked ? 30_000 : 60_000;
		refresh();
		vehicleRefreshId = setInterval(refresh, ms);
		return () => {
			clearInterval(vehicleRefreshId);
			vehicleRefreshId = null;
		};
	});

	// Update vehicle positions from tracking store's live stop arrivals
	$effect(() => {
		if (!stopId) return;
		const liveArrivals = tracking.stopArrivals[stopId];
		if (!Array.isArray(liveArrivals) || !liveArrivals.length) return;
		// minutes_before keeps a just-departed trip in this response, and OBA's
		// embedded tripStatus continues carrying its current vehicle position.
		const vehicled = liveArrivals.filter((a) => a.vehicle_lat && a.vehicle_lon);
		if (!vehicled.length) return;

		// OBA typically stops reporting GPS the moment a bus reaches/passes the stop.
		// Keep the last-known position visible for trips the user is still tracking
		// so the bus doesn't vanish right as it arrives.
		const trackedTripIds = new Set(
			tracking.trackers.filter((t) => t.stop_id === stopId).map((t) => t.trip_id),
		);
		const freshTripIds = new Set(vehicled.map((a) => a.trip_id));
		// This effect writes localVehicles, so reading it reactively here would make
		// the effect trigger itself forever and freeze all page interactions.
		const currentVehicles = untrack(() => localVehicles);
		const preserved = currentVehicles.filter(
			(v) => v.trip_id && trackedTripIds.has(v.trip_id) && !freshTripIds.has(v.trip_id),
		);
		localVehicles = [
			...vehicled.map((a) => ({
				lat: a.vehicle_lat,
				lon: a.vehicle_lon,
				vehicle_id: a.vehicle_id,
				trip_id: a.trip_id,
				route_id: a.route_id,
				route_short_name: a.route_name,
				headsign: a.headsign,
				stops_away: a.number_of_stops_away,
				bearing: a.vehicle_bearing ?? null,
			})),
			...preserved,
		];
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
		class="overflow-hidden border border-zinc-200 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-950 {isFullscreen
			? 'flex flex-col'
			: 'rounded-xl'}"
	>
		<!-- Map viewport -->
		<div class="relative" style={isFullscreen ? 'flex: 1 1 0; min-height: 0;' : 'height: 220px'}>
			<div bind:this={mapEl} style="position: absolute; inset: 0;"></div>
			{#if mapLoading}
				<div
					class="absolute inset-0 z-20 flex items-center justify-center bg-white/80 text-sm text-zinc-600 backdrop-blur-sm dark:bg-zinc-950/80 dark:text-zinc-300"
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
			<div class="absolute right-2 top-2 z-10 flex items-center gap-1">
				<!-- Theme picker -->
				<div class="relative">
					<button
						type="button"
						onclick={() => (showThemes = !showThemes)}
						title="Change map theme"
						class="flex h-7 w-7 items-center justify-center rounded-md bg-white/90 text-zinc-600 shadow-sm backdrop-blur-sm transition hover:bg-white hover:text-zinc-900 dark:bg-zinc-900/90 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
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
							class="absolute right-0 top-8 z-20 min-w-[110px] overflow-hidden rounded-lg border border-zinc-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900"
						>
							{#each THEMES as t}
								<button
									type="button"
									onclick={() => applyTheme(t.id)}
									class="flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition hover:bg-zinc-50 dark:hover:bg-zinc-800
										{activeTheme === t.id
										? 'font-semibold text-oba-600 dark:text-oba-400'
										: 'text-zinc-700 dark:text-zinc-300'}"
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
					class="flex h-7 w-7 items-center justify-center rounded-md bg-white/90 text-zinc-600 shadow-sm backdrop-blur-sm transition hover:bg-white hover:text-zinc-900 dark:bg-zinc-900/90 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
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
				<div class="absolute bottom-6 left-2 z-10 flex flex-col gap-1">
					{#each routeLegend as route}
						<div
							class="flex items-center gap-1.5 rounded-md bg-white/90 px-2 py-1 text-xs shadow-sm backdrop-blur-sm dark:bg-zinc-900/90"
						>
							<span class="h-2 w-4 rounded-full" style="background: {route.color};"></span>
							<span class="max-w-[160px] truncate font-medium text-zinc-700 dark:text-zinc-300"
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
						class="flex items-center gap-1.5 rounded-md bg-white/90 px-2 py-1 text-xs shadow-sm backdrop-blur-sm dark:bg-zinc-900/90"
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
						<span class="font-medium text-zinc-700 dark:text-zinc-300"
							>{localVehicles.length} active vehicle{localVehicles.length === 1 ? '' : 's'}</span
						>
					</div>
				</div>
			{/if}
		</div>

		<!-- Arrivals pane: shown when a clickable stop is selected -->
		{#if selectedStop}
			<div
				class="border-t border-zinc-200 dark:border-zinc-700 {isFullscreen ? 'flex-shrink-0' : ''}"
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
		color: #18181b;
	}
	:global(.dark .oba-hover-popup .maplibregl-popup-content) {
		background: #18181b;
		color: #f4f4f5;
	}
	:global(.dark .oba-hover-popup .maplibregl-popup-tip) {
		border-color: #18181b;
	}
</style>

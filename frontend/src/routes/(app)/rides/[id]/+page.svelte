<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { Map, LngLatBounds, setWorkerUrl } from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import maplibreWorkerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
	import {
		getActivitySamples,
		getActivityWeather,
		getActivityStory,
		getActivityLaps,
		deleteActivity,
		getActivityBike,
		updateActivityBike,
		formatDistance,
		formatDuration,
		formatDecimal,
		type Sample,
		type WeatherSummary,
		type RideStory,
		type RideStat,
		type Lap
	} from '$lib/rides';
	import { getShareStatus, createShare, revokeShare, type ShareStatus } from '$lib/share';
	import { listBikes, type Bike } from '$lib/bikes';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';
	import StoryCards from '$lib/components/StoryCards.svelte';
	import ZoneBars from '$lib/components/ZoneBars.svelte';
	import { confirm, pivot } from '$lib/rekta/rekta.js';
	import { toast } from '$lib/toast';

	// maplibre-gl's own worker-URL construction is a runtime string template
	// (`new URL('./maplibre-gl-worker.mjs', import.meta.url)`), which Vite's
	// static import analysis can't follow — the worker chunk never gets
	// emitted and the browser requests a URL that doesn't exist. So point
	// MapLibre at a URL Vite does emit.
	//
	// It has to be `?worker&url`, not plain `?url`: the dist worker is not
	// self-contained, it imports `./maplibre-gl-shared.mjs`. Plain `?url`
	// copies only the one file, so that sibling import 404s at runtime and
	// the worker dies silently — no map error event, no tile requests, just
	// an empty canvas. `?worker` bundles the worker with its dependencies.
	setWorkerUrl(maplibreWorkerUrl);

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let samples: Sample[] = $state([]);
	let weather: WeatherSummary | null = $state(null);
	let story: RideStory | null = $state(null);
	// Splits (#589): only some devices/rides have laps (manual button press or
	// auto-lap) — an empty list means "no splits", not an error, so the panel
	// just doesn't render rather than showing an empty table.
	let laps: Lap[] = $state([]);
	let mapContainer: HTMLDivElement = $state()!;
	let map: Map | null = null;

	// Sections instead of one long scroll (#650, refinement of #646) — Karte
	// and Analyse stay mounted (hidden, not destroyed) rather than behind an
	// {#if}: the map is expensive to tear down and recreate, and the charts
	// have nothing to lose by staying alive off-screen either.
	let activeTab: 'uebersicht' | 'karte' | 'analyse' = $state('uebersicht');
	const tabs = [
		{ key: 'uebersicht', label: 'Übersicht' },
		{ key: 'karte', label: 'Karte' },
		{ key: 'analyse', label: 'Analyse' }
	] as const;
	function facetPivot(el: HTMLElement) {
		const strip = pivot(el, { onSelect: (i) => (activeTab = tabs[i].key) });
		return { destroy: strip.destroy };
	}

	// Share status (#641): loaded best-effort alongside the story, since a
	// ride still renders fine without knowing whether it's shared yet.
	let shareStatus: ShareStatus | null = $state(null);
	let shareBusy = $state(false);
	let shareUrl = $derived.by(() => {
		const token = shareStatus?.token;
		return token ? `${page.url.origin}/share/${token}` : '';
	});

	async function share() {
		shareBusy = true;
		try {
			shareStatus = await createShare(Number(page.params.id));
		} finally {
			shareBusy = false;
		}
	}

	async function unshare() {
		shareBusy = true;
		try {
			shareStatus = await revokeShare(Number(page.params.id));
		} finally {
			shareBusy = false;
		}
	}

	let deleteBusy = $state(false);
	let deleteError = $state('');

	// Which bike this ride is credited to, correctable after the fact (#730)
	// — upload time only ever sets it automatically from the active bike
	// (#637). All bikes are offered, retired ones included: fixing a mistake
	// might mean attributing the ride to a bike the rider no longer owns.
	let bikeList: Bike[] = $state([]);
	let currentBikeId: number | null = $state(null);
	let bikeBusy = $state(false);
	let bikeError = $state('');

	async function changeBike(e: Event) {
		const value = (e.currentTarget as HTMLSelectElement).value;
		const bikeId = value === '' ? null : Number(value);
		bikeBusy = true;
		bikeError = '';
		try {
			await updateActivityBike(Number(page.params.id), bikeId);
			currentBikeId = bikeId;
		} catch (err) {
			bikeError = err instanceof ApiError ? err.message : 'Rad konnte nicht geändert werden.';
		} finally {
			bikeBusy = false;
		}
	}

	// Irreversible (#701) — everything derived from the ride goes with it,
	// same as the confirm-then-cascade pattern the CLI's user-delete already
	// uses. Native confirm() rather than a custom sheet: this is a one-shot
	// yes/no gate, not a view worth its own component.
	async function deleteRide() {
		const label = story?.subtitle ? `Fahrt vom ${story.subtitle}` : 'diese Fahrt';
		// Rekta's question sheet; keeping the ride is the safe, focused answer.
		const ok = await confirm({
			kicker: 'löschen',
			title: `${label} löschen?`,
			text: 'Das kann nicht rückgängig gemacht werden.',
			ok: 'Fahrt löschen',
			cancel: 'Behalten',
			safe: 'cancel'
		});
		if (!ok) return;
		deleteBusy = true;
		deleteError = '';
		try {
			await deleteActivity(Number(page.params.id));
			await goto(resolve('/(app)/rides'));
		} catch (err) {
			deleteError = err instanceof ApiError ? err.message : 'Fahrt konnte nicht gelöscht werden.';
			deleteBusy = false;
		}
	}

	async function copyShareLink() {
		await navigator.clipboard.writeText(shareUrl);
		toast('Link kopiert');
	}

	// MapLibre paint properties take a resolved colour, not a CSS custom
	// property — `var(--x)` works fine in a plain style attribute (the legend
	// bar below) but not here.
	function cssColor(variable: string, fallback: string): string {
		return getComputedStyle(document.documentElement).getPropertyValue(variable).trim() || fallback;
	}

	function trackColor(): string {
		return cssColor('--rk-accent', '#010205');
	}

	// A bright map inside a dark page is the one thing that still looked pasted
	// on. OpenFreeMap ships a dark style ("fiord") next to the light one, same
	// tiles, no key — so the map follows the colour scheme like everything else.
	function mapStyleUrl(): string {
		// A theme chosen in the app (Rekta prefs, data-theme) wins over the system.
		const chosen = document.documentElement.dataset.theme;
		const dark = chosen
			? chosen === 'dark'
			: window.matchMedia?.('(prefers-color-scheme: dark)').matches;
		return `https://tiles.openfreemap.org/styles/${dark ? 'fiord' : 'liberty'}`;
	}

	function hasAnyValue(values: (number | null)[]): boolean {
		return values.some((v) => v !== null);
	}

	// Elapsed minutes since the first sample — a real time axis instead of a
	// bare array index.
	let elapsedMinutes = $derived.by(() => {
		if (samples.length === 0) return [];
		const start = new Date(samples[0].time).getTime();
		return samples.map((s) => (new Date(s.time).getTime() - start) / 60000);
	});

	function formatSpeed(mps: number | null): string {
		if (mps === null) return '–';
		return `${formatDecimal(mps * 3.6, 1)} km/h`;
	}

	// formatDuration (rides.ts) only shows whole minutes, fine for a ride but
	// not for a lap — an auto-lap or a quick button press can be well under a
	// minute, and "0 min" reads as broken rather than short.
	function formatLapTime(seconds: number): string {
		return seconds < 60 ? `${seconds} s` : formatDuration(seconds);
	}

	function formatElapsed(minutes: number): string {
		if (minutes >= 60) {
			const h = Math.floor(minutes / 60);
			const m = Math.round(minutes % 60);
			return `${h}:${String(m).padStart(2, '0')} h`;
		}
		return `${Math.round(minutes)} min`;
	}

	let hasElevation = $derived(hasAnyValue(samples.map((s) => s.altitude_meters)));

	// Belastung (TSS) and Intensität (IF) are the two figures on this page a
	// hobbyist can't read cold — promoted out of the grid into their own
	// bands instead of sitting as one more tile among distance and speed
	// (Nocturne v2). Matched by label since detail_stats is a flat,
	// backend-ordered list with no machine key on the frontend (#735).
	let detailStats: RideStat[] = $derived.by(() => story?.detail_stats ?? []);
	let tssStat: RideStat | null = $derived(detailStats.find((s) => s.label === 'Belastung') ?? null);
	let ifStat: RideStat | null = $derived(
		detailStats.find((s) => s.label.startsWith('Intensität')) ?? null
	);
	let gridStats: RideStat[] = $derived(detailStats.filter((s) => s !== tssStat && s !== ifStat));
	// IF already lives on a real 0–~1.2 scale (1.0 = riding exactly at
	// threshold) — a genuine position to mark, not a fabricated percentage.
	let ifShare = $derived(
		ifStat ? Math.max(0, Math.min(1, parseFloat(ifStat.value.replace(',', '.')))) : 0
	);
	let hasPower = $derived(hasAnyValue(samples.map((s) => s.power_watts)));
	let hasHeartRate = $derived(hasAnyValue(samples.map((s) => s.heart_rate)));
	// Prefer power (more directly trainable) when both are present.
	let showPower = $derived(hasPower);
	let showHeartRate = $derived(!hasPower && hasHeartRate);

	// Kept together (not two independent filters) so a coordinate and its
	// sample — power/heart-rate/time, needed for the colour-coded track below
	// — never drift out of sync with each other.
	let trackSamples = $derived(
		samples.filter(
			(s): s is Sample & { lat: number; lon: number } => s.lat !== null && s.lon !== null
		)
	);
	let gpsCoords = $derived(trackSamples.map((s) => [s.lon, s.lat] as [number, number]));

	// Strecke einfärben nach Tempo/Puls/Leistung (#653): which metric currently
	// colours the track, chosen once the ride's samples say what it recorded
	// (power preferred over heart rate — same priority as the Analyse tab's
	// showPower/showHeartRate), 'none' being the plain single-colour line.
	type ColorMetric = 'none' | 'speed' | 'power' | 'heart_rate';
	let colorMetric: ColorMetric = $state('none');
	let mapReady = $state(false);

	let colorMetrics = $derived([
		{ key: 'none' as ColorMetric, label: 'Einfarbig' },
		...(hasPower ? [{ key: 'power' as ColorMetric, label: 'Leistung' }] : []),
		...(hasHeartRate ? [{ key: 'heart_rate' as ColorMetric, label: 'Puls' }] : []),
		{ key: 'speed' as ColorMetric, label: 'Tempo' }
	]);

	const colorMetricLabels: Record<Exclude<ColorMetric, 'none'>, string> = {
		speed: 'Tempo',
		power: 'Leistung',
		heart_rate: 'Puls'
	};

	function haversineMeters(a: [number, number], b: [number, number]): number {
		const R = 6371000;
		const toRad = (d: number) => (d * Math.PI) / 180;
		const dLat = toRad(b[1] - a[1]);
		const dLon = toRad(b[0] - a[0]);
		const h =
			Math.sin(dLat / 2) ** 2 +
			Math.cos(toRad(a[1])) * Math.cos(toRad(b[1])) * Math.sin(dLon / 2) ** 2;
		return 2 * R * Math.asin(Math.sqrt(h));
	}

	// The value a segment (between two consecutive track points) is coloured
	// by. Speed comes from GPS distance/time rather than a recorded field —
	// samples don't carry one. A implausible spike (>120 km/h) is a GPS
	// glitch, not a descent, and gets dropped rather than compressing the
	// whole ramp into its shadow.
	function segmentValue(
		metric: ColorMetric,
		a: (typeof trackSamples)[number],
		b: (typeof trackSamples)[number]
	): number | null {
		if (metric === 'speed') {
			const seconds = (new Date(b.time).getTime() - new Date(a.time).getTime()) / 1000;
			if (seconds <= 0) return null;
			const kmh = (haversineMeters([a.lon, a.lat], [b.lon, b.lat]) / seconds) * 3.6;
			return kmh <= 120 ? kmh : null;
		}
		if (metric === 'power') {
			if (a.power_watts === null || b.power_watts === null) return null;
			return (a.power_watts + b.power_watts) / 2;
		}
		if (metric === 'heart_rate') {
			if (a.heart_rate === null || b.heart_rate === null) return null;
			return (a.heart_rate + b.heart_rate) / 2;
		}
		return null;
	}

	function percentile(sorted: number[], p: number): number {
		const i = Math.min(sorted.length - 1, Math.max(0, Math.round((sorted.length - 1) * p)));
		return sorted[i];
	}

	// The stretches actually drawn by value — gaps in the chosen metric
	// (sensor dropout) simply have no feature here, so the plain track layer
	// underneath shows through rather than a fabricated value. A stretch is
	// about 15 s at its mean: power and heart rate flicker from second to
	// second, and a width per one-second segment drew as hatching, not as a
	// line that swells where the effort was.
	const STRETCH_MS = 15_000;
	let coloredTrack = $derived.by(() => {
		if (colorMetric === 'none' || trackSamples.length < 2) return null;
		const features: {
			type: 'Feature';
			properties: { value: number };
			geometry: { type: 'LineString'; coordinates: [number, number][] };
		}[] = [];
		const values: number[] = [];
		let coordinates: [number, number][] = [];
		let sum = 0;
		let count = 0;
		let start = 0;
		const close = () => {
			if (count > 0) {
				const value = sum / count;
				features.push({
					type: 'Feature',
					properties: { value },
					geometry: { type: 'LineString', coordinates }
				});
				values.push(value);
			}
			coordinates = [];
			sum = 0;
			count = 0;
		};
		for (let i = 0; i < trackSamples.length - 1; i++) {
			const a = trackSamples[i];
			const b = trackSamples[i + 1];
			const value = segmentValue(colorMetric, a, b);
			if (value === null) {
				close();
				continue;
			}
			if (count === 0) {
				coordinates = [[a.lon, a.lat]];
				start = new Date(a.time).getTime();
			}
			coordinates.push([b.lon, b.lat]);
			sum += value;
			count++;
			if (new Date(b.time).getTime() - start >= STRETCH_MS) close();
		}
		close();
		if (values.length === 0) return null;
		const sorted = [...values].sort((x, y) => x - y);
		// 5th/95th percentile, not the raw min/max — one leftover GPS-speed
		// outlier or a single missed heartbeat reading would otherwise wash out
		// the whole ramp for every other, plausible segment.
		const lo = percentile(sorted, 0.05);
		const hi = Math.max(percentile(sorted, 0.95), lo + 1);
		return { features, lo, hi };
	});

	function legendFormat(metric: ColorMetric, value: number): string {
		if (metric === 'speed') return `${value.toFixed(0)} km/h`;
		if (metric === 'power') return `${Math.round(value)} W`;
		if (metric === 'heart_rate') return `${Math.round(value)} bpm`;
		return '';
	}

	// Temperature only. The average headwind that used to sit here is the
	// hourly figure that cancels itself out on an out-and-back — it showed
	// "0.1 m/s Gegenwind" right above a card saying 50 % of the ride was into
	// a 5.5 m/s wind (#606). Wind is the story's job now, per heading.
	let conditions = $derived.by(() => {
		if (!weather || weather.buckets_enriched === 0) return [];
		if (weather.avg_temperature_celsius == null) return [];
		return [`${Math.round(weather.avg_temperature_celsius)} °C`];
	});

	onMount(async () => {
		try {
			const activityId = Number(page.params.id);
			samples = await getActivitySamples(activityId);
			colorMetric = hasAnyValue(samples.map((s) => s.power_watts))
				? 'power'
				: hasAnyValue(samples.map((s) => s.heart_rate))
					? 'heart_rate'
					: 'speed';
			viewState = 'ready';
			// Both are best-effort: a ride not yet enriched (or without power
			// data) still shows its map and curves rather than an error.
			getActivityStory(activityId)
				.then((s) => (story = s))
				.catch(() => {});
			getActivityWeather(activityId)
				.then((w) => (weather = w))
				.catch(() => {});
			getShareStatus(activityId)
				.then((s) => (shareStatus = s))
				.catch(() => {});
			getActivityLaps(activityId)
				.then((l) => (laps = l))
				.catch(() => {});
			listBikes()
				.then((b) => (bikeList = b))
				.catch(() => {});
			getActivityBike(activityId)
				.then((id) => (currentBikeId = id))
				.catch(() => {});
		} catch (err) {
			errorMessage = err instanceof ApiError ? err.message : 'Fahrt konnte nicht geladen werden.';
			viewState = 'error';
		}
	});

	// The `.map` div only exists in the DOM once viewState is 'ready' *and*
	// gpsCoords has data (see template) — bind:this only fires after that
	// render, so map creation has to react to mapContainer actually showing
	// up rather than run inline in onMount right after the fetch resolves.
	$effect(() => {
		if (map || gpsCoords.length < 2 || !mapContainer) return;
		map = new Map({
			container: mapContainer,
			style: mapStyleUrl(),
			bounds: gpsCoords.reduce((b, c) => b.extend(c), new LngLatBounds(gpsCoords[0], gpsCoords[0])),
			fitBoundsOptions: { padding: 32 }
		});
		map.on('load', () => {
			map?.addSource('track', {
				type: 'geojson',
				data: {
					type: 'Feature',
					properties: {},
					geometry: { type: 'LineString', coordinates: gpsCoords }
				}
			});
			map?.addLayer({
				id: 'track',
				type: 'line',
				source: 'track',
				// MapLibre takes a colour string, not a CSS variable, so the token
				// has to be resolved here — otherwise the track keeps the light
				// scheme's teal on a dark map.
				paint: { 'line-color': trackColor(), 'line-width': 3 }
			});
			mapReady = true;
		});
	});

	// The colour-coded overlay (#653): drawn on top of the plain 'track' layer
	// rather than replacing it, so a gap in the chosen metric just shows the
	// plain line through instead of a hole. Removed and rebuilt from scratch
	// on every change rather than patched in place — the paint expression's
	// colour stops are baked to this metric's own value range, so switching
	// metric always needs a fresh one anyway.
	$effect(() => {
		// mapReady first: `map` is not reactive, and an effect that returns
		// before reading any state never runs again — the overlay never showed.
		if (!mapReady || !map) return;
		if (map.getLayer('track-colored')) {
			map.removeLayer('track-colored');
			map.removeSource('track-colored');
		}
		const colored = coloredTrack;
		if (!colored) return;
		const { features, lo, hi } = colored;
		map.addSource('track-colored', {
			type: 'geojson',
			// geojson-vt's default simplification tolerance drops geometry below
			// its threshold per tile-zoom — each segment here is only the
			// distance between two consecutive samples (often single-digit
			// metres), short enough that entire stretches of the coloured
			// overlay silently vanished at a typical fitBounds zoom before this
			// was set to 0.
			tolerance: 0,
			data: { type: 'FeatureCollection', features }
		});
		map.addLayer({
			id: 'track-colored',
			type: 'line',
			source: 'track-colored',
			// The value is the line's width, in ink: thin where it was low,
			// thick where it was high. Rekta has no colour ramp for data (ADR
			// adr-003-wff-datenfarben-rekta), and on a map the width reads at a
			// glance just as well. Round ends and joins, so one stretch flows into
			// the next instead of a step at every seam.
			layout: { 'line-cap': 'round', 'line-join': 'round' },
			paint: {
				'line-color': trackColor(),
				'line-width': ['interpolate', ['linear'], ['get', 'value'], lo, 3, hi, 12]
			}
		});
	});

	onDestroy(() => {
		map?.remove();
	});

	// MapLibre sizes its WebGL canvas from the container's on-screen dimensions
	// at creation time — a container hidden behind the Übersicht/Analyse tabs
	// is 0×0, so the map would otherwise stay blank forever once the Karte tab
	// is finally shown. resize() re-reads the now-visible container's size.
	$effect(() => {
		if (activeTab !== 'karte' || !map) return;
		map.resize();
		// resize() alone re-reads the canvas size but doesn't recompute the
		// framing — without this the track stays positioned for a 0×0 viewport
		// and can render off to one side.
		map.fitBounds(
			gpsCoords.reduce((b, c) => b.extend(c), new LngLatBounds(gpsCoords[0], gpsCoords[0])),
			{ padding: 32, animate: false }
		);
	});
</script>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<!-- The ride's own title: what kind of ride it was, with the day (and the
	     weather) as its kicker (Rekta: kicker, one bold title). -->
	<header class="head">
		<p class="rk-kicker">{story?.subtitle}{conditions[0] ? ` · ${conditions[0]}` : ''}</p>
		<h1 class="rk-title">{story?.title || 'Deine Fahrt'}</h1>
	</header>

	<!-- Übersicht, Karte, Analyse: facets of one ride (#650), Rekta's facet
	     pivot (rekta.js pivot(): the strip glides, the panel slides in). -->
	<div class="rk-pivot facets" role="tablist" aria-label="Ansicht wählen" use:facetPivot>
		{#each tabs as t (t.key)}
			<button
				type="button"
				role="tab"
				id="ride-tab-{t.key}"
				aria-controls="ride-panel-{t.key}"
				aria-selected={activeTab === t.key}
				tabindex={activeTab === t.key ? 0 : -1}
			>
				{t.label}
			</button>
		{/each}
	</div>

	<div
		role="tabpanel"
		id="ride-panel-uebersicht"
		aria-labelledby="ride-tab-uebersicht"
		hidden={activeTab !== 'uebersicht'}
	>
		<!-- The ride's shape first: a hobbyist reads an elevation profile faster
		     than any number on the page (Nocturne v2). -->
		{#if hasElevation}
			<div class="elevation-strip">
				<LineChart
					xValues={elapsedMinutes}
					series={[{ name: 'Höhe', values: samples.map((s) => s.altitude_meters) }]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} m`}
					ariaLabel="Höhenprofil"
					height={64}
					bare
				/>
			</div>
		{/if}

		<!-- Every figure the ride has data for (story.detail_stats, the data-dense
		     variant chosen in #735), as Rekta tiles. -->
		<section class="rk-tiles figures" aria-label="Kennzahlen">
			{#if tssStat}
				<div class="rk-tile rk-w2" style="--rk-i: 0">
					<span class="rk-stat">
						<b
							>{tssStat.value}{#if tssStat.unit}<small>{tssStat.unit}</small>{/if}</b
						>
						<span>{tssStat.label}</span>
					</span>
				</div>
			{/if}
			{#if ifStat}
				<!-- Intensity as a position, not a bare 1,09 (same idea as Frische). -->
				<div class="rk-tile rk-w2" style="--rk-i: 1">
					<span class="rk-stat"><b>{ifStat.value}</b><span>{ifStat.label}</span></span>
					<span class="fact-scale" style="--fact-share: {ifShare}">
						<span class="fact-scale-marker"></span>
					</span>
					<span class="fact-scale-ends"><span>ruhig</span><span>Vollgas</span></span>
				</div>
			{/if}
			{#each gridStats as stat, i (stat.label)}
				<div class="rk-tile" style="--rk-i: {i + 2}">
					<span class="rk-stat">
						<b
							>{stat.value}{#if stat.unit}<small>{stat.unit}</small>{/if}</b
						>
						<span>{stat.label}</span>
					</span>
				</div>
			{/each}
		</section>

		<StoryCards
			statements={story?.statements ?? []}
			label="Einordnung dieser Fahrt"
			context={{
				...(showPower || showHeartRate
					? {
							effort: {
								xValues: elapsedMinutes,
								series: [
									showPower
										? { name: 'Leistung', values: samples.map((s) => s.power_watts) }
										: { name: 'Herzfrequenz', values: samples.map((s) => s.heart_rate) }
								],
								xFormat: formatElapsed,
								yFormat: (y: number) => `${Math.round(y)}${showPower ? ' W' : ' bpm'}`,
								caption: 'Verlauf während der Fahrt'
							}
						}
					: {}),
				...(hasElevation
					? {
							climb: {
								xValues: elapsedMinutes,
								series: [{ name: 'Höhe', values: samples.map((s) => s.altitude_meters) }],
								xFormat: formatElapsed,
								yFormat: (y: number) => `${Math.round(y)} m`,
								caption: 'Höhenprofil dieser Fahrt'
							}
						}
					: {})
			}}
		/>

		<section class="sec">
			<h2 class="rk-group">Teilen und verwalten</h2>
			{#if shareStatus?.active}
				<div class="rk-field">
					<label class="rk-label" for="share-url"
						>Geteilt per Link — Kennzahlen ohne Login sichtbar</label
					>
					<input
						id="share-url"
						class="rk-input"
						type="text"
						readonly
						value={shareUrl}
						onclick={(e) => e.currentTarget.select()}
					/>
				</div>
				<p class="rk-actions">
					<button class="rk-btn rk-btn--primary" type="button" onclick={copyShareLink}>
						Link kopieren
					</button>
					<button class="rk-btn" type="button" onclick={unshare} disabled={shareBusy}>
						Teilen beenden
					</button>
				</p>
			{:else}
				<p class="rk-actions">
					<button class="rk-btn" type="button" onclick={share} disabled={shareBusy}>
						Per Link teilen
					</button>
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- server download endpoint, not an app route -->
					<a class="rk-btn" href="/api/activities/{page.params.id}/export">Original-Datei laden</a>
				</p>
			{/if}

			{#if bikeList.length > 0}
				<div class="rk-field bike" data-status={bikeError ? 'error' : undefined}>
					<label class="rk-label" for="ride-bike">Rad</label>
					<select
						id="ride-bike"
						class="rk-input"
						value={currentBikeId ?? ''}
						disabled={bikeBusy}
						onchange={changeBike}
					>
						<option value="">Kein Rad</option>
						{#each bikeList as bike (bike.id)}
							<option value={bike.id}>{bike.name}{bike.retired_at ? ' (stillgelegt)' : ''}</option>
						{/each}
					</select>
					{#if bikeError}<p class="rk-hint" role="alert">{bikeError}</p>{/if}
				</div>
			{/if}

			<p class="rk-actions delete">
				<button class="rk-btn" type="button" onclick={deleteRide} disabled={deleteBusy}>
					Fahrt löschen
				</button>
			</p>
			{#if deleteError}
				<p class="rk-empty" data-status="error" role="alert">{deleteError}</p>
			{/if}
		</section>
	</div>

	<div
		role="tabpanel"
		id="ride-panel-karte"
		aria-labelledby="ride-tab-karte"
		hidden={activeTab !== 'karte'}
	>
		<section class="sec first">
			<h2 class="rk-group">Wo du gefahren bist</h2>
			{#if gpsCoords.length > 1}
				<fieldset class="rk-segments">
					<legend class="rk-sr">Strecke zeigen nach</legend>
					{#each colorMetrics as m (m.key)}
						<label>
							<input type="radio" name="track-metric" value={m.key} bind:group={colorMetric} />
							{m.label}
						</label>
					{/each}
				</fieldset>
				<div class="map" bind:this={mapContainer}></div>
				{#if coloredTrack}
					<div class="color-legend">
						<span class="color-legend-bar"></span>
						<span class="rk-meta color-legend-labels">
							<span>{legendFormat(colorMetric, coloredTrack.lo)}</span>
							<span>{colorMetricLabels[colorMetric as Exclude<ColorMetric, 'none'>]}</span>
							<span>{legendFormat(colorMetric, coloredTrack.hi)}</span>
						</span>
					</div>
				{/if}
			{:else}
				<p class="rk-empty">Für diese Fahrt wurde keine Position aufgezeichnet.</p>
			{/if}
		</section>

		<section class="sec">
			<h2 class="rk-group">Bergauf, bergab</h2>
			<p class="sub">Höhe über dem Meer im Verlauf der Fahrt</p>
			{#if hasElevation}
				<LineChart
					xValues={elapsedMinutes}
					series={[{ name: 'Höhe', values: samples.map((s) => s.altitude_meters) }]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} m`}
					ariaLabel="Höhenprofil"
					height={160}
				/>
			{:else}
				<p class="rk-empty">Für diese Fahrt wurden keine Höhendaten aufgezeichnet.</p>
			{/if}
		</section>
	</div>

	<div
		role="tabpanel"
		id="ride-panel-analyse"
		aria-labelledby="ride-tab-analyse"
		hidden={activeTab !== 'analyse'}
	>
		{#if story?.zones && story.zones.total_seconds > 0}
			<section class="sec first">
				<h2 class="rk-group">Wo dein Puls lag</h2>
				<p class="sub">
					Der Durchschnittspuls verrät nicht, ob du gleichmäßig unterwegs warst oder zwischendurch
					hart. Diese Aufteilung schon.
				</p>
				<ZoneBars distribution={story.zones} label="Zeit in den Pulsbereichen dieser Fahrt" />
			</section>
		{/if}

		<section class="sec">
			{#if showPower}
				<h2 class="rk-group">Wie viel Kraft du aufs Pedal gebracht hast</h2>
				<p class="sub">Leistung in Watt — höher heißt kräftiger getreten</p>
				<LineChart
					xValues={elapsedMinutes}
					series={[{ name: 'Leistung', values: samples.map((s) => s.power_watts) }]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} W`}
					ariaLabel="Leistung"
					height={160}
				/>
			{:else if showHeartRate}
				<h2 class="rk-group">Wie schnell dein Herz geschlagen hat</h2>
				<p class="sub">Herzfrequenz in Schlägen pro Minute — höher heißt anstrengender</p>
				<LineChart
					xValues={elapsedMinutes}
					series={[{ name: 'Herzfrequenz', values: samples.map((s) => s.heart_rate) }]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} bpm`}
					ariaLabel="Herzfrequenz"
					height={160}
				/>
			{:else}
				<h2 class="rk-group">Wie anstrengend es war</h2>
				<p class="rk-empty">
					Für diese Fahrt wurden weder Leistung noch Puls aufgezeichnet — dafür braucht das Rad
					einen Leistungsmesser oder einen Pulsgurt.
				</p>
			{/if}
		</section>

		{#if laps.length > 0}
			<section class="sec">
				<h2 class="rk-group">Zwischenzeiten</h2>
				<p class="sub">Runden, die dein Gerät selbst markiert hat</p>
				<!-- Rekta's table: on a narrow place its rows stack (container query). -->
				<div class="rk-table-wrap">
					<table class="rk-table">
						<thead>
							<tr>
								<th scope="col">Runde</th>
								<th scope="col">Zeit</th>
								<th scope="col">Distanz</th>
								<th scope="col">⌀ Leistung</th>
								<th scope="col">⌀ Puls</th>
								<th scope="col">⌀ Tempo</th>
							</tr>
						</thead>
						<tbody>
							{#each laps as lap (lap.lap_index)}
								<tr>
									<th scope="row">Runde {lap.lap_index + 1}</th>
									<td data-label="Zeit">{formatLapTime(lap.elapsed_seconds)}</td>
									<td data-label="Distanz">{formatDistance(lap.distance_meters)}</td>
									<td data-label="⌀ Leistung"
										>{lap.avg_power_watts !== null
											? `${Math.round(lap.avg_power_watts)} W`
											: '–'}</td
									>
									<td data-label="⌀ Puls"
										>{lap.avg_heart_rate !== null
											? `${Math.round(lap.avg_heart_rate)} bpm`
											: '–'}</td
									>
									<td data-label="⌀ Tempo">{formatSpeed(lap.avg_speed_mps)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}
	</div>
{/if}

<style>
	.head {
		margin-bottom: var(--rk-space-m);
	}

	.head .rk-title {
		margin-top: var(--rk-space-xs);
	}

	.facets {
		margin-bottom: var(--rk-space-m);
	}

	.elevation-strip {
		margin-bottom: var(--rk-space-m);
	}

	.figures {
		margin-bottom: var(--rk-space-s);
	}

	/* Intensity's position on a tile: the track in the tile's own text colour,
	   as Rekta draws progress on a tile. */
	.rk-tile .fact-scale {
		display: block;
		height: 4px;
		background: color-mix(in srgb, currentColor 20%, transparent);
	}

	.rk-tile .fact-scale-marker {
		box-shadow: 0 0 0 2px var(--rk-surface);
	}

	.rk-tile .fact-scale-ends {
		margin: 0;
	}

	.sec {
		margin-top: var(--rk-space-xl);
		display: grid;
		gap: var(--rk-space-m);
	}

	.sec.first {
		margin-top: 0;
	}

	.sec > :global(*) {
		max-width: 100%;
	}

	.sub {
		max-width: 60ch;
		margin-top: calc(-1 * var(--rk-space-s));
		color: var(--rk-text-contrast-high);
	}

	.rk-field {
		width: min(100%, 36rem);
	}

	.map {
		width: 100%;
		height: 320px;
	}

	.color-legend {
		width: 100%;
	}

	/* A wedge, thin to thick, the same as the line on the map. */
	.color-legend-bar {
		display: block;
		height: 10px;
		background: var(--rk-accent);
		clip-path: polygon(0 45%, 100% 0, 100% 100%, 0 55%);
	}

	.color-legend-labels {
		display: flex;
		justify-content: space-between;
		margin-top: var(--rk-space-xs);
	}

	.rk-table-wrap {
		width: 100%;
	}
</style>

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

	// Share status (#641): loaded best-effort alongside the story, since a
	// ride still renders fine without knowing whether it's shared yet.
	let shareStatus: ShareStatus | null = $state(null);
	let shareBusy = $state(false);
	let copied = $state(false);
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
		if (!confirm(`${label} endgültig löschen? Das kann nicht rückgängig gemacht werden.`)) {
			return;
		}
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
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	// MapLibre paint properties take a resolved colour, not a CSS custom
	// property — `var(--x)` works fine in a plain style attribute (the legend
	// bar below) but not here.
	function cssColor(variable: string, fallback: string): string {
		return getComputedStyle(document.documentElement).getPropertyValue(variable).trim() || fallback;
	}

	function trackColor(): string {
		return cssColor('--color-track', '#0f766e');
	}

	// Same low-to-high ramp the heart-rate zone bars already use (ZoneBars,
	// #621) — reused here instead of inventing a second colour language for
	// "how hard" (#653).
	function colorRamp(): string[] {
		return [
			cssColor('--zone-recovery', '#94a3b8'),
			cssColor('--zone-endurance', '#2dd4bf'),
			cssColor('--zone-tempo', '#fbbf24'),
			cssColor('--zone-threshold', '#fb923c'),
			cssColor('--zone-vo2', '#fb7185')
		];
	}

	// A bright map inside a dark page is the one thing that still looked pasted
	// on. OpenFreeMap ships a dark style ("fiord") next to the light one, same
	// tiles, no key — so the map follows the colour scheme like everything else.
	function mapStyleUrl(): string {
		const dark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
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
		return `${(mps * 3.6).toFixed(1)} km/h`;
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
	let ifShare = $derived(ifStat ? Math.max(0, Math.min(1, parseFloat(ifStat.value.replace(',', '.')))) : 0);
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

	// The segments actually drawn colour-coded — gaps in the chosen metric
	// (sensor dropout) simply have no feature here, so the plain track layer
	// underneath shows through rather than a fabricated value.
	let coloredTrack = $derived.by(() => {
		if (colorMetric === 'none' || trackSamples.length < 2) return null;
		const features: {
			type: 'Feature';
			properties: { value: number };
			geometry: { type: 'LineString'; coordinates: [number, number][] };
		}[] = [];
		const values: number[] = [];
		for (let i = 0; i < trackSamples.length - 1; i++) {
			const a = trackSamples[i];
			const b = trackSamples[i + 1];
			const value = segmentValue(colorMetric, a, b);
			if (value === null) continue;
			features.push({
				type: 'Feature',
				properties: { value },
				geometry: {
					type: 'LineString',
					coordinates: [
						[a.lon, a.lat],
						[b.lon, b.lat]
					]
				}
			});
			values.push(value);
		}
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
				paint: { 'line-color': trackColor(), 'line-width': 4 }
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
		if (!map || !mapReady) return;
		if (map.getLayer('track-colored')) {
			map.removeLayer('track-colored');
			map.removeSource('track-colored');
		}
		const colored = coloredTrack;
		if (!colored) return;
		const { features, lo, hi } = colored;
		const ramp = colorRamp();
		const step = (hi - lo) / 4;
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
			paint: {
				'line-width': 4,
				'line-color': [
					'interpolate',
					['linear'],
					['get', 'value'],
					lo,
					ramp[0],
					lo + step,
					ramp[1],
					lo + step * 2,
					ramp[2],
					lo + step * 3,
					ramp[3],
					hi,
					ramp[4]
				]
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
	<!-- Sections instead of one long scroll (#650): Übersicht carries the
	     always-relevant story, Karte and Analyse are their own tabs rather
	     than more panels to scroll past. -->
	<div class="tabs" role="tablist">
		<button
			class="tab"
			class:active={activeTab === 'uebersicht'}
			type="button"
			role="tab"
			aria-selected={activeTab === 'uebersicht'}
			onclick={() => (activeTab = 'uebersicht')}
		>
			Übersicht
		</button>
		<button
			class="tab"
			class:active={activeTab === 'karte'}
			type="button"
			role="tab"
			aria-selected={activeTab === 'karte'}
			onclick={() => (activeTab = 'karte')}
		>
			Karte
		</button>
		<button
			class="tab"
			class:active={activeTab === 'analyse'}
			type="button"
			role="tab"
			aria-selected={activeTab === 'analyse'}
			onclick={() => (activeTab = 'analyse')}
		>
			Analyse
		</button>
	</div>

	<div hidden={activeTab !== 'uebersicht'}>
		<!-- Data-dense grid (Nocturne reskin, 2026-08-30) — chosen over the
		     narrative-hero alternative for this page specifically; the dashboard
		     keeps StoryHero's sentence-first shape (#602) unchanged. Every figure
		     the ride actually has data for (story.detail_stats), never a curated
		     2-3 like the hero shows elsewhere. -->
		<div class="ride-header">
			<h1 class="ride-title">{story?.title || 'Deine Fahrt'}</h1>
			<p class="ride-date-mono">
				{story?.subtitle}{conditions[0] ? ` · ${conditions[0]}` : ''}
			</p>
		</div>

		<!-- The ride's shape as its own header (Nocturne v2) — the Übersicht tab
		     otherwise has zero graphics, and a hobbyist reads an elevation
		     profile faster than any number on the page. -->
		{#if hasElevation}
			<div class="bleed elevation-strip">
				<LineChart
					xValues={elapsedMinutes}
					series={[
						{ name: 'Höhe', color: 'var(--chart-elevation)', values: samples.map((s) => s.altitude_meters) }
					]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} m`}
					ariaLabel="Höhenprofil"
					height={64}
					bare
				/>
			</div>
		{/if}

		{#if tssStat}
			<div class="fact">
				<p class="fact-value">{tssStat.value}{#if tssStat.unit}<span class="fact-unit">{tssStat.unit}</span>{/if}<span class="fact-name">{tssStat.label}</span></p>
			</div>
		{/if}

		{#if ifStat}
			<p class="fact-label">{ifStat.label}</p>
			<div class="fact-scale" style="--fact-share: {ifShare}">
				<div class="fact-scale-marker"></div>
			</div>
			<p class="fact-scale-ends"><span>ruhig</span><span>{ifStat.value}</span><span>Vollgas</span></p>
		{/if}

		{#if gridStats.length > 0}
			<div class="stat-grid">
				{#each gridStats as stat, i (stat.label)}
					<div class="fact-tile" style="--i: {i}">
						<p class="fact-tile-value">{stat.value}</p>
						<p class="fact-tile-label">{stat.unit ? `${stat.unit} ` : ''}{stat.label}</p>
					</div>
				{/each}
			</div>
		{/if}

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
										? {
												name: 'Leistung',
												color: 'var(--chart-power)',
												values: samples.map((s) => s.power_watts)
											}
										: {
												name: 'Herzfrequenz',
												color: 'var(--chart-heart-rate)',
												values: samples.map((s) => s.heart_rate)
											}
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
								series: [
									{
										name: 'Höhe',
										color: 'var(--chart-elevation)',
										values: samples.map((s) => s.altitude_meters)
									}
								],
								xFormat: formatElapsed,
								yFormat: (y: number) => `${Math.round(y)} m`,
								caption: 'Höhenprofil dieser Fahrt'
							}
						}
					: {})
			}}
		/>

		{#if shareStatus?.active}
			<div class="share-banner">
				<p>
					<strong>Diese Fahrt ist per Link geteilt</strong> — Kennzahlen sind ohne Login sichtbar.
				</p>
				<div class="share-row">
					<input
						class="input"
						type="text"
						readonly
						value={shareUrl}
						onclick={(e) => e.currentTarget.select()}
					/>
					<button class="btn btn-secondary" type="button" onclick={copyShareLink}>
						{copied ? 'Kopiert!' : 'Kopieren'}
					</button>
					<button class="btn btn-secondary" type="button" onclick={unshare} disabled={shareBusy}>
						Teilen beenden
					</button>
				</div>
			</div>
		{:else}
			<p class="export-hint">
				<button class="link-button" type="button" onclick={share} disabled={shareBusy}>
					Fahrt per Link teilen
				</button>
				· <a href="/api/activities/{page.params.id}/export">Original-Datei herunterladen</a>
			</p>
		{/if}

		{#if bikeList.length > 0}
			<p class="export-hint bike-select-row">
				<label for="ride-bike">Rad:</label>
				<select
					id="ride-bike"
					class="input"
					value={currentBikeId ?? ''}
					disabled={bikeBusy}
					onchange={changeBike}
				>
					<option value="">Kein Rad</option>
					{#each bikeList as bike (bike.id)}
						<option value={bike.id}>{bike.name}{bike.retired_at ? ' (stillgelegt)' : ''}</option>
					{/each}
				</select>
			</p>
			{#if bikeError}
				<p role="alert" class="delete-error">{bikeError}</p>
			{/if}
		{/if}

		<p class="export-hint">
			<button
				class="link-button link-button-danger"
				type="button"
				onclick={deleteRide}
				disabled={deleteBusy}
			>
				Fahrt löschen
			</button>
		</p>
		{#if deleteError}
			<p role="alert" class="delete-error">{deleteError}</p>
		{/if}
	</div>

	<div hidden={activeTab !== 'karte'}>
		<section class="panel">
			<h2>Wo du gefahren bist</h2>
			{#if gpsCoords.length > 1}
				<div class="color-switch" role="group" aria-label="Strecke einfärben nach">
					<button
						type="button"
						class="btn {colorMetric === 'none' ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={colorMetric === 'none'}
						onclick={() => (colorMetric = 'none')}
					>
						Einfarbig
					</button>
					{#if hasPower}
						<button
							type="button"
							class="btn {colorMetric === 'power' ? 'btn-primary' : 'btn-secondary'}"
							aria-pressed={colorMetric === 'power'}
							onclick={() => (colorMetric = 'power')}
						>
							Leistung
						</button>
					{/if}
					{#if hasHeartRate}
						<button
							type="button"
							class="btn {colorMetric === 'heart_rate' ? 'btn-primary' : 'btn-secondary'}"
							aria-pressed={colorMetric === 'heart_rate'}
							onclick={() => (colorMetric = 'heart_rate')}
						>
							Puls
						</button>
					{/if}
					<button
						type="button"
						class="btn {colorMetric === 'speed' ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={colorMetric === 'speed'}
						onclick={() => (colorMetric = 'speed')}
					>
						Tempo
					</button>
				</div>
				<div class="map" bind:this={mapContainer}></div>
				{#if coloredTrack}
					<div class="color-legend">
						<span
							class="color-legend-bar"
							style="background: linear-gradient(to right, var(--zone-recovery), var(--zone-endurance), var(--zone-tempo), var(--zone-threshold), var(--zone-vo2))"
						></span>
						<span class="color-legend-labels">
							<span>{legendFormat(colorMetric, coloredTrack.lo)}</span>
							<span>{colorMetricLabels[colorMetric as Exclude<ColorMetric, 'none'>]}</span>
							<span>{legendFormat(colorMetric, coloredTrack.hi)}</span>
						</span>
					</div>
				{/if}
			{:else}
				<p class="empty">Für diese Fahrt wurde keine Position aufgezeichnet.</p>
			{/if}
		</section>

		<section class="panel">
			<h2>Bergauf, bergab</h2>
			<p class="panel-sub">Höhe über dem Meer im Verlauf der Fahrt</p>
			{#if hasElevation}
				<LineChart
					xValues={elapsedMinutes}
					series={[
						{
							name: 'Höhe',
							color: 'var(--chart-elevation)',
							values: samples.map((s) => s.altitude_meters)
						}
					]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} m`}
					ariaLabel="Höhenprofil"
					height={160}
				/>
			{:else}
				<p class="empty">Für diese Fahrt wurden keine Höhendaten aufgezeichnet.</p>
			{/if}
		</section>
	</div>

	<div hidden={activeTab !== 'analyse'}>
		{#if story?.zones && story.zones.total_seconds > 0}
			<section class="panel">
				<h2>Wo dein Puls lag</h2>
				<p class="panel-sub">
					Der Durchschnittspuls verrät nicht, ob du gleichmäßig unterwegs warst oder zwischendurch
					hart. Diese Aufteilung schon.
				</p>
				<ZoneBars distribution={story.zones} label="Zeit in den Pulsbereichen dieser Fahrt" />
			</section>
		{/if}

		<section class="panel">
			{#if showPower}
				<h2>Wie viel Kraft du aufs Pedal gebracht hast</h2>
				<p class="panel-sub">Leistung in Watt — höher heißt kräftiger getreten</p>
				<LineChart
					xValues={elapsedMinutes}
					series={[
						{
							name: 'Leistung',
							color: 'var(--chart-power)',
							values: samples.map((s) => s.power_watts)
						}
					]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} W`}
					ariaLabel="Leistung"
					height={160}
				/>
			{:else if showHeartRate}
				<h2>Wie schnell dein Herz geschlagen hat</h2>
				<p class="panel-sub">Herzfrequenz in Schlägen pro Minute — höher heißt anstrengender</p>
				<LineChart
					xValues={elapsedMinutes}
					series={[
						{
							name: 'Herzfrequenz',
							color: 'var(--chart-heart-rate)',
							values: samples.map((s) => s.heart_rate)
						}
					]}
					xFormat={formatElapsed}
					yFormat={(y) => `${Math.round(y)} bpm`}
					ariaLabel="Herzfrequenz"
					height={160}
				/>
			{:else}
				<h2>Wie anstrengend es war</h2>
				<p class="empty">
					Für diese Fahrt wurden weder Leistung noch Puls aufgezeichnet — dafür braucht das Rad
					einen Leistungsmesser oder einen Pulsgurt.
				</p>
			{/if}
		</section>

		{#if laps.length > 0}
			<section class="panel">
				<h2>Zwischenzeiten</h2>
				<p class="panel-sub">Runden, die dein Gerät selbst markiert hat</p>
				<div class="laps-scroll">
					<table class="laps">
						<thead>
							<tr>
								<th>Runde</th>
								<th>Zeit</th>
								<th>Distanz</th>
								<th>⌀ Leistung</th>
								<th>⌀ Puls</th>
								<th>⌀ Tempo</th>
							</tr>
						</thead>
						<tbody>
							{#each laps as lap (lap.lap_index)}
								<tr>
									<td>{lap.lap_index + 1}</td>
									<td>{formatLapTime(lap.elapsed_seconds)}</td>
									<td>{formatDistance(lap.distance_meters)}</td>
									<td
										>{lap.avg_power_watts !== null
											? `${Math.round(lap.avg_power_watts)} W`
											: '–'}</td
									>
									<td
										>{lap.avg_heart_rate !== null
											? `${Math.round(lap.avg_heart_rate)} bpm`
											: '–'}</td
									>
									<td>{formatSpeed(lap.avg_speed_mps)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}
	</div>

	<p class="glossary-hint">
		Unsicher, was ein Begriff bedeutet? Im <a href={resolve('/(app)/glossar')}>Glossar</a> steht alles
		erklärt.
	</p>
{/if}

<style>
	.tabs {
		display: flex;
		gap: 0.375rem;
		margin-bottom: 1.25rem;
	}

	.tab {
		flex: 1;
		border: none;
		border-radius: var(--radius-pill);
		padding: 0.5rem 1rem;
		font: inherit;
		font-weight: 700;
		color: var(--color-text-muted);
		background: color-mix(in srgb, var(--color-text) 6%, transparent);
		cursor: pointer;
	}

	.tab.active {
		color: color-mix(in srgb, var(--color-brand) 70%, var(--color-text));
		background: color-mix(in srgb, var(--color-brand) 16%, transparent);
	}

	.panel {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-md);
		margin-bottom: 1.5rem;
	}

	/* Stacked, not a space-between row — a long title (weather note included:
	   "Donnerstag, 18. Juni · 10:00 · 22 °C") plus the title text genuinely
	   don't both fit on one line on a real phone width, and the row version
	   let the date run past its own container with nothing to stop it. */
	.ride-header {
		margin-bottom: 1rem;
	}

	.ride-title {
		margin: 0;
		font-size: var(--text-xl);
	}

	/* Tabular Manrope, not a monospace stack — the mono date was the clearest
	   remaining "industrial control panel" tell on the page (Nocturne v2);
	   tabular-nums gives the same alignment without a second typeface. */
	.ride-date-mono {
		margin: 0.25rem 0 0;
		font-variant-numeric: tabular-nums;
		font-size: var(--text-xs);
		color: var(--color-text-muted);
	}

	.elevation-strip {
		margin-bottom: 1.25rem;
	}

	.fact,
	.fact-scale {
		margin-bottom: 1.25rem;
	}

	.fact-scale-ends {
		margin-bottom: 1.5rem;
	}

	.stat-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem;
		margin-bottom: 1.5rem;
	}

	@media (min-width: 600px) {
		.stat-grid {
			grid-template-columns: repeat(4, 1fr);
		}
	}

	.panel h2 {
		margin: 0;
	}

	.panel-sub {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0.25rem 0 1rem;
	}

	.glossary-hint,
	.export-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.link-button {
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		color: var(--color-brand);
		text-decoration: underline;
		cursor: pointer;
	}

	.link-button-danger {
		color: var(--color-danger);
	}

	.delete-error {
		color: var(--color-danger);
		font-size: var(--text-sm);
		margin: 0.5rem 0 0;
	}

	.bike-select-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.bike-select-row select {
		/* flex-basis:0% via the shorthand overrides the global .input's
		   width:100%, same trick .share-row .input already uses below. */
		flex: 1;
		min-width: 10rem;
		max-width: 16rem;
	}

	/* Prominent on purpose (#641) — a rider who shared a ride and forgot
	   should not have to go looking for that fact. */
	.share-banner {
		background: color-mix(in srgb, var(--color-brand) 10%, var(--color-surface));
		border-radius: var(--radius-md);
		padding: 1rem 1.25rem;
		margin-bottom: 1.5rem;
	}

	.share-banner p {
		margin: 0 0 0.75rem;
	}

	.share-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.share-row .input {
		flex: 1;
		min-width: 12rem;
	}

	.empty {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.map {
		width: 100%;
		height: 320px;
		border-radius: 12px;
		overflow: hidden;
		margin-top: 1rem;
	}

	.color-switch {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.color-legend {
		margin-top: 0.75rem;
	}

	.color-legend-bar {
		display: block;
		height: 8px;
		border-radius: var(--radius-pill);
	}

	.color-legend-labels {
		display: flex;
		justify-content: space-between;
		margin-top: 0.25rem;
		font-size: var(--text-xs);
		color: var(--color-text-muted);
	}

	/* A narrow phone can't fit six columns — scroll the table itself rather
	   than shrink the numbers unreadably or wrap rows onto a second line. */
	.laps-scroll {
		overflow-x: auto;
	}

	.laps {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
		white-space: nowrap;
	}

	.laps th {
		text-align: left;
		color: var(--color-text-muted);
		font-weight: 700;
		padding: 0.5rem 0.75rem;
	}

	.laps td {
		padding: 0.5rem 0.75rem;
		border-top: 1px solid var(--color-border);
	}
</style>

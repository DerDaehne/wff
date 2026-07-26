<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { Map, LngLatBounds, setWorkerUrl } from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import maplibreWorkerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
	import {
		getActivitySamples,
		getActivityWeather,
		getActivityStory,
		type Sample,
		type WeatherSummary,
		type RideStory
	} from '$lib/rides';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';
	import StoryHero from '$lib/components/StoryHero.svelte';
	import StoryCards from '$lib/components/StoryCards.svelte';

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
	let mapContainer: HTMLDivElement = $state()!;
	let map: Map | null = null;

	function trackColor(): string {
		return (
			getComputedStyle(document.documentElement).getPropertyValue('--color-track').trim() ||
			'#0f766e'
		);
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

	function formatElapsed(minutes: number): string {
		if (minutes >= 60) {
			const h = Math.floor(minutes / 60);
			const m = Math.round(minutes % 60);
			return `${h}:${String(m).padStart(2, '0')} h`;
		}
		return `${Math.round(minutes)} min`;
	}

	let hasElevation = $derived(hasAnyValue(samples.map((s) => s.altitude_meters)));
	let hasPower = $derived(hasAnyValue(samples.map((s) => s.power_watts)));
	let hasHeartRate = $derived(hasAnyValue(samples.map((s) => s.heart_rate)));
	// Prefer power (more directly trainable) when both are present.
	let showPower = $derived(hasPower);
	let showHeartRate = $derived(!hasPower && hasHeartRate);

	let gpsCoords = $derived(
		samples
			.filter((s): s is Sample & { lat: number; lon: number } => s.lat !== null && s.lon !== null)
			.map((s) => [s.lon, s.lat] as [number, number])
	);

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
			viewState = 'ready';
			// Both are best-effort: a ride not yet enriched (or without power
			// data) still shows its map and curves rather than an error.
			getActivityStory(activityId)
				.then((s) => (story = s))
				.catch(() => {});
			getActivityWeather(activityId)
				.then((w) => (weather = w))
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
		});
	});

	onDestroy(() => {
		map?.remove();
	});
</script>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<StoryHero {story} fallbackTitle="Deine Fahrt" note={conditions[0]} />

	<StoryCards statements={story?.statements ?? []} label="Einordnung dieser Fahrt" />

	<section class="panel">
		<h2>Wo du gefahren bist</h2>
		{#if gpsCoords.length > 1}
			<div class="map" bind:this={mapContainer}></div>
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
				Für diese Fahrt wurden weder Leistung noch Puls aufgezeichnet — dafür braucht das Rad einen
				Leistungsmesser oder einen Pulsgurt.
			</p>
		{/if}
	</section>

	<p class="glossary-hint">
		Unsicher, was ein Begriff bedeutet? Im <a href={resolve('/(app)/glossar')}>Glossar</a> steht alles
		erklärt.
	</p>
{/if}

<style>
	.panel {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-md);
		margin-bottom: 1.5rem;
	}

	.panel h2 {
		margin: 0;
	}

	.panel-sub {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0.25rem 0 1rem;
	}

	.glossary-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
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
</style>

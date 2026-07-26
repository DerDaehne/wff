<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { Map, LngLatBounds, setWorkerUrl } from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import maplibreWorkerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
	import {
		getActivitySamples,
		getActivityWeather,
		type Sample,
		type WeatherSummary
	} from '$lib/rides';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';

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
	let mapContainer: HTMLDivElement = $state()!;
	let map: Map | null = null;

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
	let effortLabel = $derived(
		showPower ? 'Leistung (W)' : showHeartRate ? 'Herzfrequenz (bpm)' : null
	);

	let gpsCoords = $derived(
		samples
			.filter((s): s is Sample & { lat: number; lon: number } => s.lat !== null && s.lon !== null)
			.map((s) => [s.lon, s.lat] as [number, number])
	);

	let windLabel = $derived.by(() => {
		if (!weather || weather.buckets_enriched === 0 || weather.avg_headwind_mps === null)
			return null;
		const kind = weather.avg_headwind_mps >= 0 ? 'Gegenwind' : 'Rückenwind';
		return `⌀ ${Math.abs(weather.avg_headwind_mps).toFixed(1)} m/s ${kind}`;
	});

	onMount(async () => {
		try {
			const activityId = Number(page.params.id);
			samples = await getActivitySamples(activityId);
			viewState = 'ready';
			// Best-effort — a ride not yet enriched (or without GPS) just shows
			// no weather context, not an error for the whole page.
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
			style: 'https://tiles.openfreemap.org/styles/liberty',
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
				paint: { 'line-color': '#0f766e', 'line-width': 3 }
			});
		});
	});

	onDestroy(() => {
		map?.remove();
	});
</script>

<h1>Fahrt-Detail</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	{#if windLabel !== null || weather?.avg_temperature_celsius != null}
		<p
			class="weather-context"
			title="Durchschnittstemperatur und mittlerer Wind relativ zur Fahrtrichtung während dieser Fahrt"
		>
			{#if weather?.avg_temperature_celsius != null}
				{Math.round(weather.avg_temperature_celsius)}°C
			{/if}
			{#if windLabel !== null}
				· {windLabel}
			{/if}
		</p>
	{/if}
	{#if gpsCoords.length > 1}
		<div class="map" bind:this={mapContainer}></div>
	{:else}
		<p>Keine GPS-Daten für diese Fahrt.</p>
	{/if}

	<h2>Höhenprofil</h2>
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
		<p>Keine Höhendaten für diese Fahrt.</p>
	{/if}

	<h2>{effortLabel ?? 'Leistung/Herzfrequenz'}</h2>
	{#if showPower}
		<LineChart
			xValues={elapsedMinutes}
			series={[
				{ name: 'Leistung', color: 'var(--chart-power)', values: samples.map((s) => s.power_watts) }
			]}
			xFormat={formatElapsed}
			yFormat={(y) => `${Math.round(y)} W`}
			ariaLabel={effortLabel ?? 'Leistung/Herzfrequenz'}
			height={160}
		/>
	{:else if showHeartRate}
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
			ariaLabel={effortLabel ?? 'Leistung/Herzfrequenz'}
			height={160}
		/>
	{:else}
		<p>Keine Leistungs- oder Herzfrequenzdaten für diese Fahrt.</p>
	{/if}
{/if}

<style>
	.map {
		width: 100%;
		height: 300px;
		border-radius: 0.5rem;
		overflow: hidden;
	}

	.weather-context {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin-top: -0.5rem;
	}
</style>

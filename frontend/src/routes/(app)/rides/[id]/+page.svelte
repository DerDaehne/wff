<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { Map, LngLatBounds } from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import { getActivitySamples, type Sample } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let samples: Sample[] = $state([]);
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

	onMount(async () => {
		try {
			samples = await getActivitySamples(Number(page.params.id));
			viewState = 'ready';
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
</style>

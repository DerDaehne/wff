<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { Map, LngLatBounds } from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import { getActivitySamples, type Sample } from '$lib/rides';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let samples: Sample[] = $state([]);
	let mapContainer: HTMLDivElement = $state()!;
	let map: Map | null = null;

	const width = 800;
	const height = 160;
	const pad = 24;

	function buildLine(values: (number | null)[]) {
		const points = values
			.map((v, i) => (v === null ? null : { i, v }))
			.filter((p): p is { i: number; v: number } => p !== null);
		if (points.length < 2) return null;
		const min = Math.min(...points.map((p) => p.v));
		const max = Math.max(...points.map((p) => p.v));
		const xStep = (width - 2 * pad) / Math.max(values.length - 1, 1);
		const x = (i: number) => pad + i * xStep;
		const y = (v: number) =>
			max === min ? height / 2 : height - pad - ((v - min) / (max - min)) * (height - 2 * pad);
		return {
			polyline: points.map((p) => `${x(p.i)},${y(p.v)}`).join(' '),
			min,
			max
		};
	}

	let elevation = $derived(buildLine(samples.map((s) => s.altitude_meters)));
	let powerLine = $derived(buildLine(samples.map((s) => s.power_watts)));
	let hrLine = $derived(buildLine(samples.map((s) => s.heart_rate)));
	// Prefer power (more directly trainable) when both are present.
	let effortLine = $derived(powerLine ?? hrLine);
	let effortLabel = $derived(powerLine ? 'Leistung (W)' : hrLine ? 'Herzfrequenz (bpm)' : null);

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
				data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: gpsCoords } }
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
	{#if elevation}
		<svg viewBox="0 0 {width} {height}" role="img" aria-label="Höhenprofil">
			<polyline points={elevation.polyline} fill="none" stroke="#78716c" stroke-width="2" />
		</svg>
	{:else}
		<p>Keine Höhendaten für diese Fahrt.</p>
	{/if}

	<h2>{effortLabel ?? 'Leistung/Herzfrequenz'}</h2>
	{#if effortLine}
		<svg viewBox="0 0 {width} {height}" role="img" aria-label={effortLabel}>
			<polyline points={effortLine.polyline} fill="none" stroke="#2563eb" stroke-width="2" />
		</svg>
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

	svg {
		width: 100%;
		height: auto;
		max-width: 800px;
	}
</style>

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
		type RideStory,
		type RideStatement
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
	let story: RideStory | null = $state(null);
	let mapContainer: HTMLDivElement = $state()!;
	let map: Map | null = null;

	// Every statement leads with what it MEANS; this label says which question
	// it answers, so nobody has to infer that from the sentence itself.
	const statementLabel: Record<RideStatement['kind'], string> = {
		effort: 'Wie hart war es',
		load: 'Was es gebracht hat',
		context: 'Warum es sich so anfühlte',
		comparison: 'Im Vergleich zu sonst',
		hint_profile: 'Dafür fehlt noch etwas',
		hint_history: 'Dafür fehlt noch etwas'
	};

	// Consecutive statements of the same kind share one card — three separate
	// cards all headed "Warum es sich so anfühlte" (wind, hills, heat) read as
	// a repetition bug rather than as three reasons for the same thing.
	let storyGroups = $derived.by(() => {
		const groups: { kind: RideStatement['kind']; items: RideStatement[] }[] = [];
		for (const statement of story?.statements ?? []) {
			const open = groups.at(-1);
			if (open?.kind === statement.kind) open.items.push(statement);
			else groups.push({ kind: statement.kind, items: [statement] });
		}
		return groups;
	});

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

	// Plain conditions row: always shown when known, even when the numbers are
	// unremarkable — the story only *explains* wind/heat when they were strong
	// enough to matter (#590 keeps its data, the story adds the meaning).
	let conditions = $derived.by(() => {
		if (!weather || weather.buckets_enriched === 0) return [];
		const out: string[] = [];
		if (weather.avg_temperature_celsius != null) {
			out.push(`${Math.round(weather.avg_temperature_celsius)} °C`);
		}
		if (weather.avg_headwind_mps != null) {
			const kind = weather.avg_headwind_mps >= 0 ? 'Gegenwind' : 'Rückenwind';
			out.push(`⌀ ${Math.abs(weather.avg_headwind_mps).toFixed(1)} m/s ${kind}`);
		}
		return out;
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

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<header class="ride-head">
		<h1>{story?.headline ?? 'Deine Fahrt'}</h1>
		{#if conditions.length > 0}
			<p class="conditions">{conditions.join(' · ')}</p>
		{/if}
	</header>

	{#if storyGroups.length > 0}
		<section class="story" aria-label="Einordnung dieser Fahrt">
			{#each storyGroups as group, i (i)}
				<article class="statement statement-{group.kind}">
					<p class="statement-label">{statementLabel[group.kind]}</p>
					{#each group.items as statement, j (j)}
						<p class="statement-text">{statement.text}</p>
						{#if statement.metric}
							<p class="statement-metric">{statement.metric}</p>
						{/if}
					{/each}
					{#if group.kind === 'hint_profile'}
						<a class="statement-action" href={resolve('/(app)/profile')}>
							Werte im Profil eintragen
						</a>
					{/if}
				</article>
			{/each}
		</section>
	{/if}

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
{/if}

<style>
	.ride-head {
		margin-bottom: 1.5rem;
	}

	.ride-head h1 {
		margin-bottom: 0.25rem;
	}

	.conditions {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0;
	}

	/* Statements come first on the page and read as full sentences, with the
	   technical value demoted to a small line underneath (#600, guideline 1). */
	.story {
		display: grid;
		gap: 0.75rem;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		/* Cards size to their own content — without this the short ones
		   stretch to match the tallest in the row and show a big empty box. */
		align-items: start;
		margin-bottom: 2rem;
	}

	.statement {
		border-radius: 16px;
		padding: 1rem 1.25rem;
		box-shadow: var(--shadow-sm);
		background: var(--color-surface);
	}

	.statement-label {
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		margin: 0 0 0.375rem;
	}

	.statement-text {
		font-size: var(--text-base);
		line-height: 1.5;
		margin: 0;
	}

	.statement-metric {
		font-size: var(--text-xs);
		color: var(--color-text-muted);
		margin: 0.5rem 0 0;
	}

	/* Second and later reasons inside one card need air above them. */
	.statement-metric + .statement-text {
		margin-top: 0.875rem;
	}

	.statement-action {
		display: inline-block;
		margin-top: 0.5rem;
		font-size: var(--text-sm);
	}

	/* Colour carries the same meaning as the chart series it refers to:
	   effort = power amber, load = brand teal, context = info blue. */
	.statement-effort {
		background: color-mix(in srgb, var(--chart-power) 10%, var(--color-surface));
	}

	.statement-load {
		background: color-mix(in srgb, var(--color-brand) 10%, var(--color-surface));
	}

	.statement-context {
		background: color-mix(in srgb, var(--color-info) 8%, var(--color-surface));
	}

	.statement-comparison {
		background: var(--color-surface);
	}

	.statement-hint_profile,
	.statement-hint_history {
		background: color-mix(in srgb, var(--color-warning) 8%, var(--color-surface));
	}

	.panel {
		background: var(--color-surface);
		border-radius: 16px;
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

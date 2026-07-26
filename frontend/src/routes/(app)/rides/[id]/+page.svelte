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
		pace: 'Wie schnell du warst',
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
	<!-- Hero: one big number leads, the rest are supporting figures. This is
	     what turns the page from a data sheet into a ride summary. -->
	<header class="hero">
		{#if story?.subtitle}
			<p class="hero-date">{story.subtitle}</p>
		{/if}
		<h1>{story?.title ?? 'Deine Fahrt'}</h1>

		{#if story && story.stats.length > 0}
			<div class="hero-stats">
				{#each story.stats as stat (stat.label)}
					<div class="hero-stat">
						<p class="hero-stat-figure">
							<span class="stat-value">{stat.value}</span>
							<span class="stat-unit">{stat.unit}</span>
						</p>
						<p class="hero-stat-label">{stat.label}</p>
					</div>
				{/each}
			</div>
		{/if}

		{#if story?.intensity}
			<div class="hero-meter">
				<div class="meter" style="--meter-color: var(--chart-power)">
					<span style="width: {story.intensity.percent}%"></span>
				</div>
				<p class="hero-meter-label">
					{story.intensity.label} · {story.intensity.caption}
				</p>
			</div>
		{/if}

		{#if conditions.length > 0}
			<p class="hero-conditions">{conditions.join(' · ')}</p>
		{/if}
	</header>

	{#if storyGroups.length > 0}
		<section class="story" aria-label="Einordnung dieser Fahrt">
			{#each storyGroups as group, i (i)}
				<article class="statement statement-{group.kind}">
					<p class="chip statement-chip">{statementLabel[group.kind]}</p>
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
	.hero {
		background: var(--color-hero-bg);
		color: var(--color-hero-text);
		border-radius: var(--radius-lg);
		padding: 1.75rem;
		margin-bottom: 1.5rem;
		box-shadow: var(--shadow-md);
	}

	.hero-date {
		margin: 0 0 0.25rem;
		font-size: var(--text-xs);
		font-weight: 700;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--color-hero-muted);
	}

	.hero h1 {
		margin: 0 0 1.5rem;
		font-size: var(--text-xl);
		font-weight: 700;
	}

	.hero-stats {
		display: flex;
		flex-wrap: wrap;
		gap: 1.5rem 2.5rem;
	}

	.hero-stat-figure {
		display: flex;
		align-items: baseline;
		gap: 0.375rem;
		margin: 0;
	}

	.hero-stat-figure .stat-unit {
		color: var(--color-hero-muted);
	}

	.hero-stat-label {
		margin: 0.25rem 0 0;
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-hero-muted);
	}

	.hero-meter {
		margin-top: 1.75rem;
		max-width: 26rem;
	}

	.hero-meter .meter {
		background: rgba(255, 255, 255, 0.16);
	}

	.hero-meter-label {
		margin: 0.5rem 0 0;
		font-size: var(--text-sm);
		color: var(--color-hero-muted);
	}

	.hero-conditions {
		margin: 1rem 0 0;
		font-size: var(--text-sm);
		color: var(--color-hero-muted);
	}

	@media (max-width: 600px) {
		.hero {
			padding: 1.25rem;
		}

		/* The big figure has to shrink on a phone or three stats wrap to three
		   rows and the hero eats the whole screen. */
		.hero-stats {
			gap: 1rem 1.5rem;
		}

		.hero-stat-figure .stat-value {
			font-size: var(--text-3xl);
		}
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
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-sm);
		background: var(--color-surface);
	}

	/* The chip carries the colour, so the card itself stays a calm surface —
	   a full-card wash behind body text hurt contrast in the dark scheme. */
	.statement-chip {
		margin: 0 0 0.75rem;
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

	/* Each kind gets the colour of the chart series it refers to, so the same
	   metric wears the same colour everywhere: effort = power amber,
	   pace = speed blue, load = brand teal. Only the chip is tinted — see above. */
	.statement-effort {
		--chip-color: var(--chart-power);
	}

	.statement-load {
		--chip-color: var(--color-brand);
	}

	.statement-pace {
		--chip-color: var(--chart-speed);
	}

	.statement-context {
		--chip-color: var(--color-info);
	}

	.statement-comparison {
		--chip-color: var(--color-text-muted);
	}

	.statement-hint_profile,
	.statement-hint_history {
		--chip-color: var(--color-warning);
	}

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

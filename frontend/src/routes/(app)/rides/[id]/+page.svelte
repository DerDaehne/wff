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
		getActivityLaps,
		formatDistance,
		formatDuration,
		type Sample,
		type WeatherSummary,
		type RideStory,
		type Lap
	} from '$lib/rides';
	import { getShareStatus, createShare, revokeShare, type ShareStatus } from '$lib/share';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';
	import StoryHero from '$lib/components/StoryHero.svelte';
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

	async function copyShareLink() {
		await navigator.clipboard.writeText(shareUrl);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

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
			getShareStatus(activityId)
				.then((s) => (shareStatus = s))
				.catch(() => {});
			getActivityLaps(activityId)
				.then((l) => (laps = l))
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
		<StoryHero {story} fallbackTitle="Deine Fahrt" note={conditions[0]} />

		<StoryCards statements={story?.statements ?? []} label="Einordnung dieser Fahrt" />

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
	</div>

	<div hidden={activeTab !== 'karte'}>
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

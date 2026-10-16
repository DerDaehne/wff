<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import {
		listActivities,
		getActivityWeather,
		formatDistance,
		formatDuration,
		formatDecimal,
		type ActivitySummary,
		type WeatherSummary
	} from '$lib/rides';
	import { ApiError } from '$lib/api';

	// Ride-vs-ride comparison (#595): two whole activities side by side, plus
	// each side's wind context so a slower split doesn't quietly get blamed on
	// fitness when it was headwind. Deliberately not a same-route/segment
	// matcher — Laps (#589) carry no name or geo-matching, that's a separate,
	// much larger feature this ticket doesn't build.
	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let rideA: ActivitySummary | null = $state(null);
	let rideB: ActivitySummary | null = $state(null);
	let weatherA: WeatherSummary | null = $state(null);
	let weatherB: WeatherSummary | null = $state(null);

	function rideDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			weekday: 'short',
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}

	function avgSpeedKmh(ride: ActivitySummary): string {
		if (ride.moving_seconds === 0 || ride.distance_meters === null) return '–';
		return `${formatDecimal((ride.distance_meters / ride.moving_seconds) * 3.6, 1)} km/h`;
	}

	function windLabel(weather: WeatherSummary | null): string {
		if (!weather || weather.buckets_enriched === 0) return 'Keine Winddaten';
		if (weather.avg_headwind_mps === null) return 'Keine Winddaten';
		const kind = weather.avg_headwind_mps >= 0 ? 'Gegenwind' : 'Rückenwind';
		return `⌀ ${formatDecimal(Math.abs(weather.avg_headwind_mps), 1)} m/s ${kind}`;
	}

	onMount(async () => {
		const idA = Number(page.url.searchParams.get('a'));
		const idB = Number(page.url.searchParams.get('b'));
		if (!idA || !idB) {
			errorMessage = 'Zwei Fahrten zum Vergleichen fehlen.';
			viewState = 'error';
			return;
		}
		try {
			const rides = await listActivities();
			rideA = rides.find((r) => r.id === idA) ?? null;
			rideB = rides.find((r) => r.id === idB) ?? null;
			if (!rideA || !rideB) {
				errorMessage = 'Eine der beiden Fahrten wurde nicht gefunden.';
				viewState = 'error';
				return;
			}
			viewState = 'ready';
			// Best-effort, same as the ride-detail page: a ride not yet enriched
			// still compares fine, just without wind context on that side.
			getActivityWeather(idA)
				.then((w) => (weatherA = w))
				.catch(() => {});
			getActivityWeather(idB)
				.then((w) => (weatherB = w))
				.catch(() => {});
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Vergleich konnte nicht geladen werden.';
			viewState = 'error';
		}
	});
	const watts = (w: number | null) => (w !== null ? `${Math.round(w)} W` : '–');
	const bpm = (h: number | null) => (h !== null ? `${Math.round(h)} bpm` : '–');
</script>

<h1 class="rk-title reveal" style="--i: 0">Fahrten im Vergleich</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if rideA && rideB}
	{@const rows = [
		['Dauer', formatDuration(rideA.moving_seconds), formatDuration(rideB.moving_seconds)],
		['Distanz', formatDistance(rideA.distance_meters), formatDistance(rideB.distance_meters)],
		['⌀ Tempo', avgSpeedKmh(rideA), avgSpeedKmh(rideB)],
		['⌀ Leistung', watts(rideA.avg_power_watts), watts(rideB.avg_power_watts)],
		['⌀ Puls', bpm(rideA.avg_heart_rate), bpm(rideB.avg_heart_rate)],
		['Wind', windLabel(weatherA), windLabel(weatherB)]
	]}
	<!-- Rekta hairline rows, one figure per row, the two rides side by side:
	     content over chrome, no box around the comparison. -->
	<ul class="rk-rows compare reveal" style="--i: 1">
		<li class="head">
			<span></span>
			<a class="rk-kicker" href={resolve('/(app)/rides/[id]', { id: String(rideA.id) })}
				>{rideDate(rideA.started_at)}</a
			>
			<a class="rk-kicker" href={resolve('/(app)/rides/[id]', { id: String(rideB.id) })}
				>{rideDate(rideB.started_at)}</a
			>
		</li>
		{#each rows as [label, a, b] (label)}
			<li>
				<span class="rk-meta">{label}</span>
				<b>{a}</b>
				<b>{b}</b>
			</li>
		{/each}
	</ul>
{/if}

<style>
	h1 {
		margin-bottom: var(--rk-space-l);
	}

	.compare > li {
		display: grid;
		grid-template-columns: minmax(5.5rem, 1fr) 2fr 2fr;
		align-items: baseline;
	}

	.compare b {
		font-weight: var(--rk-font-weight-medium);
		font-variant-numeric: tabular-nums;
	}
</style>

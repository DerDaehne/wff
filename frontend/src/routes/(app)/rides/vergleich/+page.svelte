<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import {
		listActivities,
		getActivityWeather,
		formatDistance,
		formatDuration,
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
		return `${((ride.distance_meters / ride.moving_seconds) * 3.6).toFixed(1)} km/h`;
	}

	function windLabel(weather: WeatherSummary | null): string {
		if (!weather || weather.buckets_enriched === 0) return 'Keine Winddaten';
		if (weather.avg_headwind_mps === null) return 'Keine Winddaten';
		const kind = weather.avg_headwind_mps >= 0 ? 'Gegenwind' : 'Rückenwind';
		return `⌀ ${Math.abs(weather.avg_headwind_mps).toFixed(1)} m/s ${kind}`;
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
</script>

<h1>Fahrten im Vergleich</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if rideA && rideB}
	<div class="panel">
		<div class="compare-scroll">
			<table class="compare">
				<thead>
					<tr>
						<th></th>
						<th
							><a href={resolve('/(app)/rides/[id]', { id: String(rideA.id) })}
								>{rideDate(rideA.started_at)}</a
							></th
						>
						<th
							><a href={resolve('/(app)/rides/[id]', { id: String(rideB.id) })}
								>{rideDate(rideB.started_at)}</a
							></th
						>
					</tr>
				</thead>
				<tbody>
					<tr>
						<td>Dauer</td>
						<td>{formatDuration(rideA.moving_seconds)}</td>
						<td>{formatDuration(rideB.moving_seconds)}</td>
					</tr>
					<tr>
						<td>Distanz</td>
						<td>{formatDistance(rideA.distance_meters)}</td>
						<td>{formatDistance(rideB.distance_meters)}</td>
					</tr>
					<tr>
						<td>⌀ Tempo</td>
						<td>{avgSpeedKmh(rideA)}</td>
						<td>{avgSpeedKmh(rideB)}</td>
					</tr>
					<tr>
						<td>⌀ Leistung</td>
						<td
							>{rideA.avg_power_watts !== null ? `${Math.round(rideA.avg_power_watts)} W` : '–'}</td
						>
						<td
							>{rideB.avg_power_watts !== null ? `${Math.round(rideB.avg_power_watts)} W` : '–'}</td
						>
					</tr>
					<tr>
						<td>⌀ Puls</td>
						<td
							>{rideA.avg_heart_rate !== null ? `${Math.round(rideA.avg_heart_rate)} bpm` : '–'}</td
						>
						<td
							>{rideB.avg_heart_rate !== null ? `${Math.round(rideB.avg_heart_rate)} bpm` : '–'}</td
						>
					</tr>
					<tr>
						<td>Wind</td>
						<td>{windLabel(weatherA)}</td>
						<td>{windLabel(weatherB)}</td>
					</tr>
				</tbody>
			</table>
		</div>
	</div>
{/if}

<style>
	.panel {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-md);
	}

	.compare-scroll {
		overflow-x: auto;
	}

	.compare {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}

	.compare th,
	.compare td {
		padding: 0.5rem 0.75rem;
		text-align: left;
		white-space: nowrap;
	}

	.compare thead th {
		border-bottom: 1px solid var(--color-border);
		font-weight: 700;
	}

	.compare tbody td:first-child {
		color: var(--color-text-muted);
	}

	.compare tbody tr + tr td {
		border-top: 1px solid var(--color-border);
	}
</style>

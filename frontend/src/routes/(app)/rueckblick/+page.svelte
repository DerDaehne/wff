<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getYearReview, type YearReview } from '$lib/yearreview';
	import { formatDistance, formatDuration } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import EmptyState from '$lib/components/EmptyState.svelte';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let review: YearReview | null = $state(null);

	let year = $derived(Number(page.url.searchParams.get('year')) || new Date().getFullYear());

	// Re-fetches whenever the year (from the URL) changes — the prev/next
	// links below navigate within this same route rather than reloading it.
	$effect(() => {
		const requested = year;
		viewState = 'loading';
		getYearReview(requested)
			.then((r) => {
				review = r;
				viewState = 'ready';
			})
			.catch((err) => {
				errorMessage =
					err instanceof ApiError ? err.message : 'Rückblick konnte nicht geladen werden.';
				viewState = 'error';
			});
	});

	function rideDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}
</script>

<header class="head reveal" style="--i: 0">
	<h1 class="rk-title">Dein Jahr {year}</h1>
	<p class="rk-actions">
		<a class="rk-btn" href="{resolve('/(app)/rueckblick')}?year={year - 1}">← {year - 1}</a>
		<a class="rk-btn" href="{resolve('/(app)/rueckblick')}?year={year + 1}">{year + 1} →</a>
	</p>
</header>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if review && review.ride_count === 0}
	<EmptyState message="Keine Fahrten im Jahr {year}." />
{:else if review}
	<!-- The year in Rekta tiles; the two records open their ride and wear
	     Rekta's success fill with the word, as a record does on a ride. -->
	<section class="rk-tiles" aria-label="Dein Jahr in Zahlen">
		<div class="rk-tile" style="--rk-i: 0">
			<span class="rk-stat"
				><b>{review.ride_count}</b><span>{review.ride_count === 1 ? 'Fahrt' : 'Fahrten'}</span
				></span
			>
		</div>
		<div class="rk-tile" style="--rk-i: 1">
			<span class="rk-stat"
				><b>{formatDistance(review.distance_meters)}</b><span>Distanz</span></span
			>
		</div>
		<div class="rk-tile" style="--rk-i: 2">
			<span class="rk-stat"
				><b>{Math.round(review.elevation_gain_meters)}<small>hm</small></b><span>Höhenmeter</span
				></span
			>
		</div>
		<div class="rk-tile" style="--rk-i: 3">
			<span class="rk-stat"
				><b>{formatDuration(review.moving_seconds)}</b><span>auf dem Rad</span></span
			>
		</div>
		{#if review.longest_ride}
			<a
				class="rk-tile rk-w2"
				data-status="success"
				style="--rk-i: 4"
				href={resolve('/(app)/rides/[id]', { id: String(review.longest_ride.activity_id) })}
			>
				<span class="rk-tile-word">Rekord</span>
				<span class="rk-kicker">deine weiteste Fahrt</span>
				<span class="rk-stat"
					><b>{formatDistance(review.longest_ride.value)}</b><span
						>am {rideDate(review.longest_ride.started_at)}</span
					></span
				>
			</a>
		{/if}
		{#if review.hardest_ride}
			<a
				class="rk-tile rk-w2"
				data-status="success"
				style="--rk-i: 5"
				href={resolve('/(app)/rides/[id]', { id: String(review.hardest_ride.activity_id) })}
			>
				<span class="rk-tile-word">Rekord</span>
				<span class="rk-kicker">deine härteste Fahrt</span>
				<span class="rk-stat"
					><b>{Math.round(review.hardest_ride.value)}</b><span
						>Belastung, am {rideDate(review.hardest_ride.started_at)}</span
					></span
				>
			</a>
		{/if}
	</section>
{/if}

<style>
	.head {
		display: grid;
		gap: var(--rk-space-s);
		margin-bottom: var(--rk-space-l);
	}
</style>

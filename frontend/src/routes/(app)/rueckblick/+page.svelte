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

<div class="page-center">
	<h1>Dein Jahr {year}</h1>
	<p class="year-nav">
		<a href="{resolve('/(app)/rueckblick')}?year={year - 1}">← {year - 1}</a>
		<a href="{resolve('/(app)/rueckblick')}?year={year + 1}">{year + 1} →</a>
	</p>

	{#if viewState === 'loading'}
		<p>Lädt…</p>
	{:else if viewState === 'error'}
		<p role="alert">{errorMessage}</p>
	{:else if review && review.ride_count === 0}
		<EmptyState message="Keine Fahrten im Jahr {year}." />
	{:else if review}
		<div class="sums">
			<div class="sum">
				<strong>{review.ride_count}</strong>
				<span>{review.ride_count === 1 ? 'Fahrt' : 'Fahrten'}</span>
			</div>
			<div class="sum">
				<strong>{formatDistance(review.distance_meters)}</strong>
				<span>Distanz</span>
			</div>
			<div class="sum">
				<strong>{Math.round(review.elevation_gain_meters)} hm</strong>
				<span>Höhenmeter</span>
			</div>
			<div class="sum">
				<strong>{formatDuration(review.moving_seconds)}</strong>
				<span>Auf dem Rad</span>
			</div>
		</div>

		{#if review.longest_ride}
			<p class="highlight">
				Deine weiteste Fahrt:
				<a href={resolve('/(app)/rides/[id]', { id: String(review.longest_ride.activity_id) })}>
					{formatDistance(review.longest_ride.value)} am {rideDate(review.longest_ride.started_at)}
				</a>
			</p>
		{/if}
		{#if review.hardest_ride}
			<p class="highlight">
				Deine härteste Fahrt:
				<a href={resolve('/(app)/rides/[id]', { id: String(review.hardest_ride.activity_id) })}>
					Belastung {Math.round(review.hardest_ride.value)} am {rideDate(
						review.hardest_ride.started_at
					)}
				</a>
			</p>
		{/if}
	{/if}
</div>

<style>
	.year-nav {
		display: flex;
		justify-content: space-between;
		max-width: 20rem;
		margin: 0 0 1.5rem;
	}

	.sums {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.sum {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		padding: 1rem 1.25rem;
	}

	.sum strong {
		font-size: var(--text-xl);
		font-variant-numeric: tabular-nums;
	}

	.sum span {
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.highlight {
		margin: 0.5rem 0;
	}
</style>

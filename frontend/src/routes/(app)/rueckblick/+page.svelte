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
			<div class="fact-tile sum">
				<p class="fact-tile-value">{review.ride_count}</p>
				<p class="fact-tile-label">{review.ride_count === 1 ? 'Fahrt' : 'Fahrten'}</p>
			</div>
			<div class="fact-tile sum">
				<p class="fact-tile-value">{formatDistance(review.distance_meters)}</p>
				<p class="fact-tile-label">Distanz</p>
			</div>
			<div class="fact-tile sum">
				<p class="fact-tile-value">{Math.round(review.elevation_gain_meters)} hm</p>
				<p class="fact-tile-label">Höhenmeter</p>
			</div>
			<div class="fact-tile sum">
				<p class="fact-tile-value">{formatDuration(review.moving_seconds)}</p>
				<p class="fact-tile-label">Auf dem Rad</p>
			</div>
		</div>

		<!-- These are literally records — the app's own achievement glow is
		     designed for exactly this screen (Nocturne v2). -->
		{#if review.longest_ride}
			<a
				class="fact fact--milestone highlight"
				href={resolve('/(app)/rides/[id]', { id: String(review.longest_ride.activity_id) })}
			>
				<p class="fact-label">Deine weiteste Fahrt</p>
				<p class="fact-value">{formatDistance(review.longest_ride.value)}</p>
				<p class="fact-meaning">am {rideDate(review.longest_ride.started_at)}</p>
			</a>
		{/if}
		{#if review.hardest_ride}
			<a
				class="fact fact--milestone highlight"
				href={resolve('/(app)/rides/[id]', { id: String(review.hardest_ride.activity_id) })}
			>
				<p class="fact-label">Deine härteste Fahrt</p>
				<p class="fact-value">Belastung {Math.round(review.hardest_ride.value)}</p>
				<p class="fact-meaning">am {rideDate(review.hardest_ride.started_at)}</p>
			</a>
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
		gap: 0.5rem;
		margin-bottom: 1.5rem;
	}

	/* The screen everyone already likes for its big numbers was running them
	   at 25px (Nocturne v2). text-4xl (64px) was tried first but these
	   values carry their unit baked into one string ("294 hm", "1:00 h" —
	   formatDistance/formatDuration return one formatted string, not a
	   separate value+unit pair) and wrapped mid-word at that size; text-3xl
	   is the largest that reliably stays on one line at a 2-column phone
	   width and is still a large jump from the original. */
	.sum .fact-tile-value {
		font-size: var(--text-3xl);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.highlight {
		display: block;
		margin: 0.75rem 0;
	}

	.highlight .fact-value {
		font-size: var(--text-2xl);
	}
</style>

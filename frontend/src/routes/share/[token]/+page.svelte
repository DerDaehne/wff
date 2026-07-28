<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getPublicShare, type PublicRideSummary } from '$lib/share';
	import { formatDistance, formatDuration } from '$lib/rides';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let summary: PublicRideSummary | null = $state(null);

	onMount(async () => {
		try {
			summary = await getPublicShare(page.params.token as string);
			viewState = 'ready';
		} catch (err) {
			errorMessage =
				err instanceof ApiError && err.status === 404
					? 'Dieser Link ist nicht gültig oder wurde widerrufen.'
					: 'Fahrt konnte nicht geladen werden.';
			viewState = 'error';
		}
	});

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			weekday: 'short',
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}
</script>

<div class="share-page">
	<div class="card share-card">
		{#if viewState === 'loading'}
			<p>Lädt…</p>
		{:else if viewState === 'error'}
			<h1>Nicht gefunden</h1>
			<p role="alert">{errorMessage}</p>
		{:else if summary}
			<p class="eyebrow">Geteilte Fahrt</p>
			<h1>{formatDate(summary.started_at)}</h1>
			<div class="stats">
				<div class="stat">
					<strong>{formatDistance(summary.distance_meters)}</strong>
					<span>Distanz</span>
				</div>
				<div class="stat">
					<strong>{formatDuration(summary.moving_seconds)}</strong>
					<span>Dauer</span>
				</div>
				{#if summary.elevation_gain_meters}
					<div class="stat">
						<strong>{Math.round(summary.elevation_gain_meters)} hm</strong>
						<span>Höhenmeter</span>
					</div>
				{/if}
				{#if summary.training_stress_score !== null}
					<div class="stat">
						<strong>{Math.round(summary.training_stress_score)}</strong>
						<span>Belastung</span>
					</div>
				{/if}
			</div>
			<p class="hint">Geteilt über WFF — die Strecke selbst ist bei einem Link nicht sichtbar.</p>
		{/if}
	</div>
</div>

<style>
	.share-page {
		display: flex;
		justify-content: center;
		padding-top: 15vh;
	}

	.share-card {
		width: 100%;
		max-width: 24rem;
	}

	.eyebrow {
		color: var(--color-text-muted);
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		margin: 0 0 0.25rem;
	}

	.share-card h1 {
		margin: 0 0 1.25rem;
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(7rem, 1fr));
		gap: 1rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.stat strong {
		font-size: var(--text-xl);
		font-variant-numeric: tabular-nums;
	}

	.stat span {
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.hint {
		margin-top: 1.5rem;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	[role='alert'] {
		color: var(--color-danger);
	}
</style>

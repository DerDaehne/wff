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

<!-- A shared ride for someone without an account: the day as title, its
     figures as Rekta tiles, on the ground (no card). -->
<main class="rk-place share reveal" style="--i: 0">
	{#if viewState === 'loading'}
		<p>Lädt…</p>
	{:else if viewState === 'error'}
		<h1 class="rk-title">Nicht gefunden</h1>
		<p class="rk-empty" data-status="error" role="alert">{errorMessage}</p>
	{:else if summary}
		<p class="rk-kicker">Geteilte Fahrt</p>
		<h1 class="rk-title">{formatDate(summary.started_at)}</h1>
		<div class="rk-tiles">
			<div class="rk-tile" style="--rk-i: 0">
				<span class="rk-stat"
					><b>{formatDistance(summary.distance_meters)}</b><span>Distanz</span></span
				>
			</div>
			<div class="rk-tile" style="--rk-i: 1">
				<span class="rk-stat"
					><b>{formatDuration(summary.moving_seconds)}</b><span>Dauer</span></span
				>
			</div>
			{#if summary.elevation_gain_meters}
				<div class="rk-tile" style="--rk-i: 2">
					<span class="rk-stat"
						><b>{Math.round(summary.elevation_gain_meters)}<small>hm</small></b><span
							>Höhenmeter</span
						></span
					>
				</div>
			{/if}
			{#if summary.training_stress_score !== null}
				<div class="rk-tile" style="--rk-i: 3">
					<span class="rk-stat"
						><b>{Math.round(summary.training_stress_score)}</b><span>Belastung</span></span
					>
				</div>
			{/if}
		</div>
		<p class="rk-facts">Geteilt über WFF — die Strecke selbst ist bei einem Link nicht sichtbar.</p>
	{/if}
</main>

<style>
	.share {
		display: grid;
		align-content: center;
		gap: var(--rk-space-m);
		width: min(100%, 40rem);
		min-height: 100dvh;
		margin-inline: auto;
		padding: var(--rk-space-m);
	}

	.rk-title {
		margin-top: calc(-1 * var(--rk-space-s));
	}
</style>

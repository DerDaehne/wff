<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getCompare, type CompareResponse } from '$lib/compare';
	import { ApiError } from '$lib/api';
	import EmptyState from '$lib/components/EmptyState.svelte';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let result: CompareResponse | null = $state(null);

	onMount(async () => {
		try {
			result = await getCompare();
			viewState = 'ready';
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Vergleich konnte nicht geladen werden.';
			viewState = 'error';
		}
	});

	function formatDelta(delta: number | null): string {
		if (delta === null) return 'Noch keine Grundlage';
		const sign = delta > 0 ? '+' : '';
		return `${sign}${delta.toFixed(1)}`;
	}
</script>

<div class="page-center">
	<h1>Trainingserfolg im Vergleich</h1>

	{#if viewState === 'loading'}
		<p>Lädt…</p>
	{:else if viewState === 'error'}
		<p role="alert">{errorMessage}</p>
	{:else if result && !result.opted_in}
		<EmptyState
			message="Um andere zu sehen, musst du selbst zustimmen — dein Trainingsstand wird dann genauso für die anderen sichtbar."
			actionHref={resolve('/(app)/profile')}
			actionLabel="Im Profil zustimmen"
		/>
	{:else if result}
		<p class="hint">
			Veränderung deines Trainingszustands (CTL) über die letzten vier Wochen — eine relative Zahl,
			bezogen auf die eigene Kapazität, kein Vergleich absoluter Kilometer oder Höhenmeter.
		</p>
		<ul class="entries">
			{#each result.entries as entry (entry.display_name)}
				<li class="entry" class:you={entry.is_you}>
					<span class="name">{entry.display_name}{entry.is_you ? ' (du)' : ''}</span>
					<span
						class="delta"
						class:positive={entry.delta_ctl !== null && entry.delta_ctl > 0}
						class:muted={entry.delta_ctl === null}
					>
						{formatDelta(entry.delta_ctl)}
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		max-width: 60ch;
		margin-bottom: 1.5rem;
	}

	.entries {
		list-style: none;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
		max-width: 30rem;
	}

	.entry {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		padding: 0.875rem 1.25rem;
	}

	.entry.you {
		font-weight: 700;
	}

	.delta {
		font-variant-numeric: tabular-nums;
	}

	.delta.positive {
		color: var(--color-success);
	}

	.delta.muted {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		font-weight: 400;
	}
</style>

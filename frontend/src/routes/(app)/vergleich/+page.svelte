<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getCompare, type CompareResponse } from '$lib/compare';
	import { formatDecimal } from '$lib/rides';
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
		return `${sign}${formatDecimal(delta, 1)}`;
	}
</script>

<h1 class="rk-title reveal" style="--i: 0">Trainingserfolg im Vergleich</h1>

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
	<p class="sub reveal" style="--i: 1">
		Veränderung deines Trainingszustands (CTL) über die letzten vier Wochen — eine relative Zahl,
		bezogen auf die eigene Kapazität, kein Vergleich absoluter Kilometer oder Höhenmeter.
	</p>
	<!-- Hairline rows, no cards; you are the bold row. -->
	<ul class="rk-rows entries reveal" style="--i: 2">
		{#each result.entries as entry (entry.display_name)}
			<li class:you={entry.is_you}>
				<span>{entry.display_name}{entry.is_you ? ' (du)' : ''}</span>
				<span class="delta" class:none={entry.delta_ctl === null}
					>{formatDelta(entry.delta_ctl)}</span
				>
			</li>
		{/each}
	</ul>
{/if}

<style>
	h1 {
		margin-bottom: var(--rk-space-s);
	}

	.sub {
		max-width: 60ch;
		margin-bottom: var(--rk-space-m);
		color: var(--rk-text-contrast-high);
	}

	.entries {
		max-width: 30rem;
	}

	.you {
		font-weight: var(--rk-font-weight-semibold);
	}

	.delta {
		font-size: var(--rk-font-size-m);
		font-variant-numeric: tabular-nums;
	}

	.delta.none {
		font-size: var(--rk-font-size-xs);
		color: var(--rk-text-contrast-high);
	}
</style>

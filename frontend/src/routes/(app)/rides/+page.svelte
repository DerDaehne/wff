<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listActivities, formatDistance, formatDuration, type ActivitySummary } from '$lib/rides';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'empty' | 'error' | 'ready' = $state('loading');
	let rides: ActivitySummary[] = $state([]);
	let errorMessage = $state('');

	onMount(async () => {
		try {
			rides = await listActivities();
			viewState = rides.length === 0 ? 'empty' : 'ready';
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Fahrten konnten nicht geladen werden.';
			viewState = 'error';
		}
	});
</script>

<h1>Fahrten</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'empty'}
	<p>Noch keine Aktivitäten hochgeladen.</p>
	<p><a href={resolve('/(app)/upload')}>Erste Fahrt hochladen</a></p>
{:else}
	<ul class="rides">
		{#each rides as ride (ride.id)}
			<li>
				<a href={resolve('/(app)/rides/[id]', { id: String(ride.id) })}>
					<strong>{new Date(ride.started_at).toLocaleDateString('de-DE')}</strong>
					— {formatDistance(ride.distance_meters)} · {formatDuration(ride.moving_seconds)}
					{#if ride.training_stress_score !== null}
						· TSS {ride.training_stress_score.toFixed(0)}
					{/if}
				</a>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.rides {
		list-style: none;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
	}

	.rides li {
		background: var(--color-surface);
		border-radius: 12px;
		box-shadow: var(--shadow-sm);
		transition:
			box-shadow 0.15s ease,
			transform 0.15s ease;
	}

	.rides li:hover {
		box-shadow: var(--shadow-md);
		transform: translateY(-1px);
	}

	.rides a {
		display: block;
		padding: 0.875rem 1rem;
		color: inherit;
		text-decoration: none;
	}
</style>

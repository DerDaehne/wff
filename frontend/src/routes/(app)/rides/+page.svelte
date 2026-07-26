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

	function rideDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			weekday: 'short',
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}
</script>

<h1>Deine Fahrten</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'empty'}
	<p>Noch keine Fahrten hochgeladen.</p>
	<p><a href={resolve('/(app)/upload')}>Erste Fahrt hochladen</a></p>
{:else}
	<ul class="rides">
		{#each rides as ride (ride.id)}
			<li>
				<a href={resolve('/(app)/rides/[id]', { id: String(ride.id) })}>
					<span class="ride-date">{rideDate(ride.started_at)}</span>
					<span class="ride-figures">
						<!-- Distance leads, as it does in the ride's own hero — the two
						     views should read as the same app. "TSS 101" became a
						     labelled figure: the abbreviation said nothing to anyone who
						     hadn't already looked it up. -->
						<span class="figure">
							<strong>{formatDistance(ride.distance_meters)}</strong>
							<span class="figure-label">Distanz</span>
						</span>
						<span class="figure">
							<strong>{formatDuration(ride.moving_seconds)}</strong>
							<span class="figure-label">Dauer</span>
						</span>
						{#if ride.training_stress_score !== null}
							<span class="figure">
								<strong>{ride.training_stress_score.toFixed(0)}</strong>
								<span class="figure-label">Belastung</span>
							</span>
						{/if}
					</span>
				</a>
			</li>
		{/each}
	</ul>
	<p class="glossary-hint">
		Was „Belastung" genau bedeutet, steht im <a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
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
		border-radius: var(--radius-md);
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
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem 1.5rem;
		padding: 1rem 1.25rem;
		color: inherit;
		text-decoration: none;
	}

	.ride-date {
		font-weight: 700;
	}

	.ride-figures {
		display: flex;
		gap: 1.5rem;
	}

	.figure {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		font-variant-numeric: tabular-nums;
	}

	.figure-label {
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.glossary-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin-top: 1.5rem;
	}
</style>

<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listActivities, formatDistance, formatDuration, type ActivitySummary } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import { getSettings } from '$lib/profile';
	import EmptyState from '$lib/components/EmptyState.svelte';

	let viewState: 'loading' | 'empty' | 'error' | 'ready' = $state('loading');
	let rides: ActivitySummary[] = $state([]);
	let errorMessage = $state('');
	let primaryMetric = $state('distance');

	// The same order the ride's own hero uses (#616) — the list and the detail
	// view must not disagree about what matters. Figures a ride has no value
	// for are dropped, so a preference never leaves a gap.
	function figuresFor(ride: ActivitySummary) {
		const figures = [
			{ metric: 'distance', value: formatDistance(ride.distance_meters), label: 'Distanz' },
			{ metric: 'duration', value: formatDuration(ride.moving_seconds), label: 'Dauer' }
		];
		if (ride.moving_seconds > 0 && ride.distance_meters !== null) {
			figures.push({
				metric: 'speed',
				value: `${((ride.distance_meters / ride.moving_seconds) * 3.6).toFixed(1)} km/h`,
				label: '⌀ Tempo'
			});
		}
		if (ride.training_stress_score !== null) {
			figures.push({
				metric: 'load',
				value: ride.training_stress_score.toFixed(0),
				label: 'Belastung'
			});
		}
		const preferred = figures.filter((f) => f.metric === primaryMetric);
		const rest = figures.filter((f) => f.metric !== primaryMetric);
		return [...preferred, ...rest].slice(0, 3);
	}

	onMount(async () => {
		try {
			rides = await listActivities();
			viewState = rides.length === 0 ? 'empty' : 'ready';
			// Best effort: without the preference the default order stands.
			getSettings()
				.then((settings) => (primaryMetric = settings.primary_metric ?? 'distance'))
				.catch(() => {});
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
	<EmptyState
		message="Noch keine Fahrten hochgeladen."
		actionHref={resolve('/(app)/upload')}
		actionLabel="Erste Fahrt hochladen"
	/>
{:else}
	<ul class="rides">
		{#each rides as ride (ride.id)}
			<li>
				<a href={resolve('/(app)/rides/[id]', { id: String(ride.id) })}>
					<span class="ride-date">{rideDate(ride.started_at)}</span>
					<!-- Same order as the ride's own hero: the list and the detail view
					     must not disagree about what matters. "TSS 101" became a
					     labelled figure — the abbreviation said nothing to anyone who
					     hadn't already looked it up. -->
					<span class="ride-figures">
						{#each figuresFor(ride) as figure (figure.metric)}
							<span class="figure">
								<strong>{figure.value}</strong>
								<span class="figure-label">{figure.label}</span>
							</span>
						{/each}
					</span>
				</a>
				<!-- Fahrt-Charakter auf einen Blick (#633): ruhige Grundlagenfahrt vs.
				     harte Intervalleinheit sind so unterscheidbar, ohne reinzuklicken. -->
				{#if ride.zones && ride.zones.zones.some((z) => z.share > 0)}
					<div
						class="zone-bar"
						role="img"
						aria-label="Pulszonen-Verteilung dieser Fahrt{ride.zones.assumed
							? ' (geschätzt aus beobachtetem Maximalpuls)'
							: ''}"
					>
						{#each ride.zones.zones.filter((z) => z.share > 0) as zone (zone.key)}
							<span
								class="zone-segment"
								style="width: {zone.share * 100}%; background: var(--zone-{zone.key})"
							></span>
						{/each}
					</div>
				{/if}
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

	.zone-bar {
		display: flex;
		height: 0.3rem;
		margin: 0 1.25rem 0.875rem;
		border-radius: var(--radius-pill);
		overflow: hidden;
		background: var(--color-border);
	}

	.zone-segment {
		height: 100%;
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

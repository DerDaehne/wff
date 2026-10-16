<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		listActivities,
		formatDistance,
		formatDuration,
		formatDecimal,
		type ActivitySummary
	} from '$lib/rides';
	import { ApiError } from '$lib/api';
	import { getSettings } from '$lib/profile';
	import { zoneCharacter } from '$lib/zones';
	import EmptyState from '$lib/components/EmptyState.svelte';

	let viewState: 'loading' | 'empty' | 'error' | 'ready' = $state('loading');
	let rides: ActivitySummary[] = $state([]);
	let errorMessage = $state('');
	let primaryMetric = $state('distance');

	// Ride-vs-ride comparison (#595): a lightweight selection mode rather than
	// a permanent checkbox on every row — most visits to this list never
	// compare anything.
	let compareMode = $state(false);
	let selectedIds: number[] = $state([]);

	function toggleCompareMode() {
		compareMode = !compareMode;
		selectedIds = [];
	}

	function toggleSelected(id: number) {
		if (selectedIds.includes(id)) {
			selectedIds = selectedIds.filter((i) => i !== id);
		} else if (selectedIds.length < 2) {
			selectedIds = [...selectedIds, id];
		}
	}

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
				value: `${formatDecimal((ride.distance_meters / ride.moving_seconds) * 3.6, 1)} km/h`,
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

	// Grouped by month so a growing history has an anchor to scroll against
	// (#635) — rides arrive newest-first from the backend, so a linear scan
	// keeping the running group open is enough, no separate sort needed.
	let monthGroups = $derived.by(() => {
		const out: { key: string; label: string; rides: ActivitySummary[] }[] = [];
		for (const ride of rides) {
			const d = new Date(ride.started_at);
			const key = `${d.getFullYear()}-${d.getMonth()}`;
			const open = out.at(-1);
			if (open?.key === key) open.rides.push(ride);
			else out.push({ key, label: monthLabel(d), rides: [ride] });
		}
		return out;
	});

	function monthLabel(d: Date): string {
		const label = d.toLocaleDateString('de-DE', { month: 'long', year: 'numeric' });
		return label.charAt(0).toUpperCase() + label.slice(1);
	}
</script>

<!-- The pivot above already names this place ("Fahrten"); a visible title
     would say it twice (Rekta typography), so the heading is for screen
     readers only. -->
<h1 class="rk-sr">Deine Fahrten</h1>

{#if viewState === 'ready'}
	<p class="rk-actions tools reveal" style="--i: 0">
		<button class="rk-btn" type="button" onclick={toggleCompareMode}>
			{compareMode ? 'Abbrechen' : 'Fahrten vergleichen'}
		</button>
		{#if compareMode}
			{#if selectedIds.length === 2}
				<a
					class="rk-btn rk-btn--primary"
					href="{resolve('/(app)/rides/vergleich')}?a={selectedIds[0]}&b={selectedIds[1]}"
				>
					Vergleichen
				</a>
			{:else}
				<span class="rk-hint">Wähle zwei Fahrten aus.</span>
			{/if}
		{/if}
	</p>
{/if}

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
	{#each monthGroups as group, i (group.key)}
		<section class="month reveal" style="--i: {i + 1}">
			<h2 class="rk-kicker">{group.label}</h2>
			<!-- Rekta hairline rows: the row is the link (it tilts back where it is
			     pressed, data-rk-press); in compare mode a checkbox leads it. -->
			<ul class="rk-rows">
				{#each group.rides as ride (ride.id)}
					<li>
						{#if compareMode}
							<label class="rk-switch pick">
								<input
									type="checkbox"
									checked={selectedIds.includes(ride.id)}
									disabled={!selectedIds.includes(ride.id) && selectedIds.length >= 2}
									onchange={() => toggleSelected(ride.id)}
								/>
								<span class="rk-sr">{rideDate(ride.started_at)} auswählen</span>
							</label>
						{/if}
						<a
							class="ride"
							href={resolve('/(app)/rides/[id]', { id: String(ride.id) })}
							data-rk-press
						>
							<span class="when">
								<b>{rideDate(ride.started_at)}</b>
								{#if ride.is_shared}<span class="rk-tag">geteilt</span>{/if}
							</span>
							<!-- The ride's character in words (#633), right under the date. -->
							{#if ride.zones && ride.zones.zones.some((z) => z.share > 0)}
								<span class="rk-meta"
									>{zoneCharacter(ride.zones)}{ride.zones.assumed ? ' (geschätzt)' : ''}</span
								>
							{/if}
							<!-- Same order as the ride's own figures: list and detail agree on
							     what matters. -->
							<span class="figures">
								{#each figuresFor(ride) as figure (figure.metric)}
									<span class="figure">
										<b>{figure.value}</b>
										<span class="rk-meta">{figure.label}</span>
									</span>
								{/each}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
	<p class="rk-facts glossary-hint">
		Was „Belastung" genau bedeutet, steht im <a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
{/if}

<style>
	.tools {
		margin-bottom: var(--rk-space-m);
	}

	.month + .month {
		margin-top: var(--rk-space-l);
	}

	.month .rk-rows {
		margin-top: var(--rk-space-xs);
	}

	.rk-rows > li {
		flex-wrap: nowrap;
		align-items: center;
	}

	.pick {
		flex: none;
	}

	/* The whole row is the link: date, character and figures stacked, the
	   same structure for every ride whatever its figures (#643). */
	.ride {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
		padding-block: var(--rk-space-xs);
		color: inherit;
		text-decoration: none;
	}

	.when {
		display: flex;
		align-items: center;
		gap: var(--rk-space-s);
	}

	.figures {
		display: flex;
		flex-wrap: wrap;
		gap: 0 var(--rk-space-l);
		margin-top: var(--rk-space-xs);
	}

	.figure {
		display: grid;
	}

	.figure b {
		font-size: var(--rk-font-size-m);
		font-weight: var(--rk-font-weight-medium);
		font-variant-numeric: tabular-nums;
	}

	.glossary-hint {
		margin-top: var(--rk-space-xl);
	}
</style>

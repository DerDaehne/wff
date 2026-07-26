<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { listActivities, type RideStory } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';
	import StoryHero from '$lib/components/StoryHero.svelte';
	import StoryCards from '$lib/components/StoryCards.svelte';

	// 'no-activities' (upload something) and 'no-training-load' (activities
	// exist, but none has a computed TSS — almost always missing FTP/LTHR in
	// the profile, not actually "nothing uploaded") are deliberately distinct:
	// they need different, accurate messages.
	let viewState: 'loading' | 'no-activities' | 'no-training-load' | 'error' | 'ready' =
		$state('loading');
	let series: DayLoad[] = $state([]);
	let insights: Insight[] = $state([]);
	let status: RideStory | null = $state(null);
	let errorMessage = $state('');

	onMount(async () => {
		try {
			const [trainingLoad, activities] = await Promise.all([getTrainingLoad(), listActivities()]);
			series = trainingLoad.series;
			insights = trainingLoad.insights;
			status = trainingLoad.status;
			if (series.length > 0) {
				viewState = 'ready';
			} else if (activities.length > 0) {
				viewState = 'no-training-load';
			} else {
				viewState = 'no-activities';
			}
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Trainingsdaten konnten nicht geladen werden.';
			viewState = 'error';
		}
	});

	let dayTimestamps = $derived(series.map((d) => new Date(d.date).getTime()));

	function formatDay(ts: number): string {
		return new Date(ts).toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit' });
	}

	let latestTSB = $derived(series.length > 0 ? series[series.length - 1].tsb : null);
	let tsbColor = $derived(
		latestTSB === null
			? 'var(--chart-tsb)'
			: latestTSB < -10
				? 'var(--color-danger)'
				: latestTSB > 5
					? 'var(--color-success)'
					: 'var(--chart-tsb)'
	);

	const severityIcon: Record<Insight['severity'], string> = {
		success: '✓',
		warning: '⚠',
		info: 'ℹ'
	};
</script>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'no-activities'}
	<h1>Willkommen</h1>
	<p>Noch keine Aktivitäten hochgeladen.</p>
	<p><a href={resolve('/(app)/upload')}>Erste Fahrt hochladen</a></p>
{:else if viewState === 'no-training-load'}
	<h1>Fast fertig</h1>
	<p>
		Fahrten sind hochgeladen, aber es fehlt eine FTP- oder Puls-Schwellenwert-Konfiguration, um
		daraus eine Trainingsbelastung zu berechnen.
	</p>
	<p><a href={resolve('/(app)/profile')}>FTP/LTHR im Profil hinterlegen</a></p>
{:else if viewState === 'ready'}
	<!-- The answer first: how you are and whether you're getting better. The
	     chart below is the evidence, not the message (#602). -->
	<StoryHero story={status} fallbackTitle="Dein Trainingsstand" meterColor="var(--chart-ctl)" />
	<StoryCards statements={status?.statements ?? []} label="Dein aktueller Trainingsstand" />

	<section class="panel">
		<h2>Wie sich das entwickelt hat</h2>
		<p class="panel-sub">
			Fitness wächst langsam über Wochen, Müdigkeit steigt und fällt mit den letzten Tagen. Wo die
			Frische-Linie über null liegt, bist du erholt.
		</p>
		<LineChart
			xValues={dayTimestamps}
			series={[
				{
					name: 'Fitness',
					color: 'var(--chart-ctl)',
					values: series.map((d) => d.ctl),
					description:
						'Fachbegriff CTL: dein Trainingsumfang der letzten 6 Wochen. Steigt langsam, fällt langsam — das ist deine Grundlage.'
				},
				{
					name: 'Müdigkeit',
					color: 'var(--chart-atl)',
					values: series.map((d) => d.atl),
					description:
						'Fachbegriff ATL: die Belastung der letzten Woche. Reagiert schnell auf einzelne harte Fahrten.'
				},
				{
					name: 'Frische',
					color: tsbColor,
					values: series.map((d) => d.tsb),
					description:
						'Fachbegriff TSB: Fitness minus Müdigkeit. Über null bist du erholt, unter null steckst du in einer Belastungsphase.'
				}
			]}
			xFormat={formatDay}
			yFormat={(y) => (Math.abs(y) < 0.05 ? '0' : y.toFixed(1))}
			ariaLabel="Verlauf von Fitness, Müdigkeit und Frische"
		/>
	</section>

	<section class="panel">
		<h2>Was du jetzt tun kannst</h2>
		<p class="panel-sub">
			Aus deinem Trainingsverlauf abgeleitet — mit Begründung, damit du es nachvollziehen kannst.
		</p>
		<div class="insights">
			{#each insights as insight, i (i)}
				<div class="insight insight-{insight.severity}">
					<span class="insight-icon">{severityIcon[insight.severity]}</span>
					<div>
						{#if insight.action}<p class="insight-action">{insight.action}</p>{/if}
						<p class="insight-reason">{insight.reason}</p>
					</div>
				</div>
			{/each}
		</div>
	</section>

	<p class="glossary-hint">
		Fitness, Müdigkeit, Frische — was dahinter steckt, steht im
		<a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
{/if}

<style>
	.panel {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-md);
		margin-bottom: 1.5rem;
	}

	.panel h2 {
		margin: 0;
	}

	.panel-sub {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0.25rem 0 1rem;
		max-width: 60ch;
	}

	.glossary-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.insights {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.insight {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		border-radius: var(--radius-sm);
		padding: 0.75rem 1rem;
		background: color-mix(in srgb, var(--color-info) var(--wash-strength), var(--wash-base));
	}

	/* The instruction carries the weight; the reason explains it underneath. */
	.insight-action {
		font-weight: 700;
		margin: 0 0 0.25rem;
	}

	.insight-reason {
		margin: 0;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.insight-icon {
		font-weight: 700;
		flex-shrink: 0;
	}

	.insight-success {
		background: color-mix(in srgb, var(--color-success) var(--wash-strength), var(--wash-base));
	}

	.insight-warning {
		background: color-mix(in srgb, var(--color-warning) var(--wash-strength), var(--wash-base));
	}
</style>

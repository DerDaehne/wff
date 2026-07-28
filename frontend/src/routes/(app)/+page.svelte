<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { listActivities, type RideStory } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import {
		getProgress,
		metricKeyFor,
		progressMetrics,
		type Progress,
		type ProgressMetricKey
	} from '$lib/progress';
	import { getSettings } from '$lib/profile';
	import { getPowerCurve, type PowerCurveHistoryPoint } from '$lib/powercurve';
	import LineChart from '$lib/components/LineChart.svelte';
	import StoryHero from '$lib/components/StoryHero.svelte';
	import StoryCards from '$lib/components/StoryCards.svelte';
	import ZoneBars from '$lib/components/ZoneBars.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';

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
	let progress: Progress | null = $state(null);
	let metricKey: ProgressMetricKey = $state('avg_speed_kmh');
	let metric = $derived(progressMetrics.find((m) => m.key === metricKey)!);

	// Power-curve trend (#593): "is my power at this duration going up",
	// answered by the ride-by-ride figures themselves rather than a
	// monotonic best-ever line. Fetched per duration, not all three at once —
	// three durations is little enough that refetching on a click is simpler
	// than caching all of them client-side.
	const powerCurveDurations = [
		{ seconds: 300, label: '5 min' },
		{ seconds: 1200, label: '20 min' },
		{ seconds: 3600, label: '60 min' }
	];
	let powerCurveDuration = $state(1200);
	let powerCurveHistory: PowerCurveHistoryPoint[] = $state([]);
	// The rider's own configured FTP (#562), for the reference line below —
	// null just means "not set", not "zero".
	let configuredFtpWatts: number | null = $state(null);

	$effect(() => {
		const duration = powerCurveDuration;
		getPowerCurve(duration)
			.then((h) => (powerCurveHistory = h))
			.catch(() => {});
	});

	let powerCurveTimestamps = $derived(
		powerCurveHistory.map((p) => new Date(p.started_at).getTime())
	);

	// FTP estimate (#594): the usual 95%-of-best-20-minutes rule, applied to
	// the same per-ride figures the trend chart already shows — only
	// meaningful at the 20-minute window, never at 5 or 60.
	let estimatedFtpSeries = $derived(powerCurveHistory.map((p) => p.watts * 0.95));
	let configuredFtpSeries = $derived(
		configuredFtpWatts !== null ? powerCurveHistory.map(() => configuredFtpWatts as number) : null
	);

	let powerCurveSeries = $derived.by(() => {
		const series: { name: string; color: string; values: number[]; description?: string }[] = [
			{
				name: 'Leistung',
				color: 'var(--chart-power)',
				values: powerCurveHistory.map((p) => p.watts)
			}
		];
		if (powerCurveDuration !== 1200) return series;
		series.push({
			name: 'Geschätzte FTP (95 %)',
			color: 'var(--color-brand)',
			values: estimatedFtpSeries,
			description:
				'Übliche Faustregel: 95 % der besten 20-Minuten-Leistung einer Fahrt gilt als Schätzung für die Schwellenleistung (FTP).'
		});
		if (configuredFtpSeries) {
			series.push({
				name: 'Deine hinterlegte FTP',
				color: 'var(--color-text-muted)',
				values: configuredFtpSeries
			});
		}
		return series;
	});

	onMount(async () => {
		try {
			const [trainingLoad, activities] = await Promise.all([getTrainingLoad(), listActivities()]);
			series = trainingLoad.series;
			insights = trainingLoad.insights;
			status = trainingLoad.status;
			// Best effort: the weekly history is an extra panel, not a reason to
			// fail the whole page.
			getProgress()
				.then((p) => (progress = p))
				.catch(() => {});
			// The chart opens on the figure the rider chose in their profile
			// (#616); failing to read it just means the default stands.
			getSettings()
				.then((settings) => {
					metricKey = metricKeyFor(settings.primary_metric);
					configuredFtpWatts = settings.ftp_watts;
				})
				.catch(() => {});
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
	<EmptyState
		heading="Willkommen"
		message="Noch keine Aktivitäten hochgeladen."
		actionHref={resolve('/(app)/upload')}
		actionLabel="Erste Fahrt hochladen"
	/>
{:else if viewState === 'no-training-load'}
	<EmptyState
		heading="Fast fertig"
		message="Fahrten sind hochgeladen, aber es fehlt eine FTP- oder Puls-Schwellenwert-Konfiguration, um daraus eine Trainingsbelastung zu berechnen."
		actionHref={resolve('/(app)/profile')}
		actionLabel="FTP/LTHR im Profil hinterlegen"
	/>
{:else if viewState === 'ready'}
	<!-- The answer first: how you are and whether you're getting better. The
	     chart below is the evidence, not the message (#602). -->
	<StoryHero story={status} fallbackTitle="Dein Trainingsstand" meterColor="var(--chart-ctl)" />
	<StoryCards statements={status?.statements ?? []} label="Dein aktueller Trainingsstand" />

	<p class="year-review-link">
		<a href={resolve('/(app)/rueckblick')}>Dein Jahresrückblick ansehen →</a>
	</p>

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

	{#if progress && progress.weeks.length > 0}
		<section class="panel">
			<h2>Wirst du besser?</h2>
			<p class="panel-sub">
				Woche für Woche statt Fahrt für Fahrt — eine einzelne Ausfahrt sagt zu wenig, weil Wind und
				Strecke sie zu stark bestimmen.
			</p>
			<div class="metric-switch" role="group" aria-label="Kennzahl wählen">
				{#each progressMetrics as m (m.key)}
					<button
						type="button"
						class="btn {metricKey === m.key ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={metricKey === m.key}
						onclick={() => (metricKey = m.key)}
					>
						{m.label}
					</button>
				{/each}
			</div>
			<LineChart
				xValues={progress.weeks.map((w) => new Date(w.start).getTime())}
				series={[
					{
						name: metric.label,
						color: metric.color,
						values: progress.weeks.map((w) => w[metric.key])
					}
				]}
				xFormat={formatDay}
				yFormat={metric.format}
				ariaLabel="Wochenverlauf {metric.label}"
				height={200}
			/>
		</section>
		<StoryCards statements={progress.statements} label="Fortschritt über die Wochen" />
	{/if}

	<!-- Panel only when there are bars to draw: without a threshold pulse the
	     heading would introduce an empty box, and the hint card below says the
	     same thing without pretending there is a chart. -->
	{#if progress && progress.zones.total_seconds > 0}
		<section class="panel">
			<h2>Wie hart fährst du eigentlich?</h2>
			<p class="panel-sub">
				Die letzten vier Wochen, aufgeteilt nach Puls. Die Verteilung entscheidet mehr über den
				Fortschritt als jede einzelne Fahrt — und der häufigste Fehler ist, ständig im mittleren
				Bereich zu fahren.
			</p>
			<ZoneBars
				distribution={progress.zones}
				label="Zeit in den Pulsbereichen der letzten Wochen"
			/>
		</section>
	{/if}
	{#if progress}
		<StoryCards statements={progress.zones.statements} label="Deine Verteilung" />
	{/if}

	{#if progress && progress.endurance.statements.length > 0}
		<section class="panel">
			<h2>Arbeitet dein Herz effizienter?</h2>
			<p class="panel-sub">
				{#if progress.endurance.from_power}
					Wie viel Leistung du je 100 Herzschläge bekommst. Steigt die Linie, leistet dein Herz für
					dieselben Watt weniger Arbeit — das ist Ausdauer, die sich aufbaut.
				{:else}
					Wie viel Tempo du je 100 Herzschläge bekommst. Steigt die Linie, leistet dein Herz für
					dasselbe Tempo weniger Arbeit — das ist Ausdauer, die sich aufbaut.
				{/if}
				Hier zählen nur Fahrten, die untereinander vergleichbar sind: ruhig, mindestens eine halbe Stunde,
				mit Puls und von ähnlicher Länge. Eine kurze Runde mit Rückenwind würde die Linie sonst nach oben
				ziehen, ohne dass sich etwas verbessert hat.
			</p>
			{#if progress.endurance.weeks.length >= 2}
				<LineChart
					xValues={progress.endurance.weeks.map((w) => new Date(w.start).getTime())}
					series={[
						{
							name: progress.endurance.from_power ? 'Leistung je Puls' : 'Tempo je Puls',
							color: 'var(--chart-heart-rate)',
							values: progress.endurance.weeks.map((w) => w.value)
						}
					]}
					xFormat={formatDay}
					yFormat={(v: number) =>
						progress?.endurance.from_power ? String(Math.round(v)) : v.toFixed(1)}
					ariaLabel="Wochenverlauf {progress.endurance.unit}"
					height={200}
				/>
				<p class="chart-unit">{progress.endurance.unit}</p>
			{/if}
		</section>
		<StoryCards statements={progress.endurance.statements} label="Ausdauer über die Wochen" />
	{/if}

	{#if progress}
		<section class="panel">
			<h2>Wird deine Leistung besser?</h2>
			<p class="panel-sub">
				Deine beste Leistung für eine feste Dauer, Fahrt für Fahrt — schwankt naturgemäß mit
				Tagesform und Bedingungen, aber ein Anstieg über mehrere Wochen ist echter Fortschritt.
			</p>
			<div class="duration-switch" role="group" aria-label="Dauer wählen">
				{#each powerCurveDurations as d (d.seconds)}
					<button
						type="button"
						class="btn {powerCurveDuration === d.seconds ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={powerCurveDuration === d.seconds}
						onclick={() => (powerCurveDuration = d.seconds)}
					>
						{d.label}
					</button>
				{/each}
			</div>
			{#if powerCurveHistory.length > 0}
				<LineChart
					xValues={powerCurveTimestamps}
					series={powerCurveSeries}
					xFormat={formatDay}
					yFormat={(y) => `${Math.round(y)} W`}
					ariaLabel="Verlauf der besten Leistung"
					height={200}
				/>
			{:else}
				<p class="empty">
					Noch keine Fahrt mit Leistungsdaten, die lang genug für diese Dauer war.
				</p>
			{/if}
		</section>
	{/if}

	<p class="glossary-hint">
		Fitness, Müdigkeit, Frische — was dahinter steckt, steht im
		<a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
{/if}

<style>
	.year-review-link {
		margin: 0 0 1.5rem;
		font-size: var(--text-sm);
	}

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

	.chart-unit {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0.25rem 0 0;
		text-align: right;
	}

	.metric-switch,
	.duration-switch {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.empty {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
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

<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { listActivities, type ActivitySummary, type RideStory } from '$lib/rides';
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
	let activities: ActivitySummary[] = $state([]);

	// The merged "Wirst du besser?" section (Nocturne v3, direction B): one
	// pill row picks which of six figures the chart below shows — the four
	// weekly Week fields, plus the two figures ("Puls-Effizienz", "Leistung")
	// that used to be their own separate panels with their own empty charts.
	// Folding them together means every pill always has a chart frame to
	// land in, instead of three sections where two are usually a heading, a
	// paragraph and a placeholder.
	type PillKey = ProgressMetricKey | 'endurance' | 'power';
	let activePill: PillKey = $state('avg_speed_kmh');
	let activeProgressMetric = $derived(progressMetrics.find((m) => m.key === activePill));

	// The four "auf einen Blick" figures around the pictogram (#657). Lifetime
	// km and the streak load with `progress`, slightly after the rest of the
	// page — the "–" placeholder covers that gap instead of a layout jump.
	let lastRide = $derived(activities[0] ?? null);

	function formatLastRide(a: ActivitySummary): string {
		const date = new Date(a.started_at).toLocaleDateString('de-DE', {
			weekday: 'short',
			day: '2-digit',
			month: '2-digit'
		});
		if (a.distance_meters == null) return date;
		return `${date} · ${(a.distance_meters / 1000).toFixed(1)} km`;
	}

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
		return series;
	});

	onMount(async () => {
		try {
			const [trainingLoad, fetchedActivities] = await Promise.all([
				getTrainingLoad(),
				listActivities()
			]);
			activities = fetchedActivities;
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
					activePill = metricKeyFor(settings.primary_metric);
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

	// Frische (TSB) as a position on the zone ramp (Nocturne v3, direction B
	// §1.4/D4) — the raw signed number means nothing to a hobbyist without
	// training background, same reasoning the ride-detail IF Skala-Band was
	// built for. −30…+25 is the range the app's own training-status backend
	// treats as the normal band (trainingstatus.go), clamped at the edges
	// rather than cut off.
	let tsbShare = $derived(latestTSB === null ? 0 : Math.max(0, Math.min(1, (latestTSB + 30) / 55)));
	let tsbLabel = $derived(
		latestTSB === null ? '–' : `${latestTSB > 0 ? '+' : ''}${Math.round(latestTSB)}`
	);

	// gauge.label bakes in "Trainingsniveau: " itself (#611) — fine for the
	// old tile, which had no label of its own, but redundant now that this
	// sits in a Fact Band with its own external "Trainingsniveau" label
	// (Nocturne v3): stripped down to just the category name so the value
	// isn't "Trainingsniveau: Einstieg" wrapping under its own heading twice.
	let gaugeCategory = $derived.by(() => status?.gauge?.label?.split(': ').pop() ?? '–');

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
	<!-- The dashboard's own shape as its header graphic (Nocturne v3, direction
	     B) — the same treatment the ride-detail page gives a ride's elevation,
	     replacing a decorative bicycle with the one chart that answers "what
	     is this page even about" before any number does. -->
	<div class="bleed">
		<LineChart
			xValues={dayTimestamps}
			series={[{ name: 'Fitness', color: 'var(--chart-ctl)', values: series.map((d) => d.ctl) }]}
			xFormat={formatDay}
			yFormat={(y) => y.toFixed(1)}
			ariaLabel="Fitness-Verlauf"
			height={64}
			bare
		/>
	</div>

	<!-- The answer first: how you are and whether you're getting better. The
	     chart below is the evidence, not the message (#602). -->
	<StoryHero
		story={status}
		fallbackTitle="Dein Trainingsstand"
		meterColor="var(--chart-ctl)"
		showGauge={false}
	/>

	<!-- Frische (TSB) as a position, not just a signed number (D4) — the one
	     figure in the hero above a hobbyist can't read cold. -->
	<p class="fact-label">Frische (TSB)</p>
	<div class="fact-scale" style="--fact-share: {tsbShare}">
		<div class="fact-scale-marker"></div>
	</div>
	<p class="fact-scale-ends"><span>ausgelaugt</span><span>{tsbLabel}</span><span>frisch</span></p>

	<StoryCards
		statements={status?.statements ?? []}
		label="Dein aktueller Trainingsstand"
		context={{
			form: {
				xValues: dayTimestamps,
				series: [{ name: 'Frische', color: tsbColor, values: series.map((d) => d.tsb) }],
				xFormat: formatDay,
				yFormat: (y) => (Math.abs(y) < 0.05 ? '0' : y.toFixed(1)),
				caption: 'Verlauf der letzten Wochen'
			},
			trend: {
				xValues: dayTimestamps,
				series: [{ name: 'Fitness', color: 'var(--chart-ctl)', values: series.map((d) => d.ctl) }],
				xFormat: formatDay,
				yFormat: (y) => y.toFixed(1),
				caption: 'Verlauf der letzten Wochen'
			}
		}}
	/>

	<!-- auf einen Blick (#657): numbers only now (D5) — the training level
	     left this grid for its own filled band below, so every tile here is a
	     figure, not a mix of figures and two-line text blobs. -->
	<section class="glance" aria-label="Auf einen Blick">
		<div class="fact-tile" style="--i: 0">
			<p class="fact-tile-value">
				{progress ? Math.round(progress.lifetime_distance_meters / 1000) : '–'}
				<span class="fact-tile-unit">km</span>
			</p>
			<p class="fact-tile-label">Insgesamt gefahren</p>
		</div>
		<div class="fact-tile" style="--i: 1">
			<p class="fact-tile-value">
				{progress ? progress.current_streak_weeks : '–'}
				<span class="fact-tile-unit">Wochen</span>
			</p>
			<p class="fact-tile-label">Aktuelle Streak</p>
		</div>
		{#if lastRide}
			<a
				class="fact-tile fact-tile-link"
				style="--i: 2"
				href={resolve('/(app)/rides/[id]', { id: String(lastRide.id) })}
			>
				<p class="fact-tile-value fact-tile-value-text">{formatLastRide(lastRide)}</p>
				<p class="fact-tile-label">Letzte Fahrt</p>
			</a>
		{/if}
	</section>

	<!-- Trainingsniveau leaves the tile grid for a full filled band (Nocturne
	     v3) — status.gauge.percent is computed server-side and used to render
	     nowhere on this page before now (D4). Label above, meaning below: the
	     hard AA rule for anything with a .fact-fill (see app.css). -->
	<p class="fact-label">Trainingsniveau</p>
	<div class="fact">
		<div class="fact-fill" style="--fact-share: {(status?.gauge?.percent ?? 0) / 100}"></div>
		<p class="fact-value">{gaugeCategory}</p>
	</div>
	{#if status?.gauge?.caption}
		<p class="fact-meaning">{status.gauge.caption}</p>
	{/if}

	<p class="year-review-link">
		<a href={resolve('/(app)/rueckblick')}>Dein Jahresrückblick ansehen →</a>
	</p>

	<section class="panel">
		<h2>Wie sich das entwickelt hat</h2>
		<p class="panel-sub">
			Fitness wächst langsam über Wochen, Müdigkeit steigt und fällt mit den letzten Tagen. Wo die
			Frische-Linie über null liegt, bist du erholt.
		</p>
		<div class="bleed">
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
				height={260}
				baseline={0}
			/>
		</div>
	</section>

	<section class="panel">
		<h2>Was du jetzt tun kannst</h2>
		<p class="panel-sub">
			Aus deinem Trainingsverlauf abgeleitet — mit Begründung, damit du es nachvollziehen kannst.
		</p>
		<div class="insights">
			{#each insights as insight, i (i)}
				<div class="insight insight-{insight.severity}" style="--i: {i}">
					<span class="insight-icon">{severityIcon[insight.severity]}</span>
					<div>
						{#if insight.action}<p class="insight-action">{insight.action}</p>{/if}
						<p class="insight-reason">{insight.reason}</p>
					</div>
				</div>
			{/each}
		</div>
	</section>

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

	<!-- Merged (Nocturne v3, direction B): what were three sections — a
	     weekly-metric panel, "Arbeitet dein Herz effizienter?" and "Wird
	     deine Leistung besser?" — are one chart frame and one pill row now.
	     The old version rendered heading + paragraph + placeholder twice
	     whenever endurance or power data was thin; folding them together
	     means every pill always lands in a frame that already has a chart or
	     an honest one-line empty state, never a lonely explanation. -->
	{#if progress}
		<section class="panel">
			<h2>Wirst du besser?</h2>
			<div class="metric-switch" role="group" aria-label="Kennzahl wählen">
				{#each progressMetrics as m (m.key)}
					<button
						type="button"
						class="btn {activePill === m.key ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={activePill === m.key}
						onclick={() => (activePill = m.key)}
					>
						{m.label}
					</button>
				{/each}
				{#if progress.endurance.statements.length > 0}
					<button
						type="button"
						class="btn {activePill === 'endurance' ? 'btn-primary' : 'btn-secondary'}"
						aria-pressed={activePill === 'endurance'}
						onclick={() => (activePill = 'endurance')}
					>
						Puls-Effizienz
					</button>
				{/if}
				<button
					type="button"
					class="btn {activePill === 'power' ? 'btn-primary' : 'btn-secondary'}"
					aria-pressed={activePill === 'power'}
					onclick={() => (activePill = 'power')}
				>
					Leistung
				</button>
			</div>

			{#if activePill === 'power'}
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
			{/if}

			<div class="bleed">
				{#if activeProgressMetric}
					<LineChart
						xValues={progress.weeks.map((w) => new Date(w.start).getTime())}
						series={[
							{
								name: activeProgressMetric.label,
								color: activeProgressMetric.color,
								values: progress.weeks.map((w) => w[activeProgressMetric!.key])
							}
						]}
						xFormat={formatDay}
						yFormat={activeProgressMetric.format}
						ariaLabel="Wochenverlauf {activeProgressMetric.label}"
						height={220}
					/>
				{:else if activePill === 'endurance'}
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
							height={220}
						/>
					{:else}
						<p class="empty">Noch nicht genug vergleichbare Fahrten für diesen Verlauf.</p>
					{/if}
				{:else if activePill === 'power'}
					{#if powerCurveHistory.length > 0}
						<LineChart
							xValues={powerCurveTimestamps}
							series={powerCurveSeries}
							xFormat={formatDay}
							yFormat={(y) => `${Math.round(y)} W`}
							ariaLabel="Verlauf der besten Leistung"
							height={220}
						/>
					{:else}
						<p class="empty">
							Noch keine Fahrt mit Leistungsdaten, die lang genug für diese Dauer war.
						</p>
					{/if}
				{/if}
			</div>

			<p class="panel-caption">
				{#if activeProgressMetric}
					Woche für Woche statt Fahrt für Fahrt — einzelne Ausfahrten sagen zu wenig.
				{:else if activePill === 'endurance'}
					{progress.endurance.from_power
						? 'Steigt die Linie, leistet dein Herz für dieselben Watt weniger Arbeit.'
						: 'Steigt die Linie, leistet dein Herz für dasselbe Tempo weniger Arbeit.'}
					({progress.endurance.unit})
				{:else if activePill === 'power'}
					Deine beste Leistung für diese Dauer, Fahrt für Fahrt.
					{#if powerCurveDuration === 1200 && configuredFtpWatts !== null}
						· Deine hinterlegte FTP: {configuredFtpWatts} W
					{/if}
				{/if}
			</p>
		</section>

		{#if activeProgressMetric}
			<StoryCards statements={progress.statements} label="Fortschritt über die Wochen" />
		{:else if activePill === 'endurance'}
			<StoryCards statements={progress.endurance.statements} label="Ausdauer über die Wochen" />
		{/if}
	{/if}

	<p class="glossary-hint">
		Fitness, Müdigkeit, Frische — was dahinter steckt, steht im
		<a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
{/if}

<style>
	/* Numbers only now (Nocturne v3, D5) — the bike and the training-level
	   text tile both left this grid, so it's a plain 2-col Fact Tile grid
	   directly, no wrapping flex column needed any more. */
	.glance {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem;
		max-width: 26rem;
		margin-bottom: 3rem;
	}

	.fact-tile-link {
		display: block;
		color: inherit;
		text-decoration: none;
		transition: transform var(--dur-fast) ease-out;
	}

	.fact-tile-link:active {
		transform: scale(0.985);
	}

	.fact-tile-value-text {
		font-size: var(--text-lg);
		font-variant-numeric: normal;
	}

	.year-review-link {
		margin: 0 0 1.5rem;
		font-size: var(--text-sm);
	}

	/* Bare section — heading, sub, content, separated by real space instead
	   of a bordered/shadowed box (Nocturne v2). */
	.panel {
		margin-bottom: 3rem;
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

	/* One line under the merged section's chart, its content swapping with
	   the active pill (Nocturne v3) — replaces what used to be a full
	   .panel-sub paragraph *before* the chart on three separate panels. */
	.panel-caption {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0.5rem 0 0;
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
		/* Opacity only, no translate (Nocturne v3) — these are the page's only
		   paragraphs that matter; sliding prose is the most template-looking
		   effect available, so it stays to a plain fade. */
		transition: opacity var(--dur-base) var(--ease-out-soft);
		transition-delay: calc(min(var(--i, 0), 5) * 60ms);
	}

	@starting-style {
		.insight {
			opacity: 0;
		}
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

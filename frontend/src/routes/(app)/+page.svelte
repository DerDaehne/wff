<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { listActivities, formatDecimal, type ActivitySummary, type RideStory } from '$lib/rides';
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
	import StoryCards from '$lib/components/StoryCards.svelte';
	import ZoneBars from '$lib/components/ZoneBars.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { pivotWidths } from '$lib/pivot';

	// 'no-activities' (upload something) and 'no-training-load' (activities
	// exist, but none has a computed TSS — almost always missing FTP/LTHR in
	// the profile, not actually "nothing uploaded") are deliberately distinct:
	// they need different, accurate messages.
	let viewState: 'loading' | 'no-activities' | 'no-training-load' | 'error' | 'ready' =
		$state('loading');
	let series: DayLoad[] = $state([]);
	let insights: Insight[] = $state([]);
	let status = $state<RideStory | null>(null);
	let errorMessage = $state('');
	let progress = $state<Progress | null>(null);
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

	function formatRideDay(a: ActivitySummary): string {
		return new Date(a.started_at).toLocaleDateString('de-DE', {
			weekday: 'short',
			day: '2-digit',
			month: '2-digit'
		});
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
		const series: { name: string; values: number[]; description?: string }[] = [
			{
				name: 'Leistung',
				values: powerCurveHistory.map((p) => p.watts)
			}
		];
		if (powerCurveDuration !== 1200) return series;
		series.push({
			name: 'Geschätzte FTP (95 %)',
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

	// Frische (TSB) as a position on the zone ramp (Nocturne v3, direction B
	// §1.4/D4) — the raw signed number means nothing to a hobbyist without
	// training background, same reasoning the ride-detail IF Skala-Band was
	// built for. −30…+25 is the range the app's own training-status backend
	// treats as the normal band (trainingstatus.go), clamped at the edges
	// rather than cut off.
	let tsbShare = $derived(latestTSB === null ? 0 : Math.max(0, Math.min(1, (latestTSB + 30) / 55)));

	// gauge.label bakes in "Trainingsniveau: " itself (#611) — fine for the
	// old tile, which had no label of its own, but redundant now that this
	// sits in a Fact Band with its own external "Trainingsniveau" label
	// (Nocturne v3): stripped down to just the category name so the value
	// isn't "Trainingsniveau: Einstieg" wrapping under its own heading twice.
	let gaugeCategory = $derived.by(() => status?.gauge?.label?.split(': ').pop() ?? '–');

	// An insight's severity is one of Rekta's statuses already; the tag says it
	// in a word too, so the square's colour is never the only signal.
	const severityWord: Record<Insight['severity'], string> = {
		success: 'passt',
		warning: 'achtung',
		info: 'tipp'
	};

	// status.stats come in the order analyze/trainingstatus.go writes them:
	// Fitness, Müdigkeit, Frische.
	let fitness = $derived(status?.stats[0]);
	let fatigue = $derived(status?.stats[1]);
	let freshness = $derived(status?.stats[2]);

	// Fitness over the weeks as the tile's sparkline (Rekta: a live tile carries
	// its recent trend), in the 200 × 28 box Rekta draws sparklines in.
	let ctlSpark = $derived.by(() => {
		const v = series.map((d) => d.ctl);
		if (v.length < 2) return '';
		const lo = Math.min(...v);
		const span = Math.max(...v) - lo || 1;
		return v
			.map((y, i) => `${(i / (v.length - 1)) * 200},${26 - ((y - lo) / span) * 24}`)
			.join(' ');
	});

	// "Wirst du besser?" picks its figure from a facet pivot (Rekta: the facets
	// of one thing), ← → Home End move between them.
	let pills = $derived([
		...progressMetrics.map((m) => ({ key: m.key as PillKey, label: m.label })),
		...(progress && progress.endurance.statements.length > 0
			? [{ key: 'endurance' as PillKey, label: 'Puls-Effizienz' }]
			: []),
		{ key: 'power' as PillKey, label: 'Leistung' }
	]);

	function choose(key: PillKey, tab: HTMLElement) {
		activePill = key;
		tab.focus();
		tab.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	}

	function facetKeys(e: KeyboardEvent & { currentTarget: HTMLElement }) {
		const tabs = [...e.currentTarget.children] as HTMLElement[];
		const at = pills.findIndex((p) => p.key === activePill);
		const next =
			e.key === 'ArrowRight'
				? Math.min(at + 1, pills.length - 1)
				: e.key === 'ArrowLeft'
					? Math.max(at - 1, 0)
					: e.key === 'Home'
						? 0
						: e.key === 'End'
							? pills.length - 1
							: null;
		if (next === null) return;
		e.preventDefault();
		choose(pills[next].key, tabs[next]);
	}
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
	<!-- The answer first (#602): what the training says is the one title of
	     this place; the tiles below are the figures behind it. -->
	<header class="head reveal" style="--i: 0">
		<p class="rk-kicker">Dein Trainingsstand</p>
		<h1 class="rk-title">{status?.title || 'Dein Trainingsstand'}</h1>
	</header>

	<!-- Rekta live tiles, sized by what they hold (#657 "auf einen Blick"). -->
	<section class="rk-tiles glance" aria-label="Auf einen Blick">
		{#if fitness}
			<div class="rk-tile rk-w2" style="--rk-i: 0">
				<span class="rk-stat"><b>{fitness.value}</b><span>{fitness.label}</span></span>
				{#if ctlSpark}
					<svg class="rk-spark" viewBox="0 0 200 28" aria-hidden="true">
						<polyline pathLength="1" points={ctlSpark} />
					</svg>
				{/if}
			</div>
		{/if}
		{#if fatigue}
			<div class="rk-tile" style="--rk-i: 1">
				<span class="rk-stat"><b>{fatigue.value}</b><span>{fatigue.label}</span></span>
			</div>
		{/if}
		<div class="rk-tile" style="--rk-i: 2">
			<span class="rk-stat">
				<b
					>{progress ? Math.round(progress.lifetime_distance_meters / 1000) : '–'}<small>km</small
					></b
				>
				<span>insgesamt gefahren</span>
			</span>
		</div>
		{#if freshness}
			<!-- Frische as a position, not just a signed number (D4): the one figure
			     a hobbyist can't read cold. -->
			<div class="rk-tile rk-w2" style="--rk-i: 3">
				<span class="rk-stat"><b>{freshness.value}</b><span>{freshness.label}</span></span>
				<div class="fact-scale" style="--fact-share: {tsbShare}">
					<div class="fact-scale-marker"></div>
				</div>
				<p class="fact-scale-ends"><span>ausgelaugt</span><span>frisch</span></p>
			</div>
		{/if}
		{#if status?.gauge}
			<div class="rk-tile rk-w2" style="--rk-i: 4">
				<span class="rk-stat"><b>{gaugeCategory}</b><span>Trainingsniveau</span></span>
				<progress
					class="rk-progress"
					max="100"
					value={status.gauge.percent}
					aria-label="Trainingsniveau"
				></progress>
				{#if status.gauge.caption}<span class="rk-tile-msg">{status.gauge.caption}</span>{/if}
			</div>
		{/if}
		<div class="rk-tile" style="--rk-i: 5">
			<span class="rk-stat">
				<b>{progress ? progress.current_streak_weeks : '–'}<small>Wochen</small></b>
				<span>am Stück gefahren</span>
			</span>
		</div>
		{#if lastRide}
			<a
				class="rk-tile rk-w2"
				style="--rk-i: 6"
				href={resolve('/(app)/rides/[id]', { id: String(lastRide.id) })}
			>
				<span class="rk-kicker">letzte Fahrt</span>
				<span class="rk-stat">
					{#if lastRide.distance_meters != null}
						<b>{formatDecimal(lastRide.distance_meters / 1000, 1)}<small>km</small></b>
					{/if}
					<span>{formatRideDay(lastRide)}</span>
				</span>
			</a>
		{/if}
		<a class="rk-tile" style="--rk-i: 7" href={resolve('/(app)/rueckblick')}>
			<span class="rk-kicker">dein Jahr</span>
			<span class="rk-stat"><b>{new Date().getFullYear()}</b><span>Jahresrückblick</span></span>
		</a>
	</section>

	<StoryCards
		statements={status?.statements ?? []}
		label="Dein aktueller Trainingsstand"
		context={{
			form: {
				xValues: dayTimestamps,
				series: [{ name: 'Frische', values: series.map((d) => d.tsb) }],
				xFormat: formatDay,
				yFormat: (y) => (Math.abs(y) < 0.05 ? '0' : formatDecimal(y, 1)),
				caption: 'Verlauf der letzten Wochen'
			},
			trend: {
				xValues: dayTimestamps,
				series: [{ name: 'Fitness', values: series.map((d) => d.ctl) }],
				xFormat: formatDay,
				yFormat: (y) => formatDecimal(y, 1),
				caption: 'Verlauf der letzten Wochen'
			}
		}}
	/>

	<section class="sec reveal" style="--i: 1">
		<h2 class="rk-group">Wie sich das entwickelt hat</h2>
		<p class="sub">
			Fitness wächst langsam über Wochen, Müdigkeit steigt und fällt mit den letzten Tagen. Wo die
			Frische-Linie über null liegt, bist du erholt.
		</p>
		<LineChart
			xValues={dayTimestamps}
			series={[
				{
					name: 'Fitness',
					values: series.map((d) => d.ctl),
					description:
						'Fachbegriff CTL: dein Trainingsumfang der letzten 6 Wochen. Steigt langsam, fällt langsam — das ist deine Grundlage.'
				},
				{
					name: 'Müdigkeit',
					values: series.map((d) => d.atl),
					description:
						'Fachbegriff ATL: die Belastung der letzten Woche. Reagiert schnell auf einzelne harte Fahrten.'
				},
				{
					name: 'Frische',
					values: series.map((d) => d.tsb),
					description:
						'Fachbegriff TSB: Fitness minus Müdigkeit. Über null bist du erholt, unter null steckst du in einer Belastungsphase.'
				}
			]}
			xFormat={formatDay}
			yFormat={(y) => (Math.abs(y) < 0.05 ? '0' : formatDecimal(y, 1))}
			ariaLabel="Verlauf von Fitness, Müdigkeit und Frische"
			height={260}
			baseline={0}
		/>
	</section>

	{#if insights.length > 0}
		<section class="sec reveal" style="--i: 2">
			<h2 class="rk-group">Was du jetzt tun kannst</h2>
			<p class="sub">
				Aus deinem Trainingsverlauf abgeleitet — mit Begründung, damit du es nachvollziehen kannst.
			</p>
			<ul class="rk-rows insights">
				{#each insights as insight, i (i)}
					<li>
						<div>
							{#if insight.action}<p class="insight-action">{insight.action}</p>{/if}
							<p class="rk-facts">{insight.reason}</p>
						</div>
						<span class="rk-tag" data-status={insight.severity}>
							{severityWord[insight.severity]}
						</span>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- Only when there is a distribution to show: without a threshold pulse
	     the heading would introduce nothing, and the hint tile says why. -->
	{#if progress && progress.zones.total_seconds > 0}
		<section class="sec reveal" style="--i: 3">
			<h2 class="rk-group">Wie hart fährst du eigentlich?</h2>
			<p class="sub">
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

	<!-- One frame for every "am I getting better" figure (Nocturne v3): the
	     facet pivot picks which one the chart shows. -->
	{#if progress}
		<section class="sec reveal" style="--i: 4">
			<h2 class="rk-group">Wirst du besser?</h2>
			<div
				class="rk-pivot facets"
				role="tablist"
				aria-label="Kennzahl wählen"
				tabindex="-1"
				use:pivotWidths
				onkeydown={facetKeys}
			>
				{#each pills as p (p.key)}
					<button
						type="button"
						role="tab"
						aria-selected={activePill === p.key}
						tabindex={activePill === p.key ? 0 : -1}
						onclick={(e) => choose(p.key, e.currentTarget)}
					>
						{p.label}
					</button>
				{/each}
			</div>

			{#if activePill === 'power'}
				<fieldset class="rk-segments durations">
					<legend class="rk-sr">Dauer wählen</legend>
					{#each powerCurveDurations as d (d.seconds)}
						<label>
							<input
								type="radio"
								name="power-duration"
								value={d.seconds}
								bind:group={powerCurveDuration}
							/>
							{d.label}
						</label>
					{/each}
				</fieldset>
			{/if}

			<div role="tabpanel">
				{#if activeProgressMetric}
					<LineChart
						xValues={progress.weeks.map((w) => new Date(w.start).getTime())}
						series={[
							{
								name: activeProgressMetric.label,
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
									values: progress.endurance.weeks.map((w) => w.value)
								}
							]}
							xFormat={formatDay}
							yFormat={(v: number) =>
								progress?.endurance.from_power ? String(Math.round(v)) : formatDecimal(v, 1)}
							ariaLabel="Wochenverlauf {progress.endurance.unit}"
							height={220}
						/>
					{:else}
						<p class="rk-empty">Noch nicht genug vergleichbare Fahrten für diesen Verlauf.</p>
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
						<p class="rk-empty">
							Noch keine Fahrt mit Leistungsdaten, die lang genug für diese Dauer war.
						</p>
					{/if}
				{/if}
			</div>

			<p class="rk-facts caption">
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

	<p class="rk-facts glossary-hint">
		Fitness, Müdigkeit, Frische — was dahinter steckt, steht im
		<a href={resolve('/(app)/glossar')}>Glossar</a>.
	</p>
{/if}

<style>
	.head {
		margin-bottom: var(--rk-space-l);
	}

	.head .rk-title {
		margin-top: var(--rk-space-xs);
	}

	.glance {
		margin-bottom: var(--rk-space-s);
	}

	/* Frische's position on a tile: the track in the tile's own text colour,
	   as Rekta draws progress on a tile. */
	.glance .fact-scale {
		height: 4px;
		background: color-mix(in srgb, currentColor 20%, transparent);
	}

	.glance .fact-scale-marker {
		box-shadow: 0 0 0 2px var(--rk-surface);
	}

	.glance .fact-scale-ends {
		margin: 0;
	}

	.sec {
		margin-top: var(--rk-space-xl);
	}

	.sub {
		max-width: 60ch;
		margin: var(--rk-space-xs) 0 var(--rk-space-m);
		color: var(--rk-text-contrast-high);
	}

	.insights > li {
		flex-wrap: nowrap;
		align-items: flex-start;
	}

	.insight-action {
		font-weight: var(--rk-font-weight-semibold);
	}

	.facets,
	.durations {
		margin-bottom: var(--rk-space-m);
	}

	.caption {
		margin-top: var(--rk-space-s);
	}

	.glossary-hint {
		margin-top: var(--rk-space-xl);
	}
</style>

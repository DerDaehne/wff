<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { listActivities } from '$lib/rides';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';

	// 'no-activities' (upload something) and 'no-training-load' (activities
	// exist, but none has a computed TSS — almost always missing FTP/LTHR in
	// the profile, not actually "nothing uploaded") are deliberately distinct:
	// they need different, accurate messages.
	let viewState: 'loading' | 'no-activities' | 'no-training-load' | 'error' | 'ready' =
		$state('loading');
	let series: DayLoad[] = $state([]);
	let insights: Insight[] = $state([]);
	let errorMessage = $state('');

	onMount(async () => {
		try {
			const [trainingLoad, activities] = await Promise.all([getTrainingLoad(), listActivities()]);
			series = trainingLoad.series;
			insights = trainingLoad.insights;
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

<h1>Dashboard</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'no-activities'}
	<p>Noch keine Aktivitäten hochgeladen.</p>
	<p><a href={resolve('/(app)/upload')}>Erste Fahrt hochladen</a></p>
{:else if viewState === 'no-training-load'}
	<p>
		Fahrten sind hochgeladen, aber es fehlt eine FTP- oder Puls-Schwellenwert-Konfiguration, um
		daraus eine Trainingsbelastung zu berechnen.
	</p>
	<p><a href={resolve('/(app)/profile')}>FTP/LTHR im Profil hinterlegen</a></p>
{:else if viewState === 'ready'}
	<p class="metric-hint">
		<strong>CTL</strong> (Fitness) und <strong>ATL</strong> (Ermüdung) sind gleitende Mittelwerte
		deiner täglichen Trainingsbelastung (42 bzw. 7 Tage) — <strong>TSB</strong> (Form) ist die
		Differenz aus beiden: positiv heißt erholt/frisch, negativ heißt du trainierst gerade hart.
	</p>
	<p>
		Aktuelle Form (TSB): <strong style="color: {tsbColor}">{latestTSB?.toFixed(1)}</strong>
	</p>
	<LineChart
		xValues={dayTimestamps}
		series={[
			{
				name: 'CTL (Fitness)',
				color: 'var(--chart-ctl)',
				values: series.map((d) => d.ctl),
				description: 'Langzeit-Trainingsbelastung (42-Tage-Mittel) — deine allgemeine Fitness.'
			},
			{
				name: 'ATL (Ermüdung)',
				color: 'var(--chart-atl)',
				values: series.map((d) => d.atl),
				description: 'Kurzzeit-Trainingsbelastung (7-Tage-Mittel) — wie erschöpft du gerade bist.'
			},
			{
				name: 'TSB (Form)',
				color: tsbColor,
				values: series.map((d) => d.tsb),
				description: 'CTL minus ATL — positiv: erholt, negativ: du trainierst gerade hart.'
			}
		]}
		xFormat={formatDay}
		yFormat={(y) => (Math.abs(y) < 0.05 ? '0' : y.toFixed(1))}
		ariaLabel="CTL/ATL/TSB-Verlauf"
	/>

	<h2>Insights</h2>
	<p class="metric-hint">Automatische Einschätzung deines Trainingsverlaufs, regelbasiert aus CTL/ATL/TSB.</p>
	<div class="insights">
		{#each insights as insight, i (i)}
			<div class="insight insight-{insight.severity}">
				<span class="insight-icon">{severityIcon[insight.severity]}</span>
				<span>{insight.message}</span>
			</div>
		{/each}
	</div>
{/if}

<style>
	.metric-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		max-width: 60ch;
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
		border-radius: 12px;
		padding: 0.875rem 1rem;
		box-shadow: var(--shadow-sm);
	}

	.insight-icon {
		font-weight: 700;
		flex-shrink: 0;
	}

	.insight-success {
		background: color-mix(in srgb, var(--color-success) 10%, var(--color-surface));
		color: color-mix(in srgb, var(--color-success) 70%, var(--color-text));
	}

	.insight-warning {
		background: color-mix(in srgb, var(--color-warning) 10%, var(--color-surface));
		color: color-mix(in srgb, var(--color-warning) 70%, var(--color-text));
	}

	.insight-info {
		background: color-mix(in srgb, var(--color-info) 8%, var(--color-surface));
		color: var(--color-text);
	}
</style>

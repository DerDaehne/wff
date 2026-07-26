<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { ApiError } from '$lib/api';
	import LineChart from '$lib/components/LineChart.svelte';

	let viewState: 'loading' | 'empty' | 'error' | 'ready' = $state('loading');
	let series: DayLoad[] = $state([]);
	let insights: Insight[] = $state([]);
	let errorMessage = $state('');

	onMount(async () => {
		try {
			const data = await getTrainingLoad();
			series = data.series;
			insights = data.insights;
			viewState = series.length === 0 ? 'empty' : 'ready';
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
</script>

<h1>Dashboard</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'empty'}
	<p>Noch keine Aktivitäten hochgeladen.</p>
	<p><a href={resolve('/(app)/upload')}>Erste Fahrt hochladen</a></p>
{:else if viewState === 'ready'}
	<p>
		Aktuelle Form (TSB): <strong style="color: {tsbColor}">{latestTSB?.toFixed(1)}</strong>
	</p>
	<LineChart
		xValues={dayTimestamps}
		series={[
			{ name: 'CTL (Fitness)', color: 'var(--chart-ctl)', values: series.map((d) => d.ctl) },
			{ name: 'ATL (Ermüdung)', color: 'var(--chart-atl)', values: series.map((d) => d.atl) },
			{ name: 'TSB (Form)', color: tsbColor, values: series.map((d) => d.tsb) }
		]}
		xFormat={formatDay}
		yFormat={(y) => y.toFixed(0)}
		ariaLabel="CTL/ATL/TSB-Verlauf"
	/>

	<h2>Insights</h2>
	<ul>
		{#each insights as insight, i (i)}
			<li>{insight.message}</li>
		{/each}
	</ul>
{/if}

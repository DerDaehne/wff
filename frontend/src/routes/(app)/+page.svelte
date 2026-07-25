<script lang="ts">
	import { onMount } from 'svelte';
	import { getTrainingLoad, type DayLoad, type Insight } from '$lib/trainingload';
	import { ApiError } from '$lib/api';

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
			errorMessage = err instanceof ApiError ? err.message : 'Trainingsdaten konnten nicht geladen werden.';
			viewState = 'error';
		}
	});

	const width = 800;
	const height = 220;
	const pad = 24;

	let chart = $derived.by(() => {
		if (series.length === 0) return null;
		const values = series.flatMap((d) => [d.ctl, d.atl, d.tsb]);
		const min = Math.min(0, ...values);
		const max = Math.max(1, ...values);
		const xStep = (width - 2 * pad) / Math.max(series.length - 1, 1);
		const x = (i: number) => pad + i * xStep;
		const y = (v: number) => height - pad - ((v - min) / (max - min)) * (height - 2 * pad);
		const zeroY = y(0);
		const line = (key: 'ctl' | 'atl' | 'tsb') =>
			series.map((d, i) => `${x(i)},${y(d[key])}`).join(' ');
		return {
			ctlPoints: line('ctl'),
			atlPoints: line('atl'),
			tsbPoints: line('tsb'),
			zeroY
		};
	});

	let latestTSB = $derived(series.length > 0 ? series[series.length - 1].tsb : null);
	let tsbColor = $derived(
		latestTSB === null ? '#64748b' : latestTSB < -10 ? '#dc2626' : latestTSB > 5 ? '#16a34a' : '#64748b'
	);
</script>

<h1>Dashboard</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else if viewState === 'empty'}
	<p>Noch keine Aktivitäten hochgeladen.</p>
	<p><a href="/upload">Erste Fahrt hochladen</a></p>
{:else if chart}
	<p>
		Aktuelle Form (TSB): <strong style="color: {tsbColor}">{latestTSB?.toFixed(1)}</strong>
	</p>
	<svg viewBox="0 0 {width} {height}" role="img" aria-label="CTL/ATL/TSB-Verlauf">
		<line x1={pad} y1={chart.zeroY} x2={width - pad} y2={chart.zeroY} stroke="#e5e7eb" />
		<polyline points={chart.ctlPoints} fill="none" stroke="#2563eb" stroke-width="2" />
		<polyline points={chart.atlPoints} fill="none" stroke="#f59e0b" stroke-width="2" />
		<polyline points={chart.tsbPoints} fill="none" stroke={tsbColor} stroke-width="2" />
	</svg>
	<ul class="legend">
		<li><span class="swatch" style="background: #2563eb"></span> CTL (Fitness)</li>
		<li><span class="swatch" style="background: #f59e0b"></span> ATL (Ermüdung)</li>
		<li><span class="swatch" style="background: {tsbColor}"></span> TSB (Form)</li>
	</ul>

	<h2>Insights</h2>
	<ul>
		{#each insights as insight, i (i)}
			<li>{insight.message}</li>
		{/each}
	</ul>
{/if}

<style>
	svg {
		width: 100%;
		height: auto;
		max-width: 800px;
	}

	.legend {
		display: flex;
		gap: 1rem;
		list-style: none;
		padding: 0;
		font-size: 0.875rem;
	}

	.swatch {
		display: inline-block;
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 50%;
		margin-right: 0.25rem;
	}
</style>

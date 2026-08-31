<script lang="ts">
	import { resolve } from '$app/paths';
	import type { RideStatement } from '$lib/rides';
	import BottomSheet from './BottomSheet.svelte';
	import LineChart from './LineChart.svelte';

	// A small chart to show inside a kind's detail sheet, when real history for
	// that exact figure already lives on the page — e.g. the dashboard's
	// Frische sparkline reuses the same `series` the main chart already
	// fetched. Deliberately keyed by kind, not by statement: a sheet only
	// shows one group's worth of items at a time, all sharing a kind, so one
	// context per kind is enough. Absent for kinds with no on-page history to
	// draw from (a single ride has no history of its own) — see
	// design-wff-nocturne-v3, "what needs backend work".
	interface SparkContext {
		xValues: number[];
		series: { name: string; color: string; values: (number | null)[] }[];
		xFormat: (x: number) => string;
		yFormat?: (y: number) => string;
		caption: string;
	}

	let {
		statements,
		label,
		context = {}
	}: {
		statements: RideStatement[];
		label: string;
		context?: Partial<Record<RideStatement['kind'], SparkContext>>;
	} = $props();

	// Every statement leads with what it MEANS; the chip says which question it
	// answers, so nobody has to infer that from the sentence itself.
	const statementLabel: Record<RideStatement['kind'], string> = {
		effort: 'Wie hart war es',
		load: 'Was es gebracht hat',
		pace: 'Wie schnell du warst',
		climb: 'Wie gut du geklettert bist',
		endurance: 'Ob es bis zum Schluss trug',
		context: 'Warum es sich so anfühlte',
		comparison: 'Im Vergleich zu sonst',
		form: 'Wie es dir gerade geht',
		trend: 'Ob du besser wirst',
		endurance_trend: 'Ob deine Ausdauer wächst',
		zones: 'Wie du die Zeit verteilt hast',
		calories: 'Was es an Energie gekostet hat',
		milestone: 'Persönlicher Rekord',
		hint_profile: 'Dafür fehlt noch etwas',
		hint_history: 'Dafür fehlt noch etwas'
	};

	// Consecutive statements of the same kind share one card — three separate
	// cards all headed "Warum es sich so anfühlte" (wind, hills, heat) read as
	// a repetition bug rather than as three reasons for the same thing.
	let groups = $derived.by(() => {
		const out: { kind: RideStatement['kind']; items: RideStatement[] }[] = [];
		for (const statement of statements) {
			const open = out.at(-1);
			if (open?.kind === statement.kind) open.items.push(statement);
			else out.push({ kind: statement.kind, items: [statement] });
		}
		return out;
	});

	// The compact row's second line — every item's metric joined, so a
	// "context" group (wind + hills + heat) still reads as one line. Groups
	// that carry no metric at all (hint_profile/hint_history, and comparison)
	// have nothing numeric to lead with — the chip alone is the row, full
	// text is one tap away either way.
	function summaryOf(group: (typeof groups)[number]): string | null {
		const metrics = group.items.map((s) => s.metric).filter((m): m is string => !!m);
		return metrics.length > 0 ? metrics.join(' · ') : null;
	}

	// The structured figures behind summaryOf's text, when the backend could
	// reduce the statement to one or two clear values (#651) — shown as big
	// numbers instead of a text fragment. Statements without any (a four-way
	// zone split, say) leave this empty and fall back to summaryOf's text.
	function metricsOf(group: (typeof groups)[number]) {
		return group.items.flatMap((s) => s.metrics ?? []);
	}

	let activeGroup: (typeof groups)[number] | null = $state(null);

	function closeDetail() {
		activeGroup = null;
	}
</script>

{#snippet row(group: (typeof groups)[number], i: number)}
	<button
		type="button"
		class="statement-row statement-{group.kind}"
		style="--i: {i}"
		onclick={() => (activeGroup = group)}
	>
		<span class="row-main">
			<span class="chip statement-chip">{statementLabel[group.kind]}</span>
			{#if metricsOf(group).length > 0}
				<span class="row-values">
					{#each metricsOf(group) as m, i (i)}
						<span class="row-metric">
							<strong class="row-metric-value"
								>{m.value}{#if m.unit}<span class="row-metric-unit">{m.unit}</span>{/if}</strong
							>
							{#if m.label}<span class="row-metric-label">{m.label}</span>{/if}
						</span>
					{/each}
				</span>
			{:else if summaryOf(group)}
				<span class="row-summary">{summaryOf(group)}</span>
			{/if}
		</span>
		<span class="row-chevron" aria-hidden="true">›</span>
	</button>
{/snippet}

{#if groups.length > 0}
	<section class="story" aria-label={label}>
		{#each groups as group, i (i)}
			{@render row(group, i)}
		{/each}
	</section>
{/if}

{#if activeGroup}
	{@const group = activeGroup}
	{@const metrics = metricsOf(group)}
	{@const spark = context[group.kind]}
	<BottomSheet open={true} title={statementLabel[group.kind]} onClose={closeDetail}>
		{#if metrics.length > 0}
			<div class="detail-stats">
				{#each metrics as m, i (i)}
					<div class="fact-tile" style="--i: {4 + i}">
						<p class="fact-tile-value">
							{m.value}{#if m.unit}<span class="fact-tile-unit">{m.unit}</span>{/if}
						</p>
						<p class="fact-tile-label">{m.label}</p>
					</div>
				{/each}
			</div>
		{/if}
		{#if spark}
			<div class="bleed detail-chart">
				<LineChart
					xValues={spark.xValues}
					series={spark.series}
					xFormat={spark.xFormat}
					yFormat={spark.yFormat ?? ((v) => String(Math.round(v)))}
					ariaLabel={spark.caption}
					height={72}
					bare
				/>
			</div>
			<p class="detail-caption">{spark.caption}</p>
		{/if}
		{#each group.items as statement, j (j)}
			<p class="detail-text">{statement.text}</p>
			{#if metrics.length === 0 && statement.metric}
				<p class="detail-metric">{statement.metric}</p>
			{/if}
		{/each}
		{#if group.kind === 'hint_profile'}
			<a class="detail-action" href={resolve('/(app)/profile')}> Werte im Profil eintragen </a>
		{/if}
	</BottomSheet>
{/if}

<style>
	.story {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-bottom: 1.5rem;
	}

	/* Flat fact-band fill instead of a bordered/shadowed card (Nocturne v2) —
	   --fact-color feeds off the same --chip-color each .statement-* rule
	   below already sets, so the band and its chip stay the same hue without
	   a second colour map to keep in sync. */
	.statement-row {
		--fact-color: var(--chip-color, var(--color-brand));
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		border: none;
		border-radius: var(--radius-sm);
		padding: 0.75rem 1rem;
		background: color-mix(in srgb, var(--fact-color) var(--fill-strength), var(--fill-base));
		font: inherit;
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		transition: transform var(--dur-fast) ease-out;
	}

	.statement-row:active {
		transform: scale(0.985);
	}

	/* Rows rise in on mount, staggered — same mechanism as the fact-tile
	   grids (Nocturne v3). `translate` rather than `transform`, so this
	   doesn't fight :active's `transform: scale()` above — the two
	   properties composite independently instead of one overwriting the
	   other. Excludes .statement-milestone: it already runs its own glow+pop
	   entrance below, and stacking a second one would make a record read as
	   noisier, the opposite of what the one-glow-in-the-app rule is for. */
	.statement-row:not(.statement-milestone) {
		transition:
			transform var(--dur-fast) ease-out,
			opacity var(--dur-base) var(--ease-out-soft),
			translate var(--dur-base) var(--ease-out-soft);
		transition-delay: 0s, calc(min(var(--i, 0), 5) * 60ms), calc(min(var(--i, 0), 5) * 60ms);
	}

	@starting-style {
		.statement-row:not(.statement-milestone) {
			opacity: 0;
			translate: 0 8px;
		}
	}

	.statement-milestone.statement-row {
		--fact-color: var(--color-achievement);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-achievement) 70%, transparent);
		animation: wff-milestone-land var(--dur-slow) var(--ease-overshoot) 1 both;
	}

	/* Chip above, summary below, both full-width — trying to fit chip +
	   summary + chevron on one line left the summary a handful of characters
	   before it had to cut off mid-word. Stacked, it gets the row's whole
	   width and only wraps (still bounded, at two lines) for the rare long
	   one. */
	.row-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.375rem;
	}

	.statement-chip {
		margin: 0;
	}

	.row-summary {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		line-height: 1.35;
	}

	/* The number itself, not a sentence fragment about it (#651) — the whole
	   point of a compact row is that the figure IS the summary. */
	.row-values {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 1.25rem;
	}

	.row-metric {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
	}

	.row-metric-value {
		font-size: var(--text-xl);
		font-weight: 800;
		line-height: 1.1;
		font-variant-numeric: tabular-nums;
	}

	.row-metric-unit {
		font-size: var(--text-xs);
		font-weight: 700;
		color: var(--color-text-muted);
		margin-left: 0.25rem;
	}

	.row-metric-label {
		font-size: var(--text-xs);
		color: var(--color-text-muted);
	}

	.row-chevron {
		flex-shrink: 0;
		color: var(--color-text-muted);
		font-size: var(--text-lg);
		line-height: 1;
	}

	/* The sheet's own figures, structured (#651) — the whole point of this
	   pass: a popup used to say "0,70" once in prose and nowhere else,
	   despite the backend already sending it as a proper Stat (Nocturne v3,
	   design-wff-nocturne-v3 §2 P1). Falls back to detail-metric below when a
	   group truly has no structured figures (a four-way zone split, the hint
	   kinds). */
	.detail-stats {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.detail-chart {
		margin-bottom: 0.25rem;
	}

	.detail-caption {
		margin: 0 0 1rem;
		font-size: var(--text-xs);
		color: var(--color-text-muted);
	}

	.detail-text {
		font-size: var(--text-base);
		line-height: 1.5;
		margin: 0;
	}

	.detail-metric {
		font-size: var(--text-xs);
		color: var(--color-text-muted);
		margin: 0.5rem 0 0;
	}

	.detail-metric + .detail-text {
		margin-top: 0.875rem;
	}

	.detail-action {
		display: inline-block;
		margin-top: 0.75rem;
		font-size: var(--text-sm);
	}

	/* Each kind gets the colour of the chart series it refers to, so the same
	   metric wears the same colour everywhere. */
	.statement-effort {
		--chip-color: var(--chart-power);
	}

	.statement-load,
	.statement-form {
		--chip-color: var(--color-brand);
	}

	.statement-pace {
		--chip-color: var(--chart-speed);
	}

	.statement-climb {
		--chip-color: var(--chart-elevation);
	}

	.statement-endurance {
		--chip-color: var(--chart-heart-rate);
	}

	.statement-context {
		--chip-color: var(--color-info);
	}

	.statement-trend {
		--chip-color: var(--color-success);
	}

	.statement-comparison {
		--chip-color: var(--color-text-muted);
	}

	.statement-calories {
		--chip-color: var(--zone-tempo);
	}

	.statement-milestone {
		--chip-color: var(--color-success);
	}

	.statement-hint_profile,
	.statement-hint_history {
		--chip-color: var(--color-warning);
	}
</style>

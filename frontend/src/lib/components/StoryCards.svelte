<script lang="ts" module>
	import type { RideStatement } from '$lib/rides';

	// Every statement leads with what it MEANS; the kicker says which question it
	// answers, so nobody has to infer that from the sentence itself.
	export const statementLabel: Record<RideStatement['kind'], string> = {
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
</script>

<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Snippet } from 'svelte';
	import Sheet from './Sheet.svelte';
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
		series: { name: string; values: (number | null)[] }[];
		xFormat: (x: number) => string;
		yFormat?: (y: number) => string;
		caption: string;
	}

	let {
		statements,
		label,
		context = {},
		own = [],
		tiles
	}: {
		statements: RideStatement[];
		label: string;
		context?: Partial<Record<RideStatement['kind'], SparkContext>>;
		// Kinds whose tile the caller draws itself, in `tiles` (the dashboard's
		// Fitness and Frische tiles are also the "trend" and "form" answers).
		// Their sheet is still this component's: `open` shows it, `has` says
		// whether there is one.
		own?: RideStatement['kind'][];
		tiles?: Snippet<
			[
				{
					open: (kind: RideStatement['kind']) => void;
					has: (kind: RideStatement['kind']) => boolean;
				}
			]
		>;
	} = $props();

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
	// have nothing numeric to lead with — the question alone is the tile, full
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

	// Rekta tiles (ADR adr-002-wff-rekta-design §3, adr-003 §5): a tile is a
	// button that opens the group's sheet. Its question is the kicker, its
	// figures the stat; the meaning is said in words, never by a colour. A
	// fill only where the card is a status — a missing prerequisite is a
	// warning, a record is a success — and then the word says it too.
	const statusOf: Partial<Record<RideStatement['kind'], { status: string; word: string }>> = {
		milestone: { status: 'success', word: 'Rekord' },
		hint_profile: { status: 'warning', word: 'fehlt' },
		hint_history: { status: 'warning', word: 'fehlt' }
	};

	// A tile's size follows what it holds: two figures or a sentence need two
	// columns, one figure fits one.
	function wide(group: (typeof groups)[number]): boolean {
		return metricsOf(group).length !== 1;
	}

	let activeGroup: (typeof groups)[number] | null = $state(null);

	const sheetFor = {
		open: (kind: RideStatement['kind']) =>
			(activeGroup = groups.find((g) => g.kind === kind) ?? null),
		has: (kind: RideStatement['kind']) => groups.some((g) => g.kind === kind)
	};

	function closeDetail() {
		activeGroup = null;
	}
</script>

{#if groups.length > 0 || tiles}
	<section class="rk-tiles story" aria-label={label}>
		{@render tiles?.(sheetFor)}
		{#each groups.filter((g) => !own.includes(g.kind)) as group, i (i)}
			{@const status = statusOf[group.kind]}
			{@const metrics = metricsOf(group)}
			<button
				type="button"
				class="rk-tile"
				class:rk-w2={wide(group)}
				data-status={status?.status}
				style="--rk-i: {i}"
				onclick={() => (activeGroup = group)}
			>
				{#if status}<span class="rk-tile-word">{status.word}</span>{/if}
				<span class="rk-kicker">{statementLabel[group.kind]}</span>
				{#if metrics.length > 0}
					<span class="figures">
						{#each metrics as m, j (j)}
							<span class="rk-stat">
								<b
									>{m.value}{#if m.unit}<small>{m.unit}</small>{/if}</b
								>
								{#if m.label}<span>{m.label}</span>{/if}
							</span>
						{/each}
					</span>
				{:else if summaryOf(group)}
					<span class="rk-tile-msg summary">{summaryOf(group)}</span>
				{/if}
			</button>
		{/each}
	</section>
{/if}

{#if activeGroup}
	{@const group = activeGroup}
	{@const metrics = metricsOf(group)}
	{@const spark = context[group.kind]}
	<Sheet title={statementLabel[group.kind]} onClose={closeDetail}>
		{#if metrics.length > 0}
			<div class="rk-tiles detail-stats">
				{#each metrics as m, i (i)}
					<div class="rk-tile" style="--rk-i: {i}">
						<span class="rk-stat">
							<b
								>{m.value}{#if m.unit}<small>{m.unit}</small>{/if}</b
							>
							<span>{m.label}</span>
						</span>
					</div>
				{/each}
			</div>
		{/if}
		{#if spark}
			<div class="detail-chart">
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
			<p class="rk-facts detail-caption">{spark.caption}</p>
		{/if}
		{#each group.items as statement, j (j)}
			<p class="detail-text">{statement.text}</p>
			{#if metrics.length === 0 && statement.metric}
				<p class="rk-facts detail-metric">{statement.metric}</p>
			{/if}
		{/each}
		{#if group.kind === 'hint_profile'}
			<a class="detail-action" href={resolve('/(app)/profile')}> Werte im Profil eintragen </a>
		{/if}
	</Sheet>
{/if}

<style>
	.story {
		margin-bottom: var(--rk-space-l);
	}

	.figures {
		display: flex;
		flex-wrap: wrap;
		gap: var(--rk-space-s) var(--rk-space-l);
	}

	/* The sentence of a group without figures, at most three lines in the
	   tile; the whole of it is in the sheet. */
	.summary {
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.detail-stats {
		margin-bottom: var(--rk-space-m);
	}

	.detail-caption {
		margin-bottom: var(--rk-space-m);
	}

	.detail-text + .detail-text,
	.detail-metric + .detail-text {
		margin-top: var(--rk-space-m);
	}

	.detail-metric {
		margin-top: var(--rk-space-s);
	}

	.detail-action {
		display: inline-block;
		margin-top: var(--rk-space-m);
	}
</style>

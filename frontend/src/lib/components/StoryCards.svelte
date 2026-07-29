<script lang="ts">
	import { resolve } from '$app/paths';
	import type { RideStatement } from '$lib/rides';
	import BottomSheet from './BottomSheet.svelte';

	let { statements, label }: { statements: RideStatement[]; label: string } = $props();

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

	let activeGroup: (typeof groups)[number] | null = $state(null);

	function closeDetail() {
		activeGroup = null;
	}
</script>

{#snippet row(group: (typeof groups)[number])}
	<button
		type="button"
		class="statement-row statement-{group.kind}"
		onclick={() => (activeGroup = group)}
	>
		<span class="chip statement-chip">{statementLabel[group.kind]}</span>
		{#if summaryOf(group)}
			<span class="row-summary">{summaryOf(group)}</span>
		{/if}
		<span class="row-chevron" aria-hidden="true">›</span>
	</button>
{/snippet}

{#if groups.length > 0}
	<section class="story" aria-label={label}>
		{#each groups as group, i (i)}
			{@render row(group)}
		{/each}
	</section>
{/if}

{#if activeGroup}
	{@const group = activeGroup}
	<BottomSheet open={true} title={statementLabel[group.kind]} onClose={closeDetail}>
		{#each group.items as statement, j (j)}
			<p class="detail-text">{statement.text}</p>
			{#if statement.metric}
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

	.statement-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		border: none;
		border-radius: var(--radius-md);
		padding: 0.75rem 1rem;
		box-shadow: var(--shadow-sm);
		background: var(--color-surface);
		font: inherit;
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
	}

	.statement-row:hover {
		box-shadow: var(--shadow-md);
	}

	.statement-chip {
		flex-shrink: 0;
		margin: 0;
	}

	.row-summary {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.row-chevron {
		flex-shrink: 0;
		color: var(--color-text-muted);
		font-size: var(--text-lg);
		line-height: 1;
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

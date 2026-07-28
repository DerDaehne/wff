<script lang="ts">
	import { resolve } from '$app/paths';
	import type { RideStatement } from '$lib/rides';

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

	// A ride with power/pulse and a full set of context signals produced ten
	// or more full-sentence cards — helpful, but a wall of text on every
	// single ride. Lead with the two universally-relevant kinds plus
	// whatever is rare and actionable (a new record, a data-gap nudge); the
	// rest — pace, climb, endurance, calories, zones prose, conditions,
	// how-it-compares — is real information, just not urgent enough to force
	// on every reading. Hidden behind one click instead of deleted.
	const alwaysVisible = new Set<RideStatement['kind']>([
		'effort',
		'load',
		'milestone',
		'hint_profile',
		'hint_history'
	]);

	// Pages whose statements don't include any always-visible kind at all
	// (the dashboard's form/trend cards, say) fall back to showing
	// everything — collapsing to an empty lead-in with a "show more" button
	// hiding the ONLY content would be worse than the wall of text this
	// exists to fix.
	let hasPriority = $derived(groups.some((g) => alwaysVisible.has(g.kind)));
	let primaryGroups = $derived(
		hasPriority ? groups.filter((g) => alwaysVisible.has(g.kind)) : groups
	);
	let secondaryGroups = $derived(
		hasPriority ? groups.filter((g) => !alwaysVisible.has(g.kind)) : []
	);
	let expanded = $state(false);
</script>

{#snippet card(group: (typeof groups)[number])}
	<article class="statement statement-{group.kind}">
		<p class="chip statement-chip">{statementLabel[group.kind]}</p>
		{#each group.items as statement, j (j)}
			<p class="statement-text">{statement.text}</p>
			{#if statement.metric}
				<p class="statement-metric">{statement.metric}</p>
			{/if}
		{/each}
		{#if group.kind === 'hint_profile'}
			<a class="statement-action" href={resolve('/(app)/profile')}> Werte im Profil eintragen </a>
		{/if}
	</article>
{/snippet}

{#if groups.length > 0}
	<section class="story" aria-label={label}>
		{#each primaryGroups as group, i (i)}
			{@render card(group)}
		{/each}
		{#if expanded}
			{#each secondaryGroups as group, i (i)}
				{@render card(group)}
			{/each}
		{/if}
	</section>
	{#if secondaryGroups.length > 0}
		<button class="details-toggle" type="button" onclick={() => (expanded = !expanded)}>
			{expanded ? 'Weniger anzeigen' : `${secondaryGroups.length} weitere Details anzeigen`}
		</button>
	{/if}
{/if}

<style>
	.story {
		display: grid;
		gap: 0.75rem;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		/* Cards size to their own content — without this the short ones stretch
		   to match the tallest in the row and show a big empty box. */
		align-items: start;
		margin-bottom: 2rem;
	}

	.statement {
		border-radius: var(--radius-md);
		padding: 1.25rem;
		box-shadow: var(--shadow-sm);
		background: var(--color-surface);
	}

	/* The chip carries the colour, so the card itself stays a calm surface — a
	   full-card wash behind body text hurt contrast in the dark scheme. */
	.statement-chip {
		margin: 0 0 0.75rem;
	}

	.statement-text {
		font-size: var(--text-base);
		line-height: 1.5;
		margin: 0;
	}

	.statement-metric {
		font-size: var(--text-xs);
		color: var(--color-text-muted);
		margin: 0.5rem 0 0;
	}

	/* Second and later reasons inside one card need air above them. */
	.statement-metric + .statement-text {
		margin-top: 0.875rem;
	}

	.statement-action {
		display: inline-block;
		margin-top: 0.5rem;
		font-size: var(--text-sm);
	}

	.details-toggle {
		display: block;
		margin: -1rem 0 2rem;
		padding: 0.5rem 0;
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		text-decoration: underline;
		cursor: pointer;
	}

	.details-toggle:hover {
		color: inherit;
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

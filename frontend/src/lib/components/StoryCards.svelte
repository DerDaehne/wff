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
</script>

{#if groups.length > 0}
	<section class="story" aria-label={label}>
		{#each groups as group, i (i)}
			<article class="statement statement-{group.kind}">
				<p class="chip statement-chip">{statementLabel[group.kind]}</p>
				{#each group.items as statement, j (j)}
					<p class="statement-text">{statement.text}</p>
					{#if statement.metric}
						<p class="statement-metric">{statement.metric}</p>
					{/if}
				{/each}
				{#if group.kind === 'hint_profile'}
					<a class="statement-action" href={resolve('/(app)/profile')}>
						Werte im Profil eintragen
					</a>
				{/if}
			</article>
		{/each}
	</section>
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

	.statement-hint_profile,
	.statement-hint_history {
		--chip-color: var(--color-warning);
	}
</style>

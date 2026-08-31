<script lang="ts">
	import type { RideStory } from '$lib/rides';

	// The deep block that opens a view: one big number leads, the rest are
	// supporting figures. Shared by the ride detail and the dashboard — both
	// answer a question with the same shape (#602), so they look the same.
	let {
		story,
		fallbackTitle,
		note,
		meterColor = 'var(--chart-power)',
		showGauge = true
	}: {
		story: RideStory | null;
		fallbackTitle: string;
		note?: string;
		meterColor?: string;
		// The dashboard shows the training-level gauge in its "auf einen Blick"
		// section instead (#657) — showing it here too would say the same thing
		// twice on one page.
		showGauge?: boolean;
	} = $props();
</script>

<header class="hero">
	{#if story?.subtitle}
		<p class="hero-date">{story.subtitle}</p>
	{/if}
	<h1>{story?.title || fallbackTitle}</h1>

	{#if story && story.stats.length > 0}
		<div class="hero-stats">
			{#each story.stats as stat, i (stat.label)}
				<div style="--i: {i}">
					<p class="hero-stat-figure">
						<span class="stat-value">{stat.value}</span>
						{#if stat.unit}<span class="stat-unit">{stat.unit}</span>{/if}
					</p>
					<p class="hero-stat-label">{stat.label}</p>
				</div>
			{/each}
		</div>
	{/if}

	{#if showGauge && story?.gauge}
		<div class="hero-meter">
			<div class="meter" style="--meter-color: {meterColor}">
				<span style="width: {story.gauge.percent}%"></span>
			</div>
			<p class="hero-note">{story.gauge.label} · {story.gauge.caption}</p>
		</div>
	{/if}

	{#if note}
		<p class="hero-note hero-note-spaced">{note}</p>
	{/if}
</header>

<style>
	/* Full-bleed, square-cornered (Nocturne v2) — its own distinct
	   --color-hero-bg tint is what separates it from the page now, not a
	   rounded card floating on top of one. */
	.hero {
		background: var(--color-hero-bg);
		color: var(--color-hero-text);
		padding: 1.75rem 1rem;
		margin: 0 -1rem 1.5rem;
	}

	@media (min-width: 768px) {
		.hero {
			margin-inline: 0;
			border-radius: var(--radius-lg);
			padding: 1.75rem;
		}
	}

	.hero-date {
		margin: 0 0 0.25rem;
		font-size: var(--text-xs);
		font-weight: 700;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--color-hero-muted);
	}

	.hero h1 {
		margin: 0 0 1.5rem;
		font-size: var(--text-xl);
		font-weight: 700;
	}

	/* Grid, not flex-wrap: three stats (CTL/ATL/TSB) at a narrow phone width
	   used to wrap 2-then-1, the lone third stat sitting oddly under a big
	   gap instead of in an aligned column. A grid keeps every row's columns
	   the same width even when a later row has fewer items. */
	.hero-stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(5.5rem, 1fr));
		gap: 1.5rem 1rem;
	}

	/* The three figures settle in on mount, staggered — starts at the same
	   t=0 as the tile grids elsewhere on the page, so nothing waits for
	   anything else to finish first (Nocturne v3). */
	.hero-stats > div {
		transition:
			opacity var(--dur-base) var(--ease-out-soft),
			transform var(--dur-base) var(--ease-out-soft);
		transition-delay: calc(min(var(--i, 0), 5) * 60ms);
	}

	@starting-style {
		.hero-stats > div {
			opacity: 0;
			transform: translateY(8px);
		}
	}

	.hero-stat-figure {
		display: flex;
		align-items: baseline;
		gap: 0.375rem;
		margin: 0;
	}

	.hero-stat-figure :global(.stat-unit) {
		color: var(--color-hero-muted);
	}

	.hero-stat-label {
		margin: 0.25rem 0 0;
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-hero-muted);
	}

	.hero-meter {
		margin-top: 1.75rem;
		max-width: 26rem;
	}

	.hero-meter :global(.meter) {
		background: rgba(255, 255, 255, 0.16);
	}

	.hero-note {
		margin: 0.5rem 0 0;
		font-size: var(--text-sm);
		color: var(--color-hero-muted);
	}

	.hero-note-spaced {
		margin-top: 1rem;
	}

	@media (max-width: 600px) {
		.hero {
			padding: 1.25rem;
		}

		/* The big figure has to shrink on a phone or three stats wrap to three
		   rows and the hero eats the whole screen. */
		.hero-stats {
			gap: 1rem 1.5rem;
		}

		.hero-stat-figure :global(.stat-value) {
			font-size: var(--text-3xl);
		}
	}
</style>

<script lang="ts">
	import type { RideStory } from '$lib/rides';

	// The deep block that opens a view: one big number leads, the rest are
	// supporting figures. Shared by the ride detail and the dashboard — both
	// answer a question with the same shape (#602), so they look the same.
	let {
		story,
		fallbackTitle,
		note
	}: { story: RideStory | null; fallbackTitle: string; note?: string } = $props();
</script>

<header class="hero">
	{#if story?.subtitle}
		<p class="hero-date">{story.subtitle}</p>
	{/if}
	<h1>{story?.title || fallbackTitle}</h1>

	{#if story && story.stats.length > 0}
		<div class="hero-stats">
			{#each story.stats as stat (stat.label)}
				<div>
					<p class="hero-stat-figure">
						<span class="stat-value">{stat.value}</span>
						{#if stat.unit}<span class="stat-unit">{stat.unit}</span>{/if}
					</p>
					<p class="hero-stat-label">{stat.label}</p>
				</div>
			{/each}
		</div>
	{/if}

	{#if story?.intensity}
		<div class="hero-meter">
			<div class="meter" style="--meter-color: var(--chart-power)">
				<span style="width: {story.intensity.percent}%"></span>
			</div>
			<p class="hero-note">{story.intensity.label} · {story.intensity.caption}</p>
		</div>
	{/if}

	{#if note}
		<p class="hero-note hero-note-spaced">{note}</p>
	{/if}
</header>

<style>
	.hero {
		background: var(--color-hero-bg);
		color: var(--color-hero-text);
		border-radius: var(--radius-lg);
		padding: 1.75rem;
		margin-bottom: 1.5rem;
		box-shadow: var(--shadow-md);
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

	.hero-stats {
		display: flex;
		flex-wrap: wrap;
		gap: 1.5rem 2.5rem;
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

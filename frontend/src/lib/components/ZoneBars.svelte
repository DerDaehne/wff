<script lang="ts">
	import type { ZoneDistribution } from '$lib/zones';

	let { distribution, label }: { distribution: ZoneDistribution; label: string } = $props();

	// Only bands the rider actually spent time in. An empty row explains
	// nothing and pushes the ones that matter off the screen on a phone.
	let used = $derived(distribution.zones.filter((z) => z.seconds > 0));

	function time(seconds: number): string {
		const h = Math.floor(seconds / 3600);
		const m = Math.round((seconds % 3600) / 60);
		return h > 0 ? `${h} h ${String(m).padStart(2, '0')} min` : `${m} min`;
	}

	function percent(share: number): string {
		return `${Math.round(share * 100)} %`;
	}
</script>

<figure class="zones">
	<figcaption class="sr-only">{label}</figcaption>
	<div
		class="bar"
		role="img"
		aria-label={used.map((z) => `${z.name} ${percent(z.share)}`).join(', ')}
	>
		{#each used as zone (zone.key)}
			<div
				class="segment"
				style="width: {zone.share * 100}%; background: var(--zone-{zone.key})"
			></div>
		{/each}
	</div>

	<dl class="legend">
		{#each used as zone (zone.key)}
			<div class="row">
				<dt>
					<span class="swatch" style="background: var(--zone-{zone.key})"></span>
					{zone.name}
				</dt>
				<dd class="numbers">{time(zone.seconds)} · {percent(zone.share)}</dd>
				<dd class="meaning">{zone.meaning}</dd>
			</div>
		{/each}
	</dl>
</figure>

<style>
	.zones {
		margin: 0;
		/* Same measure as the panel's intro text. Full panel width pushed the
		   time and percentage a screen away from the band they belong to. */
		max-width: 60ch;
	}

	.bar {
		display: flex;
		height: 1.25rem;
		border-radius: var(--radius-pill);
		overflow: hidden;
		background: var(--color-border);
	}

	.segment {
		height: 100%;
	}

	.legend {
		margin: 1rem 0 0;
	}

	.row {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.25rem 1rem;
		padding: 0.6rem 0;
		border-top: 1px solid var(--color-border);
	}

	.row:first-child {
		border-top: none;
	}

	dt {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 700;
	}

	dd {
		margin: 0;
	}

	.numbers {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	.meaning {
		grid-column: 1 / -1;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		max-width: 60ch;
	}

	.swatch {
		width: 0.75rem;
		height: 0.75rem;
		border-radius: var(--radius-sm);
		flex: none;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>

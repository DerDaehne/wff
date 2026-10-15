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

<!-- One Rekta meter per zone, cool to warm, in ink: the zone is told by its
     name, not by a colour (ADR adr-003-wff-datenfarben-rekta, until Rekta has
     a slot for colour codes, Rekta #1053). -->
<figure class="zones">
	<figcaption class="rk-sr">{label}</figcaption>
	{#if distribution.assumed}
		<p class="assumed-note">
			Geschätzt aus deinem bisher höchsten aufgezeichneten Puls, nicht aus einem echten
			Schwellentest — nimm die Grenzen als groben Anhalt.
		</p>
	{/if}
	<ul>
		{#each used as zone (zone.key)}
			<li>
				<p class="rk-meter-head">
					<span class="name">{zone.name}</span>
					<b>{percent(zone.share)}</b>
				</p>
				<div
					class="rk-meter"
					role="meter"
					aria-label={zone.name}
					aria-valuemin="0"
					aria-valuemax="100"
					aria-valuenow={Math.round(zone.share * 100)}
					aria-valuetext="{percent(zone.share)}, {time(zone.seconds)}"
					style="--rk-v: {zone.share}"
				></div>
				<p class="meaning">{time(zone.seconds)} · {zone.meaning}</p>
			</li>
		{/each}
	</ul>
</figure>

<style>
	.zones {
		margin: 0;
		/* Same measure as the panel's intro text, so the share stays near its
		   bar. */
		max-width: 60ch;
	}

	.assumed-note {
		margin: 0 0 var(--rk-space-m);
		color: var(--rk-text-contrast-high);
		font-size: var(--rk-font-size-xs);
	}

	ul {
		display: grid;
		gap: var(--rk-space-m);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.name {
		color: var(--rk-text-primary);
		font-weight: var(--rk-font-weight-semibold);
	}

	.meaning {
		margin-top: var(--rk-space-xs);
		color: var(--rk-text-contrast-high);
		font-size: var(--rk-font-size-xs);
	}
</style>

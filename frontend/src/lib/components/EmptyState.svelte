<script lang="ts">
	// One place for "there's nothing here yet" (#634) — every empty list
	// used to write its own message+link pair, and the three drifted apart
	// in structure even though they say the same thing. heading is optional:
	// a page that already has its own <h1> above the {#if} doesn't need a
	// second one.
	let {
		heading,
		message,
		actionHref,
		actionLabel
	}: {
		heading?: string;
		message: string;
		actionHref?: string;
		actionLabel?: string;
	} = $props();
</script>

<!-- Rekta's empty state (.rk-empty): a sentence that says why, with the way
     on, on a hairline instead of in a box. -->
<div class="reveal" style="--i: 0">
	{#if heading}
		<h1 class="rk-title">{heading}</h1>
	{/if}
	<div class="rk-empty">
		<p>{message}</p>
		{#if actionHref && actionLabel}
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass an already resolved path -->
			<p><a href={actionHref}>{actionLabel}</a></p>
		{/if}
	</div>
</div>

<style>
	h1 {
		margin-bottom: var(--rk-space-m);
	}

	/* Prose keeps the measure used elsewhere in the app, never edge to edge. */
	.rk-empty {
		max-width: 60ch;
	}
</style>

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

<div class="empty-state reveal" style="--i: 0">
	{#if heading}
		<h1>{heading}</h1>
	{/if}
	<p>{message}</p>
	{#if actionHref && actionLabel}
		<p><a href={actionHref}>{actionLabel}</a></p>
	{/if}
</div>

<style>
	/* No width constraint at all used to mean every "nothing here yet"
	   message — dashboard, rides, vergleich's opt-in prompt, all of them —
	   ran edge to edge of .content on a wide desktop screen: one unbroken
	   line of prose a thousand-plus pixels wide. 60ch matches the measure
	   already used for prose elsewhere in the app (panel-sub, glossar's
	   intro, ...). */
	.empty-state p {
		max-width: 60ch;
	}
</style>

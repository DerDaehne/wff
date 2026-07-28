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

<div class="empty-state">
	{#if heading}
		<h1>{heading}</h1>
	{/if}
	<p>{message}</p>
	{#if actionHref && actionLabel}
		<p><a href={actionHref}>{actionLabel}</a></p>
	{/if}
</div>

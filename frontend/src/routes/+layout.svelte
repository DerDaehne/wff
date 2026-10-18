<script lang="ts">
	import { pwaInfo } from 'virtual:pwa-info';
	import { onMount } from 'svelte';
	// Rekta first, WFF's additions after (ADR adr-002-wff-rekta-design).
	import '$lib/rekta/rekta.tokens.css';
	import '$lib/rekta/rekta.fonts.css';
	import '$lib/rekta/rekta.css';
	import '../app.css';
	import { start } from '$lib/rekta/rekta.js';
	import { de } from '$lib/rekta/rekta.strings.de.js';

	let { children } = $props();

	// Rekta's behaviour for every page (ADR adr-004-wff-rekta-js): tiles tilt
	// where they are pressed, tooltips, the first paint as an arrival, and the
	// remembered theme/density/motion. Every word rekta.js shows or says, and
	// its numbers, in German (Rekta #1068).
	onMount(() => {
		const rekta = start({ locale: 'de', strings: de });
		return () => rekta.destroy();
	});

	onMount(async () => {
		if (pwaInfo) {
			const { useRegisterSW } = await import('virtual:pwa-register/svelte');
			useRegisterSW({ immediate: true });
		}
	});

	let webManifestLink = $derived(pwaInfo ? pwaInfo.webManifest.linkTag : '');
</script>

<svelte:head>
	<!-- eslint-disable-next-line svelte/no-at-html-tags -- build-time markup from vite-plugin-pwa, not user input -->
	{@html webManifestLink}
</svelte:head>

{@render children()}

<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import { pwaInfo } from 'virtual:pwa-info';
	import { onMount } from 'svelte';
	// Rekta first, WFF aliases and data accents after (ADR adr-002-wff-rekta-design).
	import '$lib/styles/rekta.tokens.css';
	import '$lib/styles/rekta.fonts.css';
	import '$lib/styles/rekta.css';
	import '../app.css';

	let { children } = $props();

	onMount(async () => {
		if (pwaInfo) {
			const { useRegisterSW } = await import('virtual:pwa-register/svelte');
			useRegisterSW({ immediate: true });
		}
	});

	let webManifestLink = $derived(pwaInfo ? pwaInfo.webManifest.linkTag : '');
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<!-- eslint-disable-next-line svelte/no-at-html-tags -- build-time markup from vite-plugin-pwa, not user input -->
	{@html webManifestLink}
</svelte:head>

{@render children()}

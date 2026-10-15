<script lang="ts">
	import { onMount } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { whoAmI, logout } from '$lib/webauthn';
	import Palette, { type PaletteEntry } from '$lib/components/Palette.svelte';
	import { pivotWidths } from '$lib/pivot';

	let { children } = $props();

	let authState: 'checking' | 'authed' = $state('checking');

	onMount(async () => {
		const me = await whoAmI();
		if (!me) {
			await goto(resolve('/login'));
			return;
		}
		authState = 'authed';
	});

	async function handleLogout() {
		await logout();
		await goto(resolve('/login'));
	}

	// Rekta shell (#1057, ADR adr-002-wff-rekta-design, Rekta docs/navigation.md):
	// the top places are picked by the pivot at the top of the place; every place
	// below them starts with a crumb to its parent; the app bar at the bottom
	// carries commands only, plus "zurück" on a phone. Upload is a command there,
	// not a place. Rekta does not allow places in the app bar yet (Rekta #1065).
	const places = [
		{ href: resolve('/(app)'), label: 'Start' },
		{ href: resolve('/(app)/rides'), label: 'Fahrten' },
		{ href: resolve('/(app)/profile'), label: 'Profil' }
	];
	const parentOf: Record<string, (typeof places)[number]> = {
		'/(app)/rides/[id]': places[1],
		'/(app)/rides/vergleich': places[1],
		'/(app)/rueckblick': places[0],
		'/(app)/glossar': places[0],
		'/(app)/upload': places[0],
		'/(app)/raeder': places[2],
		'/(app)/vergleich': places[2]
	};
	let isTop = $derived(places.some((p) => p.href === page.url.pathname));
	let parent = $derived(page.route.id ? parentOf[page.route.id] : undefined);

	// "Back stays in the app": with no step taken inside the app yet (a cold
	// entry on a deep link), back goes forward to the parent instead of leaving.
	let depth = 0;
	// Rekta's arrival build-in (behaviour.md, data-rk-arriving): tiles rise and
	// sparklines draw only while the place carries it, so it goes again once
	// the build-in is over and a re-render never replays it.
	let arriving = $state(true);
	afterNavigate(({ from, type, delta }) => {
		if (type === 'popstate') depth += delta ?? 0;
		else if (from) depth++;
		arriving = true;
		setTimeout(() => (arriving = false), 1000);
	});
	function back() {
		if (depth > 0) history.back();
		else if (parent) goto(parent.href);
	}

	const paletteEntries: PaletteEntry[] = [
		...places.map((p) => ({ ...p, group: 'Orte' })),
		{ label: 'Räder', group: 'Orte', href: resolve('/(app)/raeder'), keywords: 'fahrrad kette' },
		{ label: 'Jahresrückblick', group: 'Orte', href: resolve('/(app)/rueckblick') },
		{ label: 'Vergleich mit anderen', group: 'Orte', href: resolve('/(app)/vergleich') },
		{ label: 'Glossar', group: 'Orte', href: resolve('/(app)/glossar'), keywords: 'begriffe' },
		{
			label: 'Fahrt hochladen',
			group: 'Befehle',
			href: resolve('/(app)/upload'),
			keywords: 'upload fit'
		},
		{ label: 'Abmelden', group: 'Befehle', run: handleLogout, keywords: 'logout' }
	];
</script>

{#if authState === 'checking'}
	<p>Lädt…</p>
{:else if authState === 'authed'}
	<main class="rk-place" data-rk-arriving={arriving ? '' : undefined}>
		{#if isTop}
			<nav class="rk-pivot rk-pivot--nav" aria-label="Orte" use:pivotWidths>
				{#each places as p (p.href)}
					<a href={p.href} aria-current={page.url.pathname === p.href ? 'page' : undefined}>
						{p.label}
					</a>
				{/each}
			</nav>
		{/if}
		<div class="content">
			{#if parent}
				<a class="rk-crumb" href={parent.href}>
					<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M11 4 5 10l6 6M5 10h11" /></svg>
					{parent.label}
				</a>
			{/if}
			{@render children()}
		</div>
	</main>

	<div class="rk-appbar" role="region" aria-label="App-Leiste">
		{#if parent}
			<button class="rk-ab rk-ab-back" type="button" onclick={back}>
				<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M11 4 5 10l6 6M5 10h11" /></svg>
				<span>zurück</span>
			</button>
		{/if}
		<button
			class="rk-ab goto"
			type="button"
			popovertarget="palette"
			aria-keyshortcuts="Control+K Meta+K"
		>
			<svg viewBox="0 0 20 20" aria-hidden="true"
				><circle cx="9" cy="9" r="5.5" /><path d="m13 13 4 4" /></svg
			>
			<span>gehe zu</span>
			<span class="rk-key rk-ab-k" aria-hidden="true">Strg K</span>
		</button>
		<a class="rk-ab" href={resolve('/(app)/upload')}>
			<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 13V3M6 7l4-4 4 4M4 16h12" /></svg>
			<span>hochladen</span>
		</a>
	</div>

	<Palette entries={paletteEntries} />
{/if}

<style>
	main {
		max-width: 64rem;
		margin-inline: auto;
	}

	.content {
		padding: var(--rk-space-m) var(--rk-space-m)
			calc(var(--rk-appbar-height) + env(safe-area-inset-bottom) + var(--rk-space-l));
	}

	@media (min-width: 760px) and (min-height: 481px) {
		.content {
			padding-inline: var(--rk-space-xl);
		}
	}

	/* No live status in WFF's bar, so the commands push themselves to the end. */
	.goto {
		margin-inline-start: auto;
	}

	a.rk-ab {
		text-decoration: none;
	}
</style>

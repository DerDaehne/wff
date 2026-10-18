<script lang="ts">
	import { onMount } from 'svelte';
	import { afterNavigate, goto, onNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { whoAmI, logout } from '$lib/webauthn';
	import Palette, { type PaletteEntry } from '$lib/components/Palette.svelte';
	import { appbar, keys, prefs, toaster, turn } from '$lib/rekta/rekta.js';
	import { setToaster } from '$lib/toast';

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
	// carries the commands, plus "zurück" on a phone. Upload is a command there,
	// not a place. On a phone the places sit in the app bar instead of the pivot,
	// in the thumb's reach, and the commands move into "mehr" (#1105, Rekta 0.3
	// .rk-ab-places). Icons are Rekta's (src/icons), inline.
	const places = [
		{ href: resolve('/(app)'), label: 'Start', icon: 'M3 9.5 10 3.5l7 6M5 8v9h10V8' },
		{ href: resolve('/(app)/rides'), label: 'Fahrten', icon: 'M3 5h14M3 10h14M3 15h14' },
		{
			href: resolve('/(app)/profile'),
			label: 'Profil',
			icon: 'M13.5 7a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0M3.5 17a6.5 6.5 0 0 1 13 0'
		}
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
	// The place you are in; below a place (a detail), the place it belongs to.
	let here = $derived(places.find((p) => p.href === page.url.pathname) ?? parent);

	// "Back stays in the app": with no step taken inside the app yet (a cold
	// entry on a deep link), back goes forward to the parent instead of leaving.
	let depth = 0;
	afterNavigate(({ from, type, delta }) => {
		if (type === 'popstate') depth += delta ?? 0;
		// The first load counts as an arrival, not a step (SvelteKit reports it as
		// 'enter', with a 'from' when there is no server rendering).
		else if (from && type !== 'enter') depth++;
	});
	function back() {
		if (depth > 0) history.back();
		else if (parent) goto(parent.href);
	}

	// Rekta's place change, the turnstile (ADR adr-004-wff-rekta-js): the place
	// turns away, SvelteKit swaps the route, the new place turns in and its
	// tiles build in. Back, or a top place earlier in the pivot's order, runs it
	// the other way round. Only a new path is a new place: Rückblick's ?year=
	// stays where it is. Between two top places the strip that picks them stays
	// put and only the place below it turns (Rekta motion.md: the pivot that
	// picks the place turns the place, not itself); to or from a place below
	// them the strip comes or goes, so the whole place turns.
	let placeEl: HTMLElement | undefined = $state();
	let contentEl: HTMLElement | undefined = $state();
	onNavigate((nav) => {
		const from = nav.from?.url.pathname;
		const to = nav.to?.url.pathname;
		if (nav.willUnload || !from || !to || from === to) return;
		const order = (path: string) => places.findIndex((p) => p.href === path);
		const el = order(from) >= 0 && order(to) >= 0 ? contentEl : placeEl;
		if (!el) return;
		const back =
			nav.type === 'popstate'
				? (nav.delta ?? 0) < 0
				: order(from) >= 0 && order(to) >= 0 && order(to) < order(from);
		const active = document.activeElement;
		const pressed = active instanceof HTMLElement && el.contains(active) ? active : null;
		return turn.leave(el, { back, pressed }).then(() => () => turn.enter(el, { back }));
	});

	// One back for everything (Rekta navigation.md): Escape goes back a place,
	// once nothing inner (a sheet, the palette, a text field) wants it.
	onMount(() =>
		keys.on(10, (e) => {
			if (e.key !== 'Escape' || !parent || !keys.escapeIsFree(e)) return false;
			back();
			return true;
		})
	);

	// The toast region stays mounted after the place, so a toast outlives the
	// view that raised it (Rekta behaviour.md, toaster).
	function toastRegion(el: HTMLElement) {
		const t = toaster(el);
		setToaster(t);
		return {
			destroy() {
				setToaster(null);
				t.destroy();
			}
		};
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
		{ label: 'Abmelden', group: 'Befehle', run: handleLogout, keywords: 'logout' },
		// Rekta's own switches (prefs), remembered in this browser.
		...[
			['Darstellung: hell', 'theme', 'light'],
			['Darstellung: dunkel', 'theme', 'dark'],
			['Darstellung: wie das System', 'theme', 'system'],
			['Bewegung: aus', 'motion', 'off'],
			['Bewegung: an', 'motion', 'on'],
			['Bewegung: wie das System', 'motion', 'system']
		].map(([label, name, value]) => ({
			label,
			group: 'Einstellungen',
			keywords: name === 'theme' ? 'theme farben modus' : 'animation motion',
			run: () => prefs.set(name as 'theme' | 'motion', value)
		}))
	];
</script>

{#if authState === 'checking'}
	<p>Lädt…</p>
{:else if authState === 'authed'}
	<main class="rk-place" bind:this={placeEl}>
		{#if isTop}
			<nav class="rk-pivot rk-pivot--nav" aria-label="Orte">
				{#each places as p (p.href)}
					<a href={p.href} aria-current={page.url.pathname === p.href ? 'page' : undefined}>
						{p.label}
					</a>
				{/each}
			</nav>
		{/if}
		<div class="content" bind:this={contentEl}>
			{#if parent}
				<a class="rk-crumb" href={parent.href}>
					<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M11 4 5 10l6 6M5 10h11" /></svg>
					{parent.label}
				</a>
			{/if}
			{@render children()}
		</div>
	</main>

	<div class="rk-toasts" role="status" aria-atomic="false" use:toastRegion></div>

	<!-- rekta.js appbar(): while the places show in the bar (a phone), every
	     command is in "mehr"; wider, only what does not fit. -->
	<div class="rk-appbar" role="region" aria-label="App-Leiste" use:appbar>
		{#if parent}
			<button class="rk-ab rk-ab-back" type="button" onclick={back}>
				<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M11 4 5 10l6 6M5 10h11" /></svg>
				<span>zurück</span>
			</button>
		{/if}
		<nav class="rk-ab-places" aria-label="Orte">
			{#each places as p (p.href)}
				<a class="rk-ab" href={p.href} aria-current={here === p ? 'page' : undefined}>
					<svg viewBox="0 0 20 20" aria-hidden="true"><path d={p.icon} /></svg>
					<span>{p.label.toLowerCase()}</span>
				</a>
			{/each}
		</nav>
		<button
			class="rk-ab goto"
			type="button"
			popovertarget="palette"
			aria-label="gehe zu"
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
		<button class="rk-ab rk-ab-more" type="button" hidden>
			<svg viewBox="0 0 20 20" aria-hidden="true"
				><path d="M3 9h2v2H3zM9 9h2v2H9zM15 9h2v2h-2z" fill="currentColor" stroke="none" /></svg
			>
			<span>mehr</span>
		</button>
	</div>

	<Palette entries={paletteEntries} />
{/if}

<style>
	/* The place spans the window and its content starts flush left under the
	   strip, at every place (Rekta: content is flush left, never centred). A
	   list or a form keeps a readable measure; a panorama does not (below). */
	.content {
		max-width: 64rem;
		padding: var(--rk-space-m) var(--rk-space-m)
			calc(var(--rk-appbar-height) + env(safe-area-inset-bottom) + var(--rk-space-l));
	}

	@media (min-width: 760px) and (min-height: 481px) {
		.content {
			padding-inline: var(--rk-space-xl);
		}
	}

	/* A place of named sections is a Rekta panorama (layout.md): the panorama
	   brings its own side padding. Sideways the place fills the height between
	   the top and the app bar, the sections stand side by side and each one
	   scrolls on its own; the page itself does not scroll. */
	main:has(> .content > :global(.rk-panorama)) > .content {
		max-width: none;
		padding-inline: 0;
	}

	@media (min-width: 760px) and (min-height: 481px) {
		main:has(> .content > :global(.rk-panorama)) {
			display: flex;
			flex-direction: column;
			height: calc(100dvh - var(--rk-appbar-height) - env(safe-area-inset-bottom));
		}

		main:has(> .content > :global(.rk-panorama)) > .content {
			display: flex;
			flex: 1 1 auto;
			flex-direction: column;
			min-height: 0;
			padding-block: 0;
		}

		.content > :global(.rk-panorama) {
			flex: 1 1 auto;
		}
	}

	/* No live status in WFF's bar, so the commands push themselves to the end. */
	.goto {
		margin-inline-start: auto;
	}

	/* On a phone the places are in the app bar (Rekta: below 760px, decided by
	   the bar's own width, which here is the window's); the pivot at the top
	   would say the same twice. */
	@media (max-width: 759.98px) {
		.rk-pivot--nav {
			display: none;
		}
	}

	a.rk-ab {
		text-decoration: none;
	}
</style>

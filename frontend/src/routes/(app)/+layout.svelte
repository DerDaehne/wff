<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { whoAmI, logout } from '$lib/webauthn';

	let { children } = $props();

	let authState: 'checking' | 'authed' = $state('checking');

	// Desktop-only (the mobile bottom bar has no collapse concept — see the
	// .sidebar-toggle CSS, hidden below the sidebar breakpoint). Remembered
	// per browser: a rider who collapses it on their desktop shouldn't have
	// to redo that every visit.
	const sidebarStorageKey = 'wff-sidebar-collapsed';
	let sidebarCollapsed = $state(false);

	onMount(async () => {
		try {
			sidebarCollapsed = localStorage.getItem(sidebarStorageKey) === '1';
		} catch {
			// Private browsing / storage disabled: default (expanded) stands.
		}
		const me = await whoAmI();
		if (!me) {
			await goto(resolve('/login'));
			return;
		}
		authState = 'authed';
	});

	function toggleSidebar() {
		sidebarCollapsed = !sidebarCollapsed;
		try {
			localStorage.setItem(sidebarStorageKey, sidebarCollapsed ? '1' : '0');
		} catch {
			// Nothing to persist to — the toggle still works for this visit.
		}
	}

	async function handleLogout() {
		await logout();
		await goto(resolve('/login'));
	}

	// short: what a collapsed sidebar shows instead of the full label — see
	// the .nav-label-short/-full split below.
	const navItems = [
		{ href: resolve('/(app)'), label: 'Start', short: 'S' },
		{ href: resolve('/(app)/rides'), label: 'Fahrten', short: 'F' },
		{ href: resolve('/(app)/upload'), label: 'Upload', short: 'U' },
		{ href: resolve('/(app)/profile'), label: 'Profil', short: 'P' }
	];
</script>

{#if authState === 'checking'}
	<p>Lädt…</p>
{:else if authState === 'authed'}
	<div class="app-shell">
		<nav class="nav" class:collapsed={sidebarCollapsed}>
			<button
				class="sidebar-toggle"
				onclick={toggleSidebar}
				aria-expanded={!sidebarCollapsed}
				aria-label={sidebarCollapsed ? 'Navigation ausklappen' : 'Navigation einklappen'}
			>
				{sidebarCollapsed ? '»' : '«'}
			</button>
			<ul>
				{#each navItems as item, i (item.href)}
					<li class="reveal" style="--i: {i}">
						<a
							href={item.href}
							aria-current={page.url.pathname === item.href ? 'page' : undefined}
							aria-label={item.label}
							title={sidebarCollapsed ? item.label : undefined}
						>
							<span class="nav-label-full" aria-hidden="true">{item.label}</span>
							<span class="nav-label-short" aria-hidden="true">{item.short}</span>
						</a>
					</li>
				{/each}
			</ul>
			<button
				class="logout"
				onclick={handleLogout}
				aria-label="Abmelden"
				title={sidebarCollapsed ? 'Abmelden' : undefined}
			>
				<span class="nav-label-full" aria-hidden="true">Abmelden</span>
				<span class="nav-label-short" aria-hidden="true">Ab</span>
			</button>
		</nav>
		<main class="content">
			{@render children()}
		</main>
	</div>
{/if}

<style>
	.app-shell {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
	}

	.content {
		flex: 1;
		padding: 1rem;
		/* Clears the fixed bottom bar INCLUDING the gesture-bar inset it now
		   carries, otherwise the last card hides behind it on a phone. */
		padding-bottom: calc(6rem + env(safe-area-inset-bottom, 0px));
	}

	.nav {
		position: fixed;
		bottom: 0;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		/* Liquid-glass pilot (#644/#645): the same translucent-blur surface the
		   chart tooltip already uses, not a new pattern. A decorative gradient
		   behind it was tried and dropped (see app.css) — real content (cards,
		   chips, chart colour) scrolling underneath gives the glass plenty to
		   refract without a synthetic backdrop. */
		background: var(--surface-glass);
		/* Was 20px — over a chart's own colours or a bright fact-tile fill,
		   20px still let enough shape through to blur legibility rather than
		   the content behind it. Stronger blur plus --surface-glass's own
		   higher opacity (app.css) is what actually fixes that; either alone
		   wasn't enough. */
		backdrop-filter: blur(28px);
		-webkit-backdrop-filter: blur(28px);
		border-top: 1px solid color-mix(in srgb, var(--color-text) 8%, transparent);
		color: var(--color-text);
		/* iOS's home indicator and Android's gesture bar both own a strip along
		   the bottom edge and swallow touches there. Padding the bar by that
		   inset keeps its targets above the strip; the 0px fallback means
		   desktops and older phones lose nothing. */
		padding: 0.5rem 1rem calc(0.5rem + env(safe-area-inset-bottom, 0px));
		box-shadow: var(--shadow-lg);
		z-index: 10;
	}

	.nav ul {
		display: flex;
		gap: 0.25rem;
		list-style: none;
		margin: 0;
		padding: 0;
	}

	/* 44px is the smallest reliable touch target (WCAG 2.5.5, and both mobile
	   platforms' own guidelines). Enforced as a min-height rather than as
	   padding so the narrow-screen rule below can shave padding without
	   shrinking the target itself. */
	.nav a,
	.nav .logout {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
	}

	.nav a {
		color: var(--color-text);
		text-decoration: none;
		padding: 0.5rem 0.875rem;
		border-radius: 10px;
		opacity: 0.75;
	}

	.nav a[aria-current='page'] {
		background: color-mix(in srgb, var(--color-brand) 16%, transparent);
		color: color-mix(in srgb, var(--color-brand) 70%, var(--color-text));
		opacity: 1;
		font-weight: 600;
	}

	.nav .logout {
		background: color-mix(in srgb, var(--color-text) 8%, transparent);
		border: none;
		color: var(--color-text);
		border-radius: 10px;
		padding: 0.4rem 0.9rem;
		cursor: pointer;
	}

	.nav .logout:hover {
		background: color-mix(in srgb, var(--color-text) 14%, transparent);
	}

	/* Both exist on mobile too (simplest to keep one markup for both layouts)
	   but only the collapse toggle below ever switches between them — the
	   bottom bar has no collapsed state, so -short stays unused there. */
	.nav-label-short {
		display: none;
	}

	/* No collapse concept on the bottom bar — hidden until the sidebar
	   breakpoint below gives it something to do. */
	.sidebar-toggle {
		display: none;
	}

	/* Four nav items plus the logout button don't fit a ~390px phone at full
	   padding — the button ran off the right edge. Tighten rather than wrap: a
	   bottom bar that grows to two rows is worse than a snug one. */
	@media (max-width: 430px) {
		.nav {
			padding: 0.25rem 0.5rem calc(0.25rem + env(safe-area-inset-bottom, 0px));
		}

		.nav a,
		.nav .logout {
			font-size: var(--text-sm);
			padding: 0.5rem;
		}
	}

	/* Desktop: sidebar instead of bottom bar */
	@media (min-width: 768px) {
		.app-shell {
			flex-direction: row;
		}

		.nav {
			position: sticky;
			top: 0;
			left: 0;
			right: auto;
			bottom: auto;
			flex-direction: column;
			align-items: stretch;
			width: 14rem;
			height: 100vh;
			padding: 1rem;
			/* A hairline meant for the top edge of a bottom bar would sit oddly
			   on a full-height sidebar's left edge — move it to the trailing
			   edge instead. */
			border-top: none;
			border-right: 1px solid color-mix(in srgb, var(--color-text) 8%, transparent);
			/* Collapsible (Nocturne v3-style motion — respects the app-wide
			   reduced-motion blanket in app.css automatically, since that just
			   zeroes every transition-duration rather than needing an opt-out
			   here). */
			transition: width var(--dur-base, 260ms) var(--ease-out-soft, ease);
		}

		.nav.collapsed {
			width: 4.5rem;
		}

		.nav ul {
			flex-direction: column;
			margin-bottom: 1rem;
		}

		.content {
			padding-bottom: 1rem;
			/* Mobile-first meant .content simply had no cap — harmless on a phone,
			   but with a real sidebar layout now in play a wide desktop screen
			   stretched paragraphs and grids across 1000px+ of unbroken width.
			   Charts (LineChart's own max-width:800px) already stopped short of
			   that on their own; this brings everything else in line without
			   touching each page. Comfortably wider than any single page's own
			   content ever needs, so nothing here changes below this cap. */
			max-width: 64rem;
			margin-inline: auto;
		}

		.sidebar-toggle {
			display: flex;
			align-items: center;
			justify-content: center;
			align-self: flex-end;
			width: 2rem;
			height: 2rem;
			margin-bottom: 0.75rem;
			border: none;
			border-radius: var(--radius-pill, 999px);
			background: color-mix(in srgb, var(--color-text) 8%, transparent);
			color: var(--color-text);
			font-size: var(--text-sm, 14px);
			cursor: pointer;
		}

		.sidebar-toggle:hover {
			background: color-mix(in srgb, var(--color-text) 14%, transparent);
		}

		/* Collapsed: the toggle centres itself (nothing to right-align against
		   any more), and every nav item swaps its full label for the short one
		   set in +layout.svelte's navItems. */
		.nav.collapsed .sidebar-toggle {
			align-self: center;
		}

		.nav.collapsed .nav-label-full {
			display: none;
		}

		.nav.collapsed .nav-label-short {
			display: inline;
			font-weight: 700;
		}

		.nav.collapsed .logout {
			padding: 0.4rem;
		}
	}
</style>

<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { whoAmI, logout } from '$lib/webauthn';

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

	const navItems = [
		{ href: resolve('/(app)'), label: 'Start' },
		{ href: resolve('/(app)/rides'), label: 'Fahrten' },
		{ href: resolve('/(app)/upload'), label: 'Upload' },
		{ href: resolve('/(app)/profile'), label: 'Profil' }
	];
</script>

{#if authState === 'checking'}
	<p>Lädt…</p>
{:else if authState === 'authed'}
	<div class="app-shell">
		<nav class="nav">
			<ul>
				{#each navItems as item (item.href)}
					<li>
						<a href={item.href} aria-current={page.url.pathname === item.href ? 'page' : undefined}>
							{item.label}
						</a>
					</li>
				{/each}
			</ul>
			<button class="logout" onclick={handleLogout}>Abmelden</button>
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
		/* Liquid-glass pilot (#644): the same translucent-blur surface the chart
		   tooltip already uses, not a new pattern. Content scrolling underneath
		   shows through, and the two body-level gradient glows (app.css) give it
		   something to visibly refract on pages that are otherwise flat. */
		background: var(--surface-glass);
		backdrop-filter: blur(20px);
		-webkit-backdrop-filter: blur(20px);
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
		}

		.nav ul {
			flex-direction: column;
			margin-bottom: 1rem;
		}

		.content {
			padding-bottom: 1rem;
		}
	}
</style>

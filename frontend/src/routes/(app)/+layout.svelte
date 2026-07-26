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
		padding-bottom: 5rem;
	}

	.nav {
		position: fixed;
		bottom: 0;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		/* Same deep wash as the ride hero, in both schemes. It used to be
		   --color-brand with white text, which breaks in the dark scheme where
		   brand teal is light: white on light teal fails contrast. */
		background: var(--color-hero-bg);
		color: var(--color-hero-text);
		padding: 0.5rem 1rem;
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

	.nav a {
		display: block;
		color: var(--color-hero-text);
		text-decoration: none;
		padding: 0.5rem 0.875rem;
		border-radius: 10px;
		opacity: 0.85;
	}

	.nav a[aria-current='page'] {
		background: rgba(255, 255, 255, 0.18);
		opacity: 1;
		font-weight: 600;
	}

	.nav .logout {
		background: rgba(255, 255, 255, 0.12);
		border: none;
		color: var(--color-hero-text);
		border-radius: 10px;
		padding: 0.4rem 0.9rem;
		cursor: pointer;
	}

	.nav .logout:hover {
		background: rgba(255, 255, 255, 0.22);
	}

	/* Four nav items plus the logout button don't fit a ~390px phone at full
	   padding — the button ran off the right edge. Tighten rather than wrap: a
	   bottom bar that grows to two rows is worse than a snug one. */
	@media (max-width: 430px) {
		.nav {
			padding: 0.5rem;
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

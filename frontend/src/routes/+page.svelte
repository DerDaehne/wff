<script lang="ts">
	import { onMount } from 'svelte';
	import { whoAmI, logout } from '$lib/webauthn';

	let me: { user_id: number } | null | undefined = $state(undefined);

	onMount(async () => {
		me = await whoAmI();
	});

	async function handleLogout() {
		await logout();
		me = null;
	}
</script>

<h1>WFF — wir fahren Fahrrad</h1>
<p>Self-hosted Radsport-Trainings-Tracker &amp; -Coach.</p>

{#if me === undefined}
	<p>Lädt…</p>
{:else if me === null}
	<p><a href="/login">Anmelden</a></p>
{:else}
	<p>Angemeldet (User #{me.user_id}).</p>
	<button onclick={handleLogout}>Abmelden</button>
{/if}

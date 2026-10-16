<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { registerPasskey, friendlyAuthError } from '$lib/webauthn';

	let status: 'idle' | 'working' | 'error' = $state('idle');
	let errorMessage = $state('');

	async function register() {
		status = 'working';
		errorMessage = '';
		try {
			await registerPasskey(page.params.token as string);
			await goto(resolve('/(app)'));
		} catch (err) {
			status = 'error';
			errorMessage = friendlyAuthError(err);
		}
	}
</script>

<!-- Rekta: no card; a kicker, the one title, a sentence and the action. -->
<main class="rk-place auth reveal" style="--i: 0">
	<div class="box">
		<p class="rk-kicker">Einladung</p>
		<h1 class="rk-title">Willkommen bei WFF</h1>
		<p>Richte einen Passkey ein, um dein Konto zu aktivieren.</p>
		<p class="rk-actions">
			<button class="rk-btn rk-btn--primary" onclick={register} disabled={status === 'working'}>
				{status === 'working' ? 'Warte auf Passkey…' : 'Passkey einrichten'}
			</button>
		</p>
		{#if status === 'error'}
			<p class="rk-empty" data-status="error" role="alert">{errorMessage}</p>
		{/if}
		<p class="rk-facts"><a href={resolve('/login')}>Schon registriert? Hier anmelden.</a></p>
	</div>
</main>

<style>
	.auth {
		display: grid;
		place-items: center;
		min-height: 100dvh;
		padding: var(--rk-space-m);
	}

	.box {
		display: grid;
		gap: var(--rk-space-m);
		width: min(100%, 22rem);
	}

	.rk-title {
		margin-top: calc(-1 * var(--rk-space-s));
	}
</style>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { loginWithPasskey, friendlyAuthError } from '$lib/webauthn';

	let username = $state('');
	let status: 'idle' | 'working' | 'error' = $state('idle');
	let errorMessage = $state('');

	async function login(e: SubmitEvent) {
		e.preventDefault();
		status = 'working';
		errorMessage = '';
		try {
			await loginWithPasskey(username);
			await goto(resolve('/(app)'));
		} catch (err) {
			status = 'error';
			errorMessage = friendlyAuthError(err);
		}
	}
</script>

<!-- Rekta: no card around a form; a kicker, the one title, the field and the
     primary action on the ground. -->
<main class="rk-place auth">
	<form onsubmit={login}>
		<p class="rk-kicker">WFF — wir fahren Fahrrad</p>
		<h1 class="rk-title">Anmelden</h1>
		<div class="rk-field" data-status={status === 'error' ? 'error' : undefined}>
			<label class="rk-label" for="username">Benutzername</label>
			<input
				id="username"
				name="username"
				class="rk-input"
				bind:value={username}
				autocomplete="username webauthn"
				required
			/>
			{#if status === 'error'}<p class="rk-hint" role="alert">{errorMessage}</p>{/if}
		</div>
		<p class="rk-actions">
			<button class="rk-btn rk-btn--primary" type="submit" disabled={status === 'working'}>
				{status === 'working' ? 'Warte auf Passkey…' : 'Mit Passkey anmelden'}
			</button>
		</p>
	</form>
</main>

<style>
	.auth {
		display: grid;
		place-items: center;
		min-height: 100dvh;
		padding: var(--rk-space-m);
	}

	form {
		display: grid;
		gap: var(--rk-space-m);
		width: min(100%, 22rem);
	}

	.rk-title {
		margin-top: calc(-1 * var(--rk-space-s));
	}
</style>

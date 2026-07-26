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

<div class="auth-page">
	<form class="card auth-card" onsubmit={login}>
		<h1>Anmelden</h1>
		<label for="username">Benutzername</label>
		<input
			id="username"
			name="username"
			class="input"
			bind:value={username}
			autocomplete="username webauthn"
			required
		/>
		<button class="btn btn-primary" type="submit" disabled={status === 'working'}>
			{status === 'working' ? 'Warte auf Passkey…' : 'Mit Passkey anmelden'}
		</button>

		{#if status === 'error'}
			<p class="error" role="alert">{errorMessage}</p>
		{/if}
	</form>
</div>

<style>
	.auth-page {
		display: flex;
		justify-content: center;
		padding-top: 15vh;
	}

	.auth-card {
		width: 100%;
		max-width: 22rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.error {
		color: var(--color-danger);
		font-size: var(--text-sm);
	}
</style>

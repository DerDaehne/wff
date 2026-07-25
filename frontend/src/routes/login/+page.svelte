<script lang="ts">
	import { goto } from '$app/navigation';
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
			await goto('/');
		} catch (err) {
			status = 'error';
			errorMessage = friendlyAuthError(err);
		}
	}
</script>

<h1>Anmelden</h1>

<form onsubmit={login}>
	<label for="username">Benutzername</label>
	<input id="username" name="username" bind:value={username} autocomplete="username webauthn" required />
	<button type="submit" disabled={status === 'working'}>
		{status === 'working' ? 'Warte auf Passkey…' : 'Mit Passkey anmelden'}
	</button>
</form>

{#if status === 'error'}
	<p role="alert">{errorMessage}</p>
{/if}

<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { registerPasskey, friendlyAuthError } from '$lib/webauthn';

	let status: 'idle' | 'working' | 'error' = $state('idle');
	let errorMessage = $state('');

	async function register() {
		status = 'working';
		errorMessage = '';
		try {
			await registerPasskey(page.params.token as string);
			await goto('/');
		} catch (err) {
			status = 'error';
			errorMessage = friendlyAuthError(err);
		}
	}
</script>

<h1>Willkommen bei WFF</h1>
<p>Richte einen Passkey ein, um dein Konto zu aktivieren.</p>

<button onclick={register} disabled={status === 'working'}>
	{status === 'working' ? 'Warte auf Passkey…' : 'Passkey einrichten'}
</button>

{#if status === 'error'}
	<p role="alert">{errorMessage}</p>
{/if}

<p><a href="/login">Schon registriert? Hier anmelden.</a></p>

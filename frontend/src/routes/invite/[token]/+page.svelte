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

<div class="auth-page">
	<div class="card auth-card">
		<h1>Willkommen bei WFF</h1>
		<p>Richte einen Passkey ein, um dein Konto zu aktivieren.</p>

		<button class="btn btn-primary" onclick={register} disabled={status === 'working'}>
			{status === 'working' ? 'Warte auf Passkey…' : 'Passkey einrichten'}
		</button>

		{#if status === 'error'}
			<p class="error" role="alert">{errorMessage}</p>
		{/if}

		<p class="alt-link"><a href={resolve('/login')}>Schon registriert? Hier anmelden.</a></p>
	</div>
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

	.alt-link {
		font-size: var(--text-sm);
		color: var(--color-text-muted);
	}
</style>

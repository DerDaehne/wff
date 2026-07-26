<script lang="ts">
	import { onMount } from 'svelte';
	import { getSettings, updateSettings } from '$lib/profile';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'ready' | 'error' = $state('loading');
	let errorMessage = $state('');
	let ftpWatts: number | '' = $state('');
	let lthrBpm: number | '' = $state('');
	let saveState: 'idle' | 'saving' | 'saved' | 'error' = $state('idle');

	onMount(async () => {
		try {
			const settings = await getSettings();
			ftpWatts = settings.ftp_watts ?? '';
			lthrBpm = settings.lthr_bpm ?? '';
			viewState = 'ready';
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Einstellungen konnten nicht geladen werden.';
			viewState = 'error';
		}
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		saveState = 'saving';
		try {
			await updateSettings({
				ftp_watts: ftpWatts === '' ? null : ftpWatts,
				lthr_bpm: lthrBpm === '' ? null : lthrBpm
			});
			saveState = 'saved';
		} catch {
			saveState = 'error';
		}
	}
</script>

<h1>Profil</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<div class="card">
		<p class="hint">
			FTP (Functional Threshold Power) oder LTHR (Lactate Threshold Heart Rate) werden gebraucht, um
			aus deinen Fahrten eine Trainingsbelastung (TSS) zu berechnen — ohne mindestens eines von
			beiden bleibt das Dashboard leer, auch wenn schon Fahrten hochgeladen sind.
		</p>
		<form onsubmit={save}>
			<label for="ftp">FTP (Watt)</label>
			<input id="ftp" class="input" type="number" min="1" bind:value={ftpWatts} />

			<label for="lthr">LTHR (bpm)</label>
			<input id="lthr" class="input" type="number" min="1" bind:value={lthrBpm} />

			<button class="btn btn-primary" type="submit" disabled={saveState === 'saving'}>
				{saveState === 'saving' ? 'Speichert…' : 'Speichern'}
			</button>

			{#if saveState === 'saved'}
				<p class="status-ok" role="status">
					Gespeichert. Bestehende Fahrten ohne Trainingsbelastung wurden neu berechnet.
				</p>
			{:else if saveState === 'error'}
				<p class="error" role="alert">Einstellungen konnten nicht gespeichert werden.</p>
			{/if}
		</form>
	</div>
{/if}

<style>
	.card {
		max-width: 24rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.status-ok {
		color: var(--color-success);
		font-size: var(--text-sm);
	}

	.error {
		color: var(--color-danger);
		font-size: var(--text-sm);
	}
</style>

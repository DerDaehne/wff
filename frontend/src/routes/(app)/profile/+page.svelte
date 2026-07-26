<script lang="ts">
	import { onMount } from 'svelte';
	import { getSettings, updateSettings, type Estimate, type Estimates } from '$lib/profile';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'ready' | 'error' = $state('loading');
	let errorMessage = $state('');
	let ftpWatts: number | '' = $state('');
	let lthrBpm: number | '' = $state('');
	let estimates: Estimates = $state({ ftp_watts: null, lthr_bpm: null });
	let saveState: 'idle' | 'saving' | 'saved' | 'error' = $state('idle');

	onMount(async () => {
		try {
			const settings = await getSettings();
			ftpWatts = settings.ftp_watts ?? '';
			lthrBpm = settings.lthr_bpm ?? '';
			estimates = settings.estimates;
			viewState = 'ready';
		} catch (err) {
			errorMessage =
				err instanceof ApiError ? err.message : 'Einstellungen konnten nicht geladen werden.';
			viewState = 'error';
		}
	});

	function rideDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}

	// Only offer what the field doesn't already say — repeating the value the
	// rider just typed back at them as a "suggestion" is noise.
	function offer(estimate: Estimate | null, current: number | ''): boolean {
		return estimate !== null && estimate.value !== current;
	}

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
			Diese beiden Werte sagen der App, wie hart eine Fahrt <em>für dich</em> war. Ohne mindestens einen
			davon kann sie deine Trainingsbelastung nicht berechnen und die Startseite bleibt leer, auch wenn
			schon Fahrten hochgeladen sind.
		</p>
		<form onsubmit={save}>
			<div class="field">
				<label for="ftp">Schwellenleistung (FTP) in Watt</label>
				<p class="field-hint">
					Die Leistung, die du ungefähr eine Stunde am Stück treten kannst. Braucht einen
					Leistungsmesser am Rad.
				</p>
				<input id="ftp" class="input" type="number" min="1" bind:value={ftpWatts} />
				{#if offer(estimates.ftp_watts, ftpWatts)}
					<div class="suggestion">
						<p>Aus deinen Fahrten geschätzt: <strong>{estimates.ftp_watts?.value} W</strong></p>
						<p class="suggestion-source">
							Deine besten 20 Minuten am Stück: {estimates.ftp_watts?.best_20min} W, gefahren am
							{rideDate(estimates.ftp_watts?.ridden_at ?? '')}. Davon 95 % — so wird die
							Schwellenleistung üblicherweise bestimmt, ohne Labor.
						</p>
						<button
							class="btn btn-secondary"
							type="button"
							onclick={() => (ftpWatts = estimates.ftp_watts?.value ?? '')}
						>
							Schätzung übernehmen
						</button>
					</div>
				{/if}
			</div>

			<div class="field">
				<label for="lthr">Schwellenpuls (LTHR) in Schlägen pro Minute</label>
				<p class="field-hint">
					Der Puls, den du ungefähr eine Stunde am Stück halten kannst. Braucht einen Pulsgurt oder
					eine Uhr, die den Puls aufzeichnet.
				</p>
				<input id="lthr" class="input" type="number" min="1" bind:value={lthrBpm} />
				{#if offer(estimates.lthr_bpm, lthrBpm)}
					<div class="suggestion">
						<p>Aus deinen Fahrten geschätzt: <strong>{estimates.lthr_bpm?.value} bpm</strong></p>
						<p class="suggestion-source">
							Dein höchster Durchschnittspuls über 20 Minuten: {estimates.lthr_bpm?.best_20min} bpm, gefahren
							am {rideDate(estimates.lthr_bpm?.ridden_at ?? '')}.
						</p>
						<button
							class="btn btn-secondary"
							type="button"
							onclick={() => (lthrBpm = estimates.lthr_bpm?.value ?? '')}
						>
							Schätzung übernehmen
						</button>
					</div>
				{/if}
			</div>

			{#if !estimates.ftp_watts && !estimates.lthr_bpm}
				<div class="suggestion">
					<p><strong>Du kennst deine Werte nicht? Dann fahr sie einfach ein.</strong></p>
					<p class="suggestion-source">
						Such dir eine Strecke, auf der du 20 Minuten am Stück fahren kannst, ohne anhalten zu
						müssen — leicht ansteigend ist ideal, weil dein Tempo dann von allein gleichmäßig
						bleibt. Fahr so schnell, wie du es gerade noch 20 Minuten durchhältst: am Ende soll es
						richtig wehtun, aber du sollst nicht schon vorher einbrechen. Danach lädst du die Fahrt
						hoch wie immer — den Rest rechnet die App aus und schlägt dir die Werte hier vor.
					</p>
				</div>
			{/if}

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
		max-width: 34rem;
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
		gap: 1.5rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.field-hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0 0 0.25rem;
	}

	/* An estimate is an offer, never an automatic overwrite: it sits next to
	   its field with its provenance and needs a deliberate click (#608). */
	.suggestion {
		margin-top: 0.5rem;
		padding: 0.875rem 1rem;
		border-radius: var(--radius-sm);
		background: color-mix(in srgb, var(--color-brand) var(--wash-strength), var(--wash-base));
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.5rem;
	}

	.suggestion p {
		margin: 0;
	}

	.suggestion-source {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
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

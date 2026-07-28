<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		getSettings,
		updateSettings,
		type Estimate,
		type Estimates,
		type Gap,
		type ObservedMaxHR
	} from '$lib/profile';
	import { ApiError } from '$lib/api';
	import { progressMetrics } from '$lib/progress';

	let viewState: 'loading' | 'ready' | 'error' = $state('loading');
	let errorMessage = $state('');
	let ftpWatts: number | '' = $state('');
	let lthrBpm: number | '' = $state('');
	let weightKg: number | '' = $state('');
	let birthYear: number | '' = $state('');
	let sex = $state('');
	let primaryMetric = $state('distance');
	let estimates: Estimates = $state({ ftp_watts: null, lthr_bpm: null });
	let gaps: Gap[] = $state([]);
	let observedMaxHR: ObservedMaxHR | null = $state(null);
	let saveState: 'idle' | 'saving' | 'saved' | 'error' = $state('idle');

	onMount(async () => {
		try {
			const settings = await getSettings();
			ftpWatts = settings.ftp_watts ?? '';
			lthrBpm = settings.lthr_bpm ?? '';
			weightKg = settings.weight_kg ?? '';
			birthYear = settings.birth_year ?? '';
			sex = settings.sex ?? '';
			primaryMetric = settings.primary_metric ?? 'distance';
			estimates = settings.estimates;
			gaps = settings.gaps;
			observedMaxHR = settings.observed_max_hr;
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
				lthr_bpm: lthrBpm === '' ? null : lthrBpm,
				weight_kg: weightKg === '' ? null : weightKg,
				birth_year: birthYear === '' ? null : birthYear,
				sex: sex === '' ? null : sex,
				primary_metric: primaryMetric
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
				{#if observedMaxHR}
					<p class="observed-max">
						Bisher härtester aufgezeichneter Puls: <strong>{observedMaxHR.bpm} bpm</strong>, am
						{rideDate(observedMaxHR.ridden_at)}. Das ist eine Beobachtung, keine Messung — solange
						du keinen Schwellenpuls einträgst, nutzt die App diesen Wert ersatzweise für
						Pulsbereiche, sofern er nach einer echten Ausbelastung aussieht.
					</p>
				{/if}
			</div>

			<div class="field">
				<label for="primary-metric">Wichtigste Zahl</label>
				<p class="field-hint">
					Die Zahl, die bei jeder Fahrt zuerst und am größten steht. Wenn eine Fahrt sie nicht
					hergibt — etwa Höhenmeter ohne Barometer — rückt die nächste nach.
				</p>
				<select id="primary-metric" class="input" bind:value={primaryMetric}>
					<option value="distance">Distanz</option>
					{#each progressMetrics.filter((m) => m.setting !== 'distance') as m (m.setting)}
						<option value={m.setting}>{m.label}</option>
					{/each}
					<option value="load">Belastung</option>
				</select>
			</div>

			<div class="field">
				<label for="weight">Körpergewicht in Kilogramm</label>
				<p class="field-hint">
					Optional. Damit kann die App aus deinem Tempo am Berg überschlagen, wie viel Kraft du
					dabei getreten hast — auch ganz ohne Leistungsmesser.
				</p>
				<input
					id="weight"
					class="input"
					type="number"
					min="20"
					max="300"
					step="0.5"
					bind:value={weightKg}
				/>
			</div>

			<div class="field">
				<label for="birth-year">Geburtsjahr</label>
				<p class="field-hint">
					Optional. Zusammen mit dem Gewicht reicht das, um aus deinem Puls den Kalorienverbrauch
					einer Fahrt zu schätzen. Das Jahr statt des Alters, damit auch alte Fahrten mit dem Alter
					von damals gerechnet werden.
				</p>
				<input
					id="birth-year"
					class="input"
					type="number"
					min="1900"
					max={new Date().getFullYear() - 10}
					bind:value={birthYear}
				/>
			</div>

			<div class="field">
				<label for="sex">Variante der Kalorienformel</label>
				<p class="field-hint">
					Optional, und nur hierfür. Die zugrunde liegende Studie (Keytel u. a., 2005) hat genau
					zwei Sätze von Koeffizienten veröffentlicht — deshalb gibt es nur diese zwei
					Möglichkeiten. Ohne Angabe zeigt die App keinen Kalorienwert an, statt einen der beiden zu
					raten.
				</p>
				<select id="sex" class="input" bind:value={sex}>
					<option value="">Keine Angabe</option>
					<option value="male">Männlich</option>
					<option value="female">Weiblich</option>
				</select>
			</div>

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

	{#if gaps.length > 0}
		<section class="gaps">
			<h2>Was die App noch nicht über dich weiß</h2>
			<p class="hint">
				Kein Test-Modus, kein Knopf während der Fahrt — die Fahrt normal hochladen genügt, den Rest
				rechnet die App aus.
			</p>
			{#each gaps as gap (gap.key)}
				<article class="gap">
					<p class="gap-unlocks">{gap.unlocks}</p>
					<p class="gap-instruction">{gap.instruction}</p>
				</article>
			{/each}
		</section>
	{/if}

	<section class="export">
		<h2>Deine Räder</h2>
		<p class="hint">
			Kilometerstand pro Rad und Erinnerung an den Kettenwechsel — neue Fahrten werden automatisch
			dem aktiven Rad zugeordnet.
		</p>
		<a class="btn btn-secondary" href={resolve('/(app)/raeder')}>Räder verwalten</a>
	</section>

	<section class="export">
		<h2>Deine Daten</h2>
		<p class="hint">
			Alle deine Fahrten, Profildaten und die Original-Dateien als ZIP-Archiv. Für einzelne Fahrten
			gibt es einen Download-Link direkt auf der jeweiligen Fahrt-Seite.
		</p>
		<a class="btn btn-secondary" href="/api/me/export">Alle Daten herunterladen</a>
	</section>
{/if}

<style>
	.gaps,
	.export {
		max-width: 34rem;
		margin-top: 2.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.gaps h2,
	.export h2 {
		margin: 0;
	}

	/* Deliberately not styled as a warning: these are things the rider hasn't
	   done yet, not things they got wrong. */
	.gap {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		padding: 1.25rem;
	}

	.gap-unlocks {
		margin: 0 0 0.5rem;
		font-weight: 700;
	}

	.gap-instruction {
		margin: 0;
		color: var(--color-text-muted);
		line-height: 1.5;
	}

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

	.observed-max {
		margin: 0.5rem 0 0;
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

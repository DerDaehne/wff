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
	import { createInvite } from '$lib/invites';
	import { listPasskeys, addPasskey, deletePasskey, type Passkey } from '$lib/passkeys';
	import { friendlyAuthError } from '$lib/webauthn';

	let viewState: 'loading' | 'ready' | 'error' = $state('loading');
	let errorMessage = $state('');
	let username = $state('');
	let displayName = $state('');
	let ftpWatts: number | '' = $state('');
	let lthrBpm: number | '' = $state('');
	let weightKg: number | '' = $state('');
	let birthYear: number | '' = $state('');
	let sex = $state('');
	let compareOptIn = $state(false);
	let primaryMetric = $state('distance');
	let estimates: Estimates = $state({ ftp_watts: null, lthr_bpm: null });
	let gaps: Gap[] = $state([]);
	let observedMaxHR: ObservedMaxHR | null = $state(null);
	let saveState: 'idle' | 'saving' | 'saved' | 'error' = $state('idle');

	onMount(async () => {
		try {
			const settings = await getSettings();
			username = settings.username;
			displayName = settings.display_name;
			ftpWatts = settings.ftp_watts ?? '';
			lthrBpm = settings.lthr_bpm ?? '';
			weightKg = settings.weight_kg ?? '';
			birthYear = settings.birth_year ?? '';
			sex = settings.sex ?? '';
			compareOptIn = settings.compare_opt_in ?? false;
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
		loadPasskeys();
	});

	// Passkeys (#754 follow-up): manage additional passkeys — add, list,
	// delete. Loaded independently of the settings form above so one failing
	// doesn't block the other.
	let passkeys: Passkey[] = $state([]);
	let passkeysState: 'loading' | 'ready' | 'error' = $state('loading');
	let newPasskeyName = $state('');
	let addPasskeyBusy = $state(false);
	let addPasskeyError = $state('');
	let deletingPasskeyID: number | null = $state(null);
	let deletePasskeyError = $state('');

	async function loadPasskeys() {
		try {
			passkeys = await listPasskeys();
			passkeysState = 'ready';
		} catch {
			passkeysState = 'error';
		}
	}

	function passkeyDateLabel(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}

	async function submitAddPasskey(e: SubmitEvent) {
		e.preventDefault();
		const name = newPasskeyName.trim();
		if (!name) return;
		addPasskeyBusy = true;
		addPasskeyError = '';
		try {
			const created = await addPasskey(name);
			passkeys = [...passkeys, created];
			newPasskeyName = '';
		} catch (err) {
			addPasskeyError = friendlyAuthError(err);
		} finally {
			addPasskeyBusy = false;
		}
	}

	async function removePasskey(id: number) {
		deletingPasskeyID = id;
		deletePasskeyError = '';
		try {
			await deletePasskey(id);
			passkeys = passkeys.filter((p) => p.id !== id);
		} catch (err) {
			// 409 is the one error case a rider can actually cause here (trying
			// to delete their last passkey) — worth a real translation; every
			// other case is unexpected enough that the raw message is fine.
			deletePasskeyError =
				err instanceof ApiError && err.status === 409
					? 'Das ist dein letzter Passkey — ohne ihn kämst du nicht mehr rein. Erst einen weiteren hinzufügen, dann diesen löschen.'
					: friendlyAuthError(err);
		} finally {
			deletingPasskeyID = null;
		}
	}

	// Einladung erstellen (#702): jede registrierte Person, keine Admin-Rolle.
	let inviteUsername = $state('');
	let inviteDisplayName = $state('');
	let inviteBusy = $state(false);
	let inviteError = $state('');
	let inviteLink = $state('');
	let inviteExpiresAt = $state('');
	let inviteCopied = $state(false);

	async function submitInvite(e: SubmitEvent) {
		e.preventDefault();
		inviteBusy = true;
		inviteError = '';
		inviteLink = '';
		try {
			const created = await createInvite(inviteUsername.trim(), inviteDisplayName.trim());
			inviteLink = `${window.location.origin}/invite/${created.token}`;
			inviteExpiresAt = created.expires_at;
			inviteUsername = '';
			inviteDisplayName = '';
		} catch (err) {
			// 409 (Nutzername vergeben) ist der einzige Fehlerfall, den jemand
			// beim Ausfüllen selbst auslösen kann — verdient eine echte
			// Übersetzung statt der rohen (englischen) Backend-Meldung.
			inviteError =
				err instanceof ApiError && err.status === 409
					? 'Dieser Nutzername ist schon vergeben.'
					: 'Einladung konnte nicht erstellt werden.';
		} finally {
			inviteBusy = false;
		}
	}

	async function copyInviteLink() {
		await navigator.clipboard.writeText(inviteLink);
		inviteCopied = true;
		setTimeout(() => (inviteCopied = false), 2000);
	}

	function inviteExpiryLabel(iso: string): string {
		return new Date(iso).toLocaleString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

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
				compare_opt_in: compareOptIn,
				primary_metric: primaryMetric
			});
			saveState = 'saved';
		} catch {
			saveState = 'error';
		}
	}
</script>

{#if viewState === 'ready'}
	<header class="profile-header reveal" style="--i: 0">
		<div class="avatar" aria-hidden="true">{displayName.charAt(0).toUpperCase()}</div>
		<div>
			<h1>{displayName}</h1>
			<p class="username">@{username}</p>
		</div>
	</header>
{:else}
	<h1>Profil</h1>
{/if}

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<section class="card reveal" style="--i: 1">
		<h2>Trainingswerte</h2>
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

			<div class="field">
				<label class="checkbox-label">
					<input type="checkbox" bind:checked={compareOptIn} />
					Am Trainingserfolg-Vergleich teilnehmen
				</label>
				<p class="field-hint">
					Zeigt dich und jeden anderen zugestimmten Nutzer mit einer relativen Kennzahl (wie sehr
					sich dein Trainingszustand in den letzten Wochen verändert hat) — nie absolute Kilometer
					oder Höhenmeter. Ohne Zustimmung siehst du niemanden, und niemand sieht dich.
				</p>
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
	</section>

	<section class="card passkeys reveal" style="--i: 2">
		<h2>Passkeys</h2>
		<p class="hint">
			Der einzige Weg in dieses Konto — es gibt kein Passwort und keine E-Mail-Wiederherstellung.
			Ein zweiter Passkey (etwa auf einem weiteren Gerät) ist deine Absicherung, falls das erste
			verloren geht.
		</p>

		{#if passkeysState === 'loading'}
			<p class="hint">Lädt…</p>
		{:else if passkeysState === 'error'}
			<p class="error" role="alert">Passkeys konnten nicht geladen werden.</p>
		{:else}
			<ul class="passkey-list">
				{#each passkeys as passkey (passkey.id)}
					<li class="passkey-row">
						<div>
							<p class="passkey-name">{passkey.name}</p>
							<p class="passkey-meta">
								Hinzugefügt am {passkeyDateLabel(passkey.created_at)}{#if passkey.last_used_at}
									· zuletzt genutzt am {passkeyDateLabel(passkey.last_used_at)}{/if}
							</p>
						</div>
						<button
							class="btn btn-secondary btn-danger"
							type="button"
							disabled={deletingPasskeyID === passkey.id || passkeys.length <= 1}
							title={passkeys.length <= 1
								? 'Dein letzter Passkey — erst einen weiteren hinzufügen'
								: undefined}
							onclick={() => removePasskey(passkey.id)}
						>
							{deletingPasskeyID === passkey.id ? 'Löscht…' : 'Löschen'}
						</button>
					</li>
				{/each}
			</ul>
			{#if deletePasskeyError}
				<p class="error" role="alert">{deletePasskeyError}</p>
			{/if}

			<form class="passkey-add" onsubmit={submitAddPasskey}>
				<div class="field">
					<label for="passkey-name">Neuen Passkey hinzufügen</label>
					<p class="field-hint">
						Ein Name, an dem du ihn später wiedererkennst — z. B. „iPhone“ oder „YubiKey“.
					</p>
					<div class="passkey-add-row">
						<input
							id="passkey-name"
							class="input"
							type="text"
							placeholder="z. B. iPhone"
							bind:value={newPasskeyName}
							required
						/>
						<button class="btn btn-secondary" type="submit" disabled={addPasskeyBusy}>
							{addPasskeyBusy ? 'Wartet auf Gerät…' : 'Hinzufügen'}
						</button>
					</div>
				</div>
				{#if addPasskeyError}
					<p class="error" role="alert">{addPasskeyError}</p>
				{/if}
			</form>
		{/if}
	</section>

	{#if gaps.length > 0}
		<section class="gaps reveal" style="--i: 3">
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

	{#if compareOptIn}
		<section class="export card reveal" style="--i: 4">
			<h2>Trainingserfolg-Vergleich</h2>
			<p class="hint">
				Wie sich dein Trainingszustand im Vergleich zu anderen zugestimmten Nutzern entwickelt hat.
			</p>
			<a class="btn btn-secondary" href={resolve('/(app)/vergleich')}>Vergleich ansehen</a>
		</section>
	{/if}

	<section class="export card reveal" style="--i: 5">
		<h2>Deine Räder</h2>
		<p class="hint">
			Kilometerstand pro Rad und Erinnerung an den Kettenwechsel — neue Fahrten werden automatisch
			dem aktiven Rad zugeordnet.
		</p>
		<a class="btn btn-secondary" href={resolve('/(app)/raeder')}>Räder verwalten</a>
	</section>

	<section class="export card reveal" style="--i: 6">
		<h2>Neue Person einladen</h2>
		<p class="hint">
			Jede registrierte Person kann weitere einladen — es gibt keine Admin-Rolle. Der Link ist 72
			Stunden gültig und nur einmal einlösbar.
		</p>
		<form onsubmit={submitInvite}>
			<div class="field">
				<label for="invite-username">Nutzername</label>
				<input
					id="invite-username"
					class="input"
					type="text"
					bind:value={inviteUsername}
					required
				/>
			</div>
			<div class="field">
				<label for="invite-display-name">Anzeigename</label>
				<input
					id="invite-display-name"
					class="input"
					type="text"
					bind:value={inviteDisplayName}
					required
				/>
			</div>
			<button class="btn btn-secondary" type="submit" disabled={inviteBusy}>
				{inviteBusy ? 'Erstellt…' : 'Einladung erstellen'}
			</button>
		</form>
		{#if inviteError}
			<p class="error" role="alert">{inviteError}</p>
		{/if}
		{#if inviteLink}
			<div class="invite-fresh">
				<input
					class="input"
					type="text"
					readonly
					value={inviteLink}
					onclick={(e) => e.currentTarget.select()}
				/>
				<button class="btn btn-secondary" type="button" onclick={copyInviteLink}>
					{inviteCopied ? 'Kopiert!' : 'Kopieren'}
				</button>
				<p class="hint">
					Gültig bis {inviteExpiryLabel(inviteExpiresAt)}. Nur der eingeladenen Person schicken —
					wer den Link öffnet, kann ihn einlösen.
				</p>
			</div>
		{/if}
	</section>

	<section class="export card reveal" style="--i: 7">
		<h2>Deine Daten</h2>
		<p class="hint">
			Alle deine Fahrten, Profildaten und die Original-Dateien als ZIP-Archiv. Für einzelne Fahrten
			gibt es einen Download-Link direkt auf der jeweiligen Fahrt-Seite.
		</p>
		<a class="btn btn-secondary" href="/api/me/export">Alle Daten herunterladen</a>
	</section>
{/if}

<style>
	/* Every section on this page shares the same 34rem measure and centers
	   the same way (including the h1 below) — visually identical to wrapping
	   the whole page in one centered container, without having to restructure
	   the template's several independent {#if}-gated sections to do it. */
	h1,
	.gaps,
	.export {
		max-width: 34rem;
		margin-inline: auto;
	}

	.gaps,
	.export {
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
		margin-inline: auto;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	/* .export sections all carry both classes now (#754 follow-up) — .card
	   for the box, .export for its own margin/layout — so this page reads as
	   one consistent stack of panels instead of "boxed form up top, bare
	   text-and-button rows below it". */
	.card h2 {
		margin: 0;
	}

	/* Passkeys card doesn't carry .export (it's not export-shaped: a list
	   plus an inline add-form, not a single button), so its margin-top needs
	   restating here to match the rhythm of every card below it. */
	.passkeys {
		margin-top: 2.5rem;
	}

	.profile-header {
		max-width: 34rem;
		margin: 0 auto 2rem;
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.profile-header h1 {
		margin: 0;
	}

	.avatar {
		flex-shrink: 0;
		width: 3rem;
		height: 3rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: color-mix(in srgb, var(--color-brand) 20%, transparent);
		color: var(--color-brand);
		font-weight: 700;
		font-size: var(--text-lg, 18px);
	}

	.username {
		margin: 0.125rem 0 0;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.passkey-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.passkey-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 1rem;
		background: color-mix(in srgb, var(--color-text) 4%, transparent);
		border-radius: var(--radius-sm);
	}

	.passkey-name {
		margin: 0;
		font-weight: 600;
	}

	.passkey-meta {
		margin: 0.125rem 0 0;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.btn-danger {
		background: color-mix(in srgb, var(--color-danger) 12%, transparent);
		color: var(--color-danger);
	}

	.btn-danger:not(:disabled):hover {
		background: color-mix(in srgb, var(--color-danger) 20%, transparent);
	}

	.passkey-add-row {
		display: flex;
		gap: 0.5rem;
	}

	.passkey-add-row .input {
		flex: 1;
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

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
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

	.invite-fresh {
		margin-top: 0.75rem;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
	}

	.invite-fresh .input {
		flex: 1;
		min-width: 12rem;
	}

	.invite-fresh .hint {
		flex-basis: 100%;
		margin: 0.25rem 0 0;
	}
</style>

<script lang="ts">
	import { confirm, panorama } from '$lib/rekta/rekta.js';
	import { toast } from '$lib/toast';
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

	// Rekta's question sheet before a passkey goes; keeping it is the safe answer.
	async function askRemovePasskey(id: number, name: string) {
		const ok = await confirm({
			kicker: 'Passkey löschen',
			title: `„${name}“ löschen?`,
			text: 'Mit diesem Passkey kommst du danach nicht mehr in dein Konto.',
			ok: 'Löschen',
			cancel: 'Behalten',
			safe: 'cancel'
		});
		if (ok) await removePasskey(id);
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
		toast('Einladungslink kopiert');
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
			toast('Gespeichert. Bestehende Fahrten ohne Trainingsbelastung wurden neu berechnet.');
		} catch {
			saveState = 'error';
		}
	}
</script>

{#if viewState === 'loading' || viewState === 'error'}
	<header class="head">
		<h1 class="rk-title">Profil</h1>
	</header>
	{#if viewState === 'loading'}
		<p>Lädt…</p>
	{:else}
		<p role="alert">{errorMessage}</p>
	{/if}
{:else}
	<!-- A Rekta panorama (layout.md, #1115): the profile's sections side by
	     side on a wide screen, stacked on a phone. The first one is the
	     overview: who you are, what the app is still missing, where to go
	     from here. -->
	<div class="rk-panorama" aria-labelledby="profile-title" use:panorama>
		<section class="rk-sec" aria-labelledby="profile-title">
			<header class="head">
				<p class="rk-kicker">@{username}</p>
				<h1 class="rk-title" id="profile-title">{displayName}</h1>
			</header>
			{#if gaps.length > 0}
				<!-- What the app cannot tell yet, and the ride that would change it
		     (#612): missing prerequisites, so Rekta's warning tiles. -->
				<h2 class="rk-group">Was die App noch nicht über dich weiß</h2>
				<p class="sub">
					Kein Test-Modus, kein Knopf während der Fahrt — die Fahrt normal hochladen genügt, den
					Rest rechnet die App aus.
				</p>
				<div class="rk-tiles">
					{#each gaps as gap, i (gap.key)}
						<div class="rk-tile rk-w2" data-status="warning" style="--rk-i: {i}">
							<span class="rk-tile-word">fehlt</span>
							<span class="rk-kicker">{gap.unlocks}</span>
							<span class="rk-tile-msg">{gap.instruction}</span>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Places below the profile open as tiles (Rekta: a tile that opens is a
	     link to a place). -->
			<h2 class="rk-group">Mehr</h2>
			<div class="rk-tiles">
				<a class="rk-tile rk-w2" href={resolve('/(app)/raeder')} style="--rk-i: 0">
					<span class="rk-kicker">deine Räder</span>
					<span class="rk-tile-msg"
						>Kilometerstand pro Rad und Erinnerung an den Kettenwechsel.</span
					>
				</a>
				{#if compareOptIn}
					<a class="rk-tile rk-w2" href={resolve('/(app)/vergleich')} style="--rk-i: 1">
						<span class="rk-kicker">Trainingserfolg-Vergleich</span>
						<span class="rk-tile-msg"
							>Wie sich dein Trainingszustand im Vergleich entwickelt hat.</span
						>
					</a>
				{/if}
			</div>
		</section>

		<section class="rk-sec">
			<h2 class="rk-group">Trainingswerte</h2>
			<p class="sub">
				Diese beiden Werte sagen der App, wie hart eine Fahrt <em>für dich</em> war. Ohne mindestens einen
				davon kann sie deine Trainingsbelastung nicht berechnen und die Startseite bleibt leer, auch wenn
				schon Fahrten hochgeladen sind.
			</p>
			<form class="form" onsubmit={save}>
				<div class="rk-field">
					<label class="rk-label" for="ftp">Schwellenleistung (FTP) in Watt</label>
					<input id="ftp" class="rk-input" type="number" min="1" bind:value={ftpWatts} />
					<p class="rk-hint">
						Die Leistung, die du ungefähr eine Stunde am Stück treten kannst. Braucht einen
						Leistungsmesser am Rad.
					</p>
					{#if offer(estimates.ftp_watts, ftpWatts)}
						<div class="suggest">
							<p>Aus deinen Fahrten geschätzt: <b>{estimates.ftp_watts?.value} W</b></p>
							<p class="rk-hint">
								Deine besten 20 Minuten am Stück: {estimates.ftp_watts?.best_20min} W, gefahren am
								{rideDate(estimates.ftp_watts?.ridden_at ?? '')}. Davon 95 % — so wird die
								Schwellenleistung üblicherweise bestimmt, ohne Labor.
							</p>
							<button
								class="rk-btn"
								type="button"
								onclick={() => (ftpWatts = estimates.ftp_watts?.value ?? '')}
							>
								Schätzung übernehmen
							</button>
						</div>
					{/if}
				</div>

				<div class="rk-field">
					<label class="rk-label" for="lthr">Schwellenpuls (LTHR) in Schlägen pro Minute</label>
					<input id="lthr" class="rk-input" type="number" min="1" bind:value={lthrBpm} />
					<p class="rk-hint">
						Der Puls, den du ungefähr eine Stunde am Stück halten kannst. Braucht einen Pulsgurt
						oder eine Uhr, die den Puls aufzeichnet.
					</p>
					{#if offer(estimates.lthr_bpm, lthrBpm)}
						<div class="suggest">
							<p>Aus deinen Fahrten geschätzt: <b>{estimates.lthr_bpm?.value} bpm</b></p>
							<p class="rk-hint">
								Dein höchster Durchschnittspuls über 20 Minuten: {estimates.lthr_bpm?.best_20min} bpm,
								gefahren am {rideDate(estimates.lthr_bpm?.ridden_at ?? '')}.
							</p>
							<button
								class="rk-btn"
								type="button"
								onclick={() => (lthrBpm = estimates.lthr_bpm?.value ?? '')}
							>
								Schätzung übernehmen
							</button>
						</div>
					{/if}
					{#if observedMaxHR}
						<p class="rk-hint">
							Bisher härtester aufgezeichneter Puls: <b>{observedMaxHR.bpm} bpm</b>, am
							{rideDate(observedMaxHR.ridden_at)}. Das ist eine Beobachtung, keine Messung — solange
							du keinen Schwellenpuls einträgst, nutzt die App diesen Wert ersatzweise für
							Pulsbereiche, sofern er nach einer echten Ausbelastung aussieht.
						</p>
					{/if}
				</div>

				<div class="rk-field">
					<label class="rk-label" for="primary-metric">Wichtigste Zahl</label>
					<select id="primary-metric" class="rk-input" bind:value={primaryMetric}>
						<option value="distance">Distanz</option>
						{#each progressMetrics.filter((m) => m.setting !== 'distance') as m (m.setting)}
							<option value={m.setting}>{m.label}</option>
						{/each}
						<option value="load">Belastung</option>
					</select>
					<p class="rk-hint">
						Die Zahl, die bei jeder Fahrt zuerst und am größten steht. Wenn eine Fahrt sie nicht
						hergibt — etwa Höhenmeter ohne Barometer — rückt die nächste nach.
					</p>
				</div>

				<div class="rk-field">
					<label class="rk-label" for="weight"
						>Körpergewicht in Kilogramm <span class="rk-optional">optional</span></label
					>
					<input
						id="weight"
						class="rk-input"
						type="number"
						min="20"
						max="300"
						step="0.5"
						bind:value={weightKg}
					/>
					<p class="rk-hint">
						Damit kann die App aus deinem Tempo am Berg überschlagen, wie viel Kraft du dabei
						getreten hast — auch ganz ohne Leistungsmesser.
					</p>
				</div>

				<div class="rk-field">
					<label class="rk-label" for="birth-year"
						>Geburtsjahr <span class="rk-optional">optional</span></label
					>
					<input
						id="birth-year"
						class="rk-input"
						type="number"
						min="1900"
						max={new Date().getFullYear() - 10}
						bind:value={birthYear}
					/>
					<p class="rk-hint">
						Zusammen mit dem Gewicht reicht das, um aus deinem Puls den Kalorienverbrauch einer
						Fahrt zu schätzen. Das Jahr statt des Alters, damit auch alte Fahrten mit dem Alter von
						damals gerechnet werden.
					</p>
				</div>

				<div class="rk-field">
					<label class="rk-label" for="sex"
						>Variante der Kalorienformel <span class="rk-optional">optional</span></label
					>
					<select id="sex" class="rk-input" bind:value={sex}>
						<option value="">Keine Angabe</option>
						<option value="male">Männlich</option>
						<option value="female">Weiblich</option>
					</select>
					<p class="rk-hint">
						Nur hierfür. Die zugrunde liegende Studie (Keytel u. a., 2005) hat genau zwei Sätze von
						Koeffizienten veröffentlicht — deshalb gibt es nur diese zwei Möglichkeiten. Ohne Angabe
						zeigt die App keinen Kalorienwert an, statt einen der beiden zu raten.
					</p>
				</div>

				<div class="rk-field">
					<label class="rk-toggle">
						<span class="rk-toggle-label">Am Trainingserfolg-Vergleich teilnehmen</span>
						<input type="checkbox" role="switch" bind:checked={compareOptIn} />
						<span class="rk-toggle-track" aria-hidden="true"></span>
						<span class="rk-toggle-word" data-on="an" data-off="aus" aria-hidden="true"></span>
					</label>
					<p class="rk-hint">
						Zeigt dich und jeden anderen zugestimmten Nutzer mit einer relativen Kennzahl (wie sehr
						sich dein Trainingszustand in den letzten Wochen verändert hat) — nie absolute Kilometer
						oder Höhenmeter. Ohne Zustimmung siehst du niemanden, und niemand sieht dich.
					</p>
				</div>

				<p class="rk-actions">
					<button class="rk-btn rk-btn--primary" type="submit" disabled={saveState === 'saving'}>
						{saveState === 'saving' ? 'Speichert…' : 'Speichern'}
					</button>
				</p>
				{#if saveState === 'error'}
					<p class="rk-empty" data-status="error" role="alert">
						Einstellungen konnten nicht gespeichert werden.
					</p>
				{/if}
			</form>
		</section>

		<section class="rk-sec">
			<h2 class="rk-group">Passkeys</h2>
			<p class="sub">
				Der einzige Weg in dieses Konto — es gibt kein Passwort und keine E-Mail-Wiederherstellung.
				Ein zweiter Passkey (etwa auf einem weiteren Gerät) ist deine Absicherung, falls das erste
				verloren geht.
			</p>

			{#if passkeysState === 'loading'}
				<p class="rk-hint">Lädt…</p>
			{:else if passkeysState === 'error'}
				<p class="rk-empty" data-status="error" role="alert">
					Passkeys konnten nicht geladen werden.
				</p>
			{:else}
				<ul class="rk-rows">
					{#each passkeys as passkey (passkey.id)}
						<li>
							<div>
								<b>{passkey.name}</b>
								<p class="rk-meta">
									Hinzugefügt am {passkeyDateLabel(passkey.created_at)}{#if passkey.last_used_at}
										· zuletzt genutzt am {passkeyDateLabel(passkey.last_used_at)}{/if}
								</p>
							</div>
							{#if passkeys.length <= 1}
								<!-- Rekta: a control that cannot act says why, instead of being dead. -->
								<span class="rk-hint">Dein letzter Passkey — erst einen weiteren hinzufügen</span>
							{:else}
								<button
									class="rk-btn"
									data-status="error"
									type="button"
									disabled={deletingPasskeyID === passkey.id}
									onclick={() => askRemovePasskey(passkey.id, passkey.name)}
								>
									{deletingPasskeyID === passkey.id ? 'Löscht…' : 'Löschen'}
								</button>
							{/if}
						</li>
					{/each}
				</ul>
				{#if deletePasskeyError}
					<p class="rk-empty" data-status="error" role="alert">{deletePasskeyError}</p>
				{/if}

				<form class="form" onsubmit={submitAddPasskey}>
					<div class="rk-field" data-status={addPasskeyError ? 'error' : undefined}>
						<label class="rk-label" for="passkey-name">Neuen Passkey hinzufügen</label>
						<input
							id="passkey-name"
							class="rk-input"
							type="text"
							placeholder="z. B. iPhone"
							bind:value={newPasskeyName}
							required
						/>
						<p class="rk-hint">
							{addPasskeyError ||
								'Ein Name, an dem du ihn später wiedererkennst — z. B. „iPhone“ oder „YubiKey“.'}
						</p>
					</div>
					<p class="rk-actions">
						<button class="rk-btn" type="submit" disabled={addPasskeyBusy}>
							{addPasskeyBusy ? 'Wartet auf Gerät…' : 'Hinzufügen'}
						</button>
					</p>
				</form>
			{/if}
		</section>

		<section class="rk-sec">
			<h2 class="rk-group">Neue Person einladen</h2>
			<p class="sub">
				Jede registrierte Person kann weitere einladen — es gibt keine Admin-Rolle. Der Link ist 72
				Stunden gültig und nur einmal einlösbar.
			</p>
			<form class="form" onsubmit={submitInvite}>
				<div class="rk-field">
					<label class="rk-label" for="invite-username">Nutzername</label>
					<input
						id="invite-username"
						class="rk-input"
						type="text"
						bind:value={inviteUsername}
						required
					/>
				</div>
				<div class="rk-field">
					<label class="rk-label" for="invite-display-name">Anzeigename</label>
					<input
						id="invite-display-name"
						class="rk-input"
						type="text"
						bind:value={inviteDisplayName}
						required
					/>
				</div>
				<p class="rk-actions">
					<button class="rk-btn" type="submit" disabled={inviteBusy}>
						{inviteBusy ? 'Erstellt…' : 'Einladung erstellen'}
					</button>
				</p>
			</form>
			{#if inviteError}
				<p class="rk-empty" data-status="error" role="alert">{inviteError}</p>
			{/if}
			{#if inviteLink}
				<div class="rk-field">
					<label class="rk-label" for="invite-link">Einladungslink</label>
					<input
						id="invite-link"
						class="rk-input"
						type="text"
						readonly
						value={inviteLink}
						onclick={(e) => e.currentTarget.select()}
					/>
					<p class="rk-hint">
						Gültig bis {inviteExpiryLabel(inviteExpiresAt)}. Nur der eingeladenen Person schicken —
						wer den Link öffnet, kann ihn einlösen.
					</p>
				</div>
				<p class="rk-actions">
					<button class="rk-btn rk-btn--primary" type="button" onclick={copyInviteLink}>
						Link kopieren
					</button>
				</p>
			{/if}
		</section>

		<section class="rk-sec">
			<h2 class="rk-group">Deine Daten</h2>
			<p class="sub">
				Alle deine Fahrten, Profildaten und die Original-Dateien als ZIP-Archiv. Für einzelne
				Fahrten gibt es einen Download-Link direkt auf der jeweiligen Fahrt-Seite.
			</p>
			<p class="rk-actions">
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- server download endpoint, not an app route -->
				<a class="rk-btn" href="/api/me/export">Alle Daten herunterladen</a>
			</p>
		</section>
		<span class="rk-panorama-end" aria-hidden="true"></span>
	</div>
{/if}

<style>
	.head {
		margin-bottom: var(--rk-space-l);
	}

	.head .rk-title {
		margin-top: var(--rk-space-xs);
	}

	.sub {
		max-width: 60ch;
		margin: var(--rk-space-xs) 0 var(--rk-space-m);
		color: var(--rk-text-contrast-high);
	}

	/* One column of fields at a readable measure, spaced by Rekta's steps. */
	.form {
		display: grid;
		gap: var(--rk-space-l);
		max-width: 36rem;
	}

	.rk-rows + .form {
		margin-top: var(--rk-space-l);
	}

	.suggest {
		display: grid;
		gap: var(--rk-space-xs);
		justify-items: start;
		margin-top: var(--rk-space-s);
		padding-left: var(--rk-space-m);
		border-left: 2px solid var(--rk-text-contrast-low);
	}

	.rk-rows > li {
		align-items: center;
	}

	.rk-rows .rk-hint {
		max-width: 16rem;
		text-align: right;
	}
</style>

<script lang="ts">
	import { confirm, drop } from '$lib/rekta/rekta.js';
	import { toast } from '$lib/toast';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { uploadActivity, friendlyUploadError } from '$lib/activities';
	import {
		listDeviceTokens,
		createDeviceToken,
		revokeDeviceToken,
		type DeviceToken
	} from '$lib/devicetokens';

	// A share from Android's share sheet that couldn't be accepted lands back
	// here with a reason (#617). The backend sends a key rather than a
	// sentence so the wording lives with the rest of the interface.
	const shareProblems: Record<string, string> = {
		'zu-gross': 'Die geteilte Datei war zu groß. Über 50 MB nimmt die App nicht an.',
		'keine-datei': 'Beim Teilen ist keine Datei angekommen. Versuch es noch einmal.',
		'nicht-lesbar': 'Die geteilte Datei ließ sich nicht lesen.',
		'keine-fit-datei':
			'Das war keine .fit-Datei. Radcomputer und Apps wie SIGMA RIDE exportieren sie unter genau dieser Endung.',
		'schon-vorhanden': 'Diese Fahrt ist schon da — du findest sie unter „Fahrten".',
		fehlgeschlagen: 'Die geteilte Fahrt konnte nicht gespeichert werden.'
	};

	let sharedProblem = $derived(shareProblems[page.url.searchParams.get('geteilt') ?? '']);

	// Device tokens (#617): an iPhone can't share into a web app, so the iOS
	// route is a Shortcut that posts the file itself — and it needs a key of its
	// own, because a Shortcut can't do a passkey login.
	let tokens: DeviceToken[] = $state([]);
	let tokensLoaded = $state(false);
	let newTokenName = $state('iPhone');
	let freshToken: string | null = $state(null);
	let tokenError = $state('');

	// Only fetched once the person actually opens the section — most uploads are
	// a drag-and-drop and never need this.
	async function loadTokens() {
		if (tokensLoaded) return;
		try {
			tokens = await listDeviceTokens();
			tokensLoaded = true;
		} catch {
			tokenError = 'Die Geräteschlüssel konnten nicht geladen werden.';
		}
	}

	async function addToken() {
		tokenError = '';
		try {
			const created = await createDeviceToken(newTokenName.trim() || 'Mein Handy');
			freshToken = created.token ?? null;
			tokens = [created, ...tokens];
		} catch {
			tokenError = 'Der Geräteschlüssel konnte nicht angelegt werden.';
		}
	}

	async function revoke(token: DeviceToken) {
		const ok = await confirm({
			kicker: 'Schlüssel löschen',
			title: `„${token.name}“ löschen?`,
			text: 'Das Gerät kann dann nicht mehr hochladen.',
			ok: 'Löschen',
			cancel: 'Behalten',
			safe: 'cancel'
		});
		if (!ok) return;
		tokenError = '';
		try {
			await revokeDeviceToken(token.id);
			tokens = tokens.filter((t) => t.id !== token.id);
		} catch {
			tokenError = 'Der Geräteschlüssel konnte nicht gelöscht werden.';
		}
	}

	async function copyToken() {
		if (!freshToken) return;
		await navigator.clipboard.writeText(freshToken);
		toast('Schlüssel kopiert');
	}

	function tokenDate(iso: string): string {
		return new Date(iso).toLocaleDateString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}

	let status: 'idle' | 'uploading' | 'success' | 'error' = $state('idle');
	let errorMessage = $state('');
	let activityId: number | null = $state(null);
	let fileInput: HTMLInputElement;

	async function handleFile(file: File) {
		status = 'uploading';
		errorMessage = '';
		try {
			const result = await uploadActivity(file);
			activityId = result.activityId;
			status = 'success';
		} catch (err) {
			errorMessage = friendlyUploadError(err);
			status = 'error';
		}
	}

	// rekta.js drop(): files dropped on the zone or chosen with its input arrive
	// here, sorted by the input's accept; a refused one says why.
	function dropZone(el: HTMLElement) {
		return drop(el, {
			onFiles: (accepted, refused) => {
				if (accepted[0]) handleFile(accepted[0]);
				else if (refused[0]) {
					errorMessage = `„${refused[0].name}“ ist keine .fit-Datei.`;
					status = 'error';
				}
			}
		});
	}

	function reset() {
		status = 'idle';
		errorMessage = '';
		activityId = null;
		fileInput.value = '';
	}
</script>

<h1 class="rk-title reveal" style="--i: 0">Fahrt hochladen</h1>
<p class="sub reveal" style="--i: 0">
	Zieh die <code>.fit</code>-Datei deines Radcomputers hier hinein. Auswertung, Wetter und
	Einordnung macht die App danach von allein.
</p>

{#if sharedProblem}
	<p class="rk-empty" data-status="error" role="alert">{sharedProblem}</p>
{/if}

<!-- Rekta's drop zone (rekta.js drop()): a dashed frame around the file
     input; a dragged file makes it solid, a drop or a choice uploads. -->
<label class="rk-drop reveal" style="--i: 1" use:dropZone>
	<input bind:this={fileInput} type="file" accept=".fit" disabled={status === 'uploading'} />
	{#if status === 'uploading'}
		<b>Wird hochgeladen…</b>
		<progress class="rk-progress" aria-label="Upload läuft"></progress>
	{:else}
		<b>Datei hierher ziehen</b>
		<span>oder eine auswählen</span>
	{/if}
</label>

{#if status === 'success'}
	<p class="rk-actions result" role="status">
		<span>Fahrt ist da.</span>
		<a
			class="rk-btn rk-btn--primary"
			href={resolve('/(app)/rides/[id]', { id: String(activityId) })}
		>
			Fahrt ansehen
		</a>
		<button class="rk-btn" type="button" onclick={reset}>Noch eine hochladen</button>
	</p>
{:else if status === 'error'}
	<p class="rk-empty" data-status="error" role="alert">{errorMessage}</p>
	<p class="rk-actions">
		<button class="rk-btn rk-btn--primary" type="button" onclick={reset}>Erneut versuchen</button>
	</p>
{/if}

<details class="phone reveal" style="--i: 2" ontoggle={loadTokens}>
	<summary class="rk-group">Direkt vom Handy hochladen</summary>
	<p class="rk-facts">ohne Umweg über den Rechner</p>

	<h2 class="rk-kicker">Android</h2>
	<p>
		Nichts einzurichten. Öffne die Fahrt in SIGMA RIDE, geh auf <b>Teilen</b> und wähle
		<b>WFF</b> aus der Liste. Die Fahrt landet direkt hier — vorausgesetzt, du hast WFF über
		<em>„Zum Startbildschirm hinzufügen"</em> installiert.
	</p>

	<h2 class="rk-kicker">iPhone</h2>
	<p>
		Apple lässt Web-Apps nicht in die Teilen-Liste. Der Umweg ist ein <b>Kurzbefehl</b> — einmal angelegt,
		taucht er beim Teilen genauso auf. Dafür braucht er einen eigenen Schlüssel, weil er sich nicht per
		Face ID anmelden kann.
	</p>

	<h3 class="rk-kicker">1. Schlüssel anlegen</h3>
	{#if tokenError}
		<p class="rk-empty" data-status="error" role="alert">{tokenError}</p>
	{/if}
	<div class="add">
		<div class="rk-field">
			<label class="rk-label" for="token-name">Name des Geräts</label>
			<input id="token-name" class="rk-input" bind:value={newTokenName} maxlength="60" />
		</div>
		<button class="rk-btn" type="button" onclick={addToken}>Schlüssel erzeugen</button>
	</div>

	{#if freshToken}
		<div class="rk-field fresh">
			<label class="rk-label" for="fresh-token">Jetzt kopieren</label>
			<input
				id="fresh-token"
				class="rk-input"
				readonly
				value={freshToken}
				onclick={(e) => e.currentTarget.select()}
			/>
			<p class="rk-hint">Danach zeigt ihn dir niemand mehr — auch die App nicht.</p>
		</div>
		<p class="rk-actions">
			<button class="rk-btn rk-btn--primary" type="button" onclick={copyToken}>
				Schlüssel kopieren
			</button>
		</p>
	{/if}

	{#if tokens.length > 0}
		<ul class="rk-rows tokens">
			{#each tokens as token (token.id)}
				<li>
					<div>
						<b>{token.name}</b>
						<p class="rk-meta">
							angelegt am {tokenDate(token.created_at)} ·
							{token.last_used_at
								? `zuletzt benutzt am ${tokenDate(token.last_used_at)}`
								: 'noch nie benutzt'}
						</p>
					</div>
					<button class="rk-btn" type="button" onclick={() => revoke(token)}>Löschen</button>
				</li>
			{/each}
		</ul>
	{/if}

	<h3 class="rk-kicker">2. Kurzbefehl bauen</h3>
	<ol class="recipe">
		<li>App <b>Kurzbefehle</b> öffnen, oben rechts auf <b>+</b>.</li>
		<li><b>Aktion hinzufügen</b> → nach <em>„Inhalte von URL abrufen"</em> suchen und antippen.</li>
		<li>Auf <b>URL</b> tippen und eintragen: <code>{page.url.origin}/api/activities</code></li>
		<li>
			Auf <em>„Weitere anzeigen"</em> tippen und <b>Methode</b> auf <code>POST</code> stellen.
		</li>
		<li>
			Bei <b>Header</b> ein Feld hinzufügen: Schlüssel <code>Authorization</code>, Wert
			<code>Bearer</code> + Leerzeichen + dein Schlüssel von oben.
		</li>
		<li>
			Bei <b>Anfragetext</b> die Art <b>Formular</b> wählen, ein Feld hinzufügen, dessen Typ auf
			<b>Datei</b> stellen, Schlüssel <code>file</code>, Wert <em>„Kurzbefehlseingabe"</em>.
		</li>
		<li>
			Oben auf den Namen tippen → <em>„Details"</em> → <b>„Bei Teilen-Sheet anzeigen"</b>
			einschalten, Eingabetyp <b>Dateien</b>.
		</li>
		<li>Kurzbefehl benennen, z. B. <em>„An WFF senden"</em>, und sichern.</li>
	</ol>

	<h3 class="rk-kicker">3. Benutzen</h3>
	<p>
		Fahrt in SIGMA RIDE öffnen → <b>Exportieren/Teilen</b> → <code>.fit</code> → in der Liste
		<em>„An WFF senden"</em>. Danach steht sie unter „Fahrten".
	</p>

	<p class="rk-facts">
		Der Schlüssel darf nur eines: Fahrten hochladen. Er kommt an keine Fahrt, keine Auswertung und
		kein Profil heran und kann sich auch nicht selbst verlängern. Geht das Handy verloren, löschst
		du ihn hier — ab dem Moment nimmt die App von diesem Gerät nichts mehr an.
	</p>
</details>

<style>
	h1 {
		margin-bottom: var(--rk-space-s);
	}

	.sub {
		max-width: 60ch;
		margin-bottom: var(--rk-space-l);
		color: var(--rk-text-contrast-high);
	}

	.rk-drop {
		max-width: 36rem;
	}

	.rk-drop .rk-progress {
		width: min(100%, 16rem);
	}

	.result,
	.rk-empty {
		margin-top: var(--rk-space-m);
	}

	.phone {
		max-width: 60ch;
		margin-top: var(--rk-space-xl);
		padding-top: var(--rk-space-m);
		border-top: 1px solid var(--rk-text-contrast-low);
	}

	.phone summary {
		cursor: pointer;
	}

	.phone :is(p, ol, ul, .add, .fresh) {
		margin-top: var(--rk-space-s);
	}

	.phone :is(h2, h3) {
		margin-top: var(--rk-space-l);
	}

	/* A field and its button on one line, the button on the field's baseline. */
	.add {
		display: flex;
		flex-wrap: wrap;
		align-items: end;
		gap: var(--rk-space-s);
	}

	.add .rk-field {
		flex: 1 1 12rem;
	}

	.recipe {
		padding-left: 1.25rem;
		display: grid;
		gap: var(--rk-space-xs);
	}

	code {
		font-family: var(--rk-font-mono);
		font-size: 0.9em;
	}
</style>

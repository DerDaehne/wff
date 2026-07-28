<script lang="ts">
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
	let copied = $state(false);

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
			copied = false;
			tokens = [created, ...tokens];
		} catch {
			tokenError = 'Der Geräteschlüssel konnte nicht angelegt werden.';
		}
	}

	async function revoke(token: DeviceToken) {
		if (
			!confirm(
				`Schlüssel „${token.name}" wirklich löschen? Das Gerät kann dann nicht mehr hochladen.`
			)
		)
			return;
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
		copied = true;
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
	let dragOver = $state(false);
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

	function onInputChange(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (file) handleFile(file);
	}

	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		const file = e.dataTransfer?.files?.[0];
		if (file) handleFile(file);
	}

	function onDragOver(e: DragEvent) {
		e.preventDefault();
		dragOver = true;
	}

	function onDragLeave() {
		dragOver = false;
	}

	function reset() {
		status = 'idle';
		errorMessage = '';
		activityId = null;
		fileInput.value = '';
	}
</script>

<div class="page-center">
	<h1>Fahrt hochladen</h1>
	<p class="lead">
		Zieh die <code>.fit</code>-Datei deines Radcomputers hier hinein. Auswertung, Wetter und
		Einordnung macht die App danach von allein.
	</p>

	{#if sharedProblem}
		<p class="shared-problem" role="alert">{sharedProblem}</p>
	{/if}

	<div
		class="dropzone"
		class:dragover={dragOver}
		ondrop={onDrop}
		ondragover={onDragOver}
		ondragleave={onDragLeave}
		role="button"
		tabindex="0"
		onclick={() => fileInput.click()}
		onkeydown={(e) => e.key === 'Enter' && fileInput.click()}
	>
		{#if status === 'uploading'}
			<p>Wird hochgeladen…</p>
		{:else}
			<p>Datei hierher ziehen oder klicken zum Auswählen</p>
		{/if}
		<input
			bind:this={fileInput}
			type="file"
			accept=".fit"
			hidden
			onchange={onInputChange}
			disabled={status === 'uploading'}
		/>
	</div>

	{#if status === 'success'}
		<p role="status" class="done">Fahrt ist da.</p>
		<div class="actions">
			<a class="btn btn-primary" href={resolve('/(app)/rides/[id]', { id: String(activityId) })}>
				Fahrt ansehen
			</a>
			<button class="btn btn-secondary" onclick={reset}>Noch eine hochladen</button>
		</div>
	{:else if status === 'error'}
		<p role="alert">{errorMessage}</p>
		<button class="btn btn-primary" onclick={reset}>Erneut versuchen</button>
	{/if}

	<details class="phone" ontoggle={loadTokens}>
		<summary>Direkt vom Handy hochladen — ohne Umweg über den Rechner</summary>

		<h2>Android</h2>
		<p>
			Nichts einzurichten. Öffne die Fahrt in SIGMA RIDE, geh auf <strong>Teilen</strong> und wähle
			<strong>WFF</strong> aus der Liste. Die Fahrt landet direkt hier — vorausgesetzt, du hast WFF
			über <em>„Zum Startbildschirm hinzufügen"</em> installiert.
		</p>

		<h2>iPhone</h2>
		<p>
			Apple lässt Web-Apps nicht in die Teilen-Liste. Der Umweg ist ein <strong>Kurzbefehl</strong> —
			einmal angelegt, taucht er beim Teilen genauso auf. Dafür braucht er einen eigenen Schlüssel, weil
			er sich nicht per Face ID anmelden kann.
		</p>

		<h3>1. Schlüssel anlegen</h3>
		{#if tokenError}
			<p role="alert">{tokenError}</p>
		{/if}
		<div class="token-new">
			<input class="input" bind:value={newTokenName} aria-label="Name des Geräts" maxlength="60" />
			<button class="btn btn-secondary" type="button" onclick={addToken}>Schlüssel erzeugen</button>
		</div>

		{#if freshToken}
			<div class="token-fresh">
				<p>
					<strong>Jetzt kopieren.</strong> Danach zeigt ihn dir niemand mehr — auch die App nicht.
				</p>
				<code>{freshToken}</code>
				<button class="btn btn-secondary" type="button" onclick={copyToken}>
					{copied ? 'Kopiert' : 'Kopieren'}
				</button>
			</div>
		{/if}

		{#if tokens.length > 0}
			<ul class="token-list">
				{#each tokens as token (token.id)}
					<li>
						<div>
							<strong>{token.name}</strong>
							<span class="token-meta">
								angelegt am {tokenDate(token.created_at)} ·
								{token.last_used_at
									? `zuletzt benutzt am ${tokenDate(token.last_used_at)}`
									: 'noch nie benutzt'}
							</span>
						</div>
						<button class="btn btn-secondary" type="button" onclick={() => revoke(token)}>
							Löschen
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		<h3>2. Kurzbefehl bauen</h3>
		<ol class="recipe">
			<li>App <strong>Kurzbefehle</strong> öffnen, oben rechts auf <strong>+</strong>.</li>
			<li>
				<strong>Aktion hinzufügen</strong> → nach <em>„Inhalte von URL abrufen"</em> suchen und antippen.
			</li>
			<li>
				Auf <strong>URL</strong> tippen und eintragen: <code>{page.url.origin}/api/activities</code>
			</li>
			<li>
				Auf <em>„Weitere anzeigen"</em> tippen und <strong>Methode</strong> auf <code>POST</code> stellen.
			</li>
			<li>
				Bei <strong>Header</strong> ein Feld hinzufügen: Schlüssel <code>Authorization</code>, Wert
				<code>Bearer</code> + Leerzeichen + dein Schlüssel von oben.
			</li>
			<li>
				Bei <strong>Anfragetext</strong> die Art <strong>Formular</strong> wählen, ein Feld
				hinzufügen, dessen Typ auf <strong>Datei</strong> stellen, Schlüssel <code>file</code>, Wert
				<em>„Kurzbefehlseingabe"</em>.
			</li>
			<li>
				Oben auf den Namen tippen → <em>„Details"</em> →
				<strong>„Bei Teilen-Sheet anzeigen"</strong>
				einschalten, Eingabetyp <strong>Dateien</strong>.
			</li>
			<li>Kurzbefehl benennen, z. B. <em>„An WFF senden"</em>, und sichern.</li>
		</ol>

		<h3>3. Benutzen</h3>
		<p>
			Fahrt in SIGMA RIDE öffnen → <strong>Exportieren/Teilen</strong> → <code>.fit</code> → in der
			Liste <em>„An WFF senden"</em>. Danach steht sie unter „Fahrten".
		</p>

		<p class="token-note">
			Der Schlüssel darf nur eines: Fahrten hochladen. Er kommt an keine Fahrt, keine Auswertung und
			kein Profil heran und kann sich auch nicht selbst verlängern. Geht das Handy verloren, löschst
			du ihn hier — ab dem Moment nimmt die App von diesem Gerät nichts mehr an.
		</p>
	</details>
</div>

<style>
	.lead {
		color: var(--color-text-muted);
		max-width: 55ch;
	}

	.shared-problem {
		background: color-mix(in srgb, var(--color-warning) var(--wash-strength), var(--wash-base));
		border-radius: var(--radius-sm);
		padding: 0.875rem 1rem;
		max-width: 55ch;
	}

	.done {
		color: var(--color-success);
		font-weight: 700;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
	}

	.dropzone {
		background: color-mix(in srgb, var(--color-brand) 6%, var(--color-surface));
		box-shadow: var(--shadow-md);
		border-radius: var(--radius-md);
		padding: 3rem 1rem;
		text-align: center;
		cursor: pointer;
		transition:
			box-shadow 0.15s ease,
			background 0.15s ease;
	}

	.dropzone.dragover {
		background: color-mix(in srgb, var(--color-brand) 14%, var(--color-surface));
		box-shadow: var(--shadow-lg);
	}

	.phone {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		padding: 1.25rem 1.5rem;
		margin-top: 2rem;
		max-width: 60ch;
	}

	.phone summary {
		cursor: pointer;
		font-weight: 700;
	}

	.phone h2 {
		font-size: var(--text-lg);
		margin: 1.5rem 0 0.25rem;
	}

	.phone h3 {
		font-size: var(--text-base);
		margin: 1.25rem 0 0.5rem;
	}

	.phone code {
		background: color-mix(in srgb, var(--color-brand) var(--wash-strength), var(--wash-base));
		border-radius: var(--radius-sm);
		padding: 0.1rem 0.35rem;
		/* anywhere, not break-all: only break inside the word when the line has
		   no other option, otherwise a phone chops `file` into `fil e`. */
		overflow-wrap: anywhere;
	}

	.token-new {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.token-new .input {
		flex: 1 1 12ch;
	}

	.token-fresh {
		background: color-mix(in srgb, var(--color-warning) var(--wash-strength), var(--wash-base));
		border-radius: var(--radius-sm);
		padding: 0.875rem 1rem;
		margin-top: 0.75rem;
	}

	.token-fresh code {
		display: block;
		background: var(--color-bg);
		margin: 0.5rem 0;
		padding: 0.5rem;
		/* The token has no break opportunities at all, so here break-all is the
		   only thing that keeps it inside the box on a phone. */
		word-break: break-all;
	}

	.token-list {
		list-style: none;
		padding: 0;
		margin: 1rem 0 0;
	}

	.token-list li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		border-top: 1px solid var(--color-border);
		padding: 0.6rem 0;
	}

	.token-meta {
		display: block;
		color: var(--color-text-muted);
		font-size: var(--text-sm);
	}

	.recipe li {
		margin-bottom: 0.5rem;
	}

	.token-note {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		border-top: 1px solid var(--color-border);
		padding-top: 1rem;
		margin-top: 1.5rem;
	}
</style>

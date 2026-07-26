<script lang="ts">
	import { resolve } from '$app/paths';
	import { uploadActivity, friendlyUploadError } from '$lib/activities';

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

<h1>Fahrt hochladen</h1>
<p class="lead">
	Zieh die <code>.fit</code>-Datei deines Radcomputers hier hinein. Auswertung, Wetter und
	Einordnung macht die App danach von allein.
</p>

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

<style>
	.lead {
		color: var(--color-text-muted);
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
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { sheet } from '$lib/rekta/rekta.js';

	// A detail in Rekta's sheet (dialog.rk-sheet with rekta.js sheet()): it
	// comes from the top edge, the page behind it stays put and does not
	// scroll, a tap on the shading or Escape closes it, and it leaves the way it
	// came. Opening also pushes one history entry, so the phone's back gesture
	// closes the sheet instead of leaving the app (concept-wff-bottom-sheet).
	let { title, onClose, children }: { title: string; onClose: () => void; children: Snippet } =
		$props();

	let dialog: HTMLDialogElement;

	$effect(() => {
		const behaviour = sheet(dialog);
		dialog.showModal();
		history.pushState({ sheet: true }, '');
		let closedByBack = false;
		const onPopstate = () => {
			closedByBack = true;
			dialog.close();
		};
		// Escape, the shading or the close button: take the pushed entry back
		// off, so the next back leaves the place, not an empty sheet entry.
		const onDialogClose = () => {
			if (!closedByBack) history.back();
			onClose();
		};
		window.addEventListener('popstate', onPopstate);
		dialog.addEventListener('close', onDialogClose);
		return () => {
			window.removeEventListener('popstate', onPopstate);
			dialog.removeEventListener('close', onDialogClose);
			behaviour.destroy();
		};
	});
</script>

<dialog class="rk-sheet" bind:this={dialog} aria-labelledby="sheet-title">
	<header>
		<h2 class="rk-group" id="sheet-title">{title}</h2>
		<form method="dialog"><button class="rk-btn">Schließen</button></form>
	</header>
	{@render children()}
</dialog>

<style>
	header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--rk-space-m);
	}
</style>

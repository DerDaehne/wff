<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fly, fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	let {
		open,
		title,
		onClose,
		children
	}: { open: boolean; title: string; onClose: () => void; children: Snippet } = $props();

	let sheet: HTMLDivElement | undefined = $state();
	let triggerElement: Element | null = null;

	// Closing always goes through history.back(), never a direct onClose() —
	// that's what makes the iOS swipe-back gesture close the sheet instead of
	// leaving the app: opening pushes one history entry, and back (gesture,
	// hardware button, or this call) pops it, which fires popstate below.
	function requestClose() {
		history.back();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			requestClose();
			return;
		}
		if (e.key !== 'Tab' || !sheet) return;
		// Manual focus trap: cycle Tab/Shift+Tab between the sheet's own
		// focusable elements instead of letting focus escape to the page
		// underneath.
		const focusable = sheet.querySelectorAll<HTMLElement>(
			'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])'
		);
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	$effect(() => {
		if (!open) return;
		triggerElement = document.activeElement;
		history.pushState({ sheet: true }, '');
		const handlePopstate = () => onClose();
		window.addEventListener('popstate', handlePopstate);
		return () => {
			window.removeEventListener('popstate', handlePopstate);
			(triggerElement as HTMLElement | null)?.focus?.();
		};
	});

	$effect(() => {
		if (open) sheet?.focus();
	});
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="backdrop" onclick={requestClose} transition:fade={{ duration: 200 }}></div>
	<div
		class="sheet"
		bind:this={sheet}
		role="dialog"
		aria-modal="true"
		aria-labelledby="bottom-sheet-title"
		tabindex="-1"
		onkeydown={onKeydown}
		transition:fly={{ y: '100%', duration: 300, easing: cubicOut }}
	>
		<div class="sheet-header">
			<h2 id="bottom-sheet-title">{title}</h2>
			<button class="close" type="button" onclick={requestClose} aria-label="Schließen">✕</button>
		</div>
		<div class="sheet-body">
			{@render children()}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.4);
		z-index: 20;
	}

	/* Same translucent-blur surface as the nav bar (#645) and the chart
	   tooltip — glass belongs on this kind of transient control layer, not
	   pasted on as a new visual language. */
	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 21;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--surface-glass);
		backdrop-filter: blur(20px);
		-webkit-backdrop-filter: blur(20px);
		border-top-left-radius: var(--radius-lg);
		border-top-right-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: 1.25rem 1.25rem calc(1.25rem + env(safe-area-inset-bottom, 0px));
	}

	.sheet-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.sheet-header h2 {
		margin: 0;
		font-size: var(--text-lg);
	}

	/* A chart inside the sheet body would otherwise start its own draw-in at
	   the same instant the sheet itself starts sliding up (300ms) — content
	   moving while its container is also moving reads as jittery, not lively
	   (Nocturne v3). Waiting until the sheet is most of the way open before
	   the chart starts drawing keeps the two motions from competing. */
	.sheet-body :global(.chart-ink) {
		transition-delay: 260ms;
	}

	.close {
		background: color-mix(in srgb, var(--color-text) 8%, transparent);
		border: none;
		border-radius: var(--radius-pill);
		width: 2rem;
		height: 2rem;
		color: var(--color-text);
		cursor: pointer;
		flex-shrink: 0;
	}

	.close:hover {
		background: color-mix(in srgb, var(--color-text) 14%, transparent);
	}
</style>

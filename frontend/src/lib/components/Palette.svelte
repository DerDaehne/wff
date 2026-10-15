<script lang="ts" module>
	export interface PaletteEntry {
		label: string;
		group: string;
		/** Extra words the filter matches, e.g. what a place is about. */
		keywords?: string;
		href?: string;
		run?: () => void;
	}
</script>

<script lang="ts">
	// Rekta's "go to" palette (docs/navigation.md, .rk-palette): one list to go to
	// a place or run a command. It rises from the app bar; typing filters (each
	// word must start a word of an entry or its keywords), ↑ ↓ move, Enter runs,
	// Escape first clears the text, then closes. Ctrl/Cmd+K toggles it anywhere.
	// The app bar's button opens it through popovertarget="palette".
	import { goto } from '$app/navigation';

	let { entries }: { entries: PaletteEntry[] } = $props();

	let el: HTMLDivElement;
	let input: HTMLInputElement;
	let query = $state('');
	let active = $state(0);

	let shown = $derived.by(() => {
		const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
		return entries.filter((e) => {
			const words = `${e.label} ${e.keywords ?? ''}`.toLowerCase().split(/\s+/);
			return terms.every((t) => words.some((w) => w.startsWith(t)));
		});
	});
	let groups = $derived([...new Set(shown.map((e) => e.group))]);

	function run(entry: PaletteEntry) {
		el.hidePopover();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- entries carry resolved paths
		if (entry.href) goto(entry.href);
		else entry.run?.();
	}

	function onkeydown(ev: KeyboardEvent) {
		if (ev.key === 'ArrowDown') active = Math.min(active + 1, shown.length - 1);
		else if (ev.key === 'ArrowUp') active = Math.max(active - 1, 0);
		else if (ev.key === 'Enter' && shown[active]) run(shown[active]);
		// With text in the field, Escape only clears it; the popover's own
		// Escape closes it the next time.
		else if (ev.key === 'Escape' && query) query = '';
		else return;
		ev.preventDefault();
	}

	function ontoggle(ev: ToggleEvent) {
		if (ev.newState !== 'open') return;
		query = '';
		active = 0;
		input.focus();
	}
</script>

<svelte:window
	onkeydown={(ev) => {
		if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 'k') {
			ev.preventDefault();
			el.togglePopover();
		}
	}}
/>

<div class="rk-float rk-palette" popover id="palette" bind:this={el} {ontoggle}>
	<input
		bind:this={input}
		bind:value={query}
		oninput={() => (active = 0)}
		{onkeydown}
		role="combobox"
		aria-expanded="true"
		aria-controls="palette-list"
		aria-activedescendant={shown[active] ? `palette-${active}` : undefined}
		aria-label="Gehe zu"
		placeholder="Gehe zu …"
		autocomplete="off"
	/>
	<ul id="palette-list" role="listbox" aria-label="Orte und Befehle">
		{#each groups as group (group)}
			<li role="presentation" class="rk-menu-label">{group}</li>
			{#each shown as entry, i (entry.label)}
				{#if entry.group === group}
					<!-- Keys go through the combobox above (aria-activedescendant). -->
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<li
						id="palette-{i}"
						role="option"
						aria-selected={i === active}
						class="rk-menu-item"
						onclick={() => run(entry)}
						onpointermove={() => (active = i)}
					>
						{entry.label}
					</li>
				{/if}
			{/each}
		{/each}
	</ul>
	{#if shown.length === 0}
		<p class="rk-palette-empty rk-facts">Nichts gefunden.</p>
	{/if}
</div>

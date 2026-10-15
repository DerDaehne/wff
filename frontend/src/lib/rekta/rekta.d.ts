// WFF's own types for the parts of rekta.js it uses (ADR adr-004-wff-rekta-js).
// TypeScript resolves `$lib/rekta/rekta.js` to this file, so the vendored
// script itself is never type-checked; svelte.config excludes it from the
// project too. Extend this when WFF starts using another part.

interface Handle {
	destroy(): void;
}

export function start(options?: { root?: Document | HTMLElement; restore?: boolean }): Handle;

export const turn: {
	readonly running: boolean;
	finish(): void;
	leave(place: HTMLElement, options?: { pressed?: Element | null; back?: boolean }): Promise<void>;
	enter(
		place: HTMLElement,
		options?: { back?: boolean; focus?: HTMLElement | null; arriving?: boolean }
	): Promise<void>;
};

export function pivot(
	el: HTMLElement,
	options?: { onSelect?: (index: number, tab: HTMLElement) => void }
): Handle & { select(index: number, options?: { focus?: boolean }): void };

export const keys: {
	on(priority: number, handler: (e: KeyboardEvent) => boolean | void): () => void;
	escapeIsFree(e: KeyboardEvent): boolean;
};

type Pref = 'theme' | 'density' | 'motion';
export const prefs: {
	get(name: Pref): string;
	set(name: Pref, value: string, options?: { remember?: boolean }): void;
	restore(): void;
};

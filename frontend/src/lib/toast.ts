// One Rekta toaster for the app (rekta.js toaster(), ADR adr-004-wff-rekta-js):
// the (app) layout mounts the region and hands it over here, and any view
// says what just happened with toast(text). Without an undo, Rekta adds no
// words of its own, so the text stays German.
import type { Toaster } from '$lib/rekta/rekta.js';

let current: Toaster | null = null;

export function setToaster(toaster: Toaster | null) {
	current = toaster;
}

export function toast(text: string) {
	current?.show(text);
}

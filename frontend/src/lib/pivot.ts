// A scaled-down pivot title keeps its full layout width; Rekta's own script
// writes that width to --_rk-w so the end margin takes the gap back and every
// gap reads as space-l. WFF does not load rekta.js (ADR adr-002-wff-rekta-design),
// so this does that one thing. ponytail: an internal Rekta name; if Rekta
// renames it, the gaps just widen again. Public hook requested (Rekta #1066).
export function pivotWidths(nav: HTMLElement) {
	const titles = [...nav.children] as HTMLElement[];
	const ro = new ResizeObserver(() => {
		for (const t of titles) t.style.setProperty('--_rk-w', `${t.offsetWidth}px`);
	});
	for (const t of titles) ro.observe(t);
	return { destroy: () => ro.disconnect() };
}

// WFF is a single-page app: the Go binary serves the static build with an SPA
// fallback (adapter-static) and every page loads its data in the browser. No
// server rendering, so browser-only modules load fine; rekta.js reads
// `document` as soon as it is imported (ADR adr-004-wff-rekta-js, Rekta #1069).
export const ssr = false;

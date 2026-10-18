// WFF is a single-page app: the Go binary serves the static build with an SPA
// fallback (adapter-static) and every page loads its data in the browser. No
// server rendering, so browser-only modules load fine.
export const ssr = false;

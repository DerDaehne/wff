import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		// Backend has no CORS headers by design (prod serves both from the same
		// origin via go:embed, #573) — proxy here so `pnpm dev` sees one origin
		// too, which the WebAuthn ceremony's origin check also requires.
		proxy: {
			'/auth': 'http://127.0.0.1:8080',
			'/api': 'http://127.0.0.1:8080'
		}
	},
	plugins: [
		sveltekit({
			// Root-relative (not page-relative) asset/SW paths. Relative paths
			// (SvelteKit's default) broke the PWA plugin's service-worker
			// registration URL on multi-segment routes like /invite/[token]
			// (resolved to /invite/sw.js instead of /sw.js) — verified via a
			// real browser test. We don't need portable-subpath deploys (single
			// fixed origin, see arch-wff-frontend), so absolute paths are simply
			// correct here.
			paths: { relative: false },
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({ fallback: 'index.html' }), // SPA fallback for client-side routing
			serviceWorker: { register: false } // registration handled by vite-plugin-pwa
		}),
		SvelteKitPWA({
			registerType: 'autoUpdate',
			manifest: {
				name: 'WFF — wir fahren Fahrrad',
				short_name: 'WFF',
				description: 'Self-hosted Radsport-Trainings-Tracker & -Coach',
				start_url: '/',
				scope: '/',
				display: 'standalone',
				theme_color: '#0f766e',
				background_color: '#0f766e',
				icons: [
					{ src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' }
				]
			},
			workbox: {
				globPatterns: ['**/*.{js,css,html,svg,png,ico,webmanifest}'],
				// adapter-static writes the SPA-fallback index.html *after* Vite's
				// build finishes, so it never exists for the plugin to glob or hash
				// at build time. @vite-pwa/sveltekit's kit.adapterFallback option
				// works around this by hashing .svelte-kit/output/client/_app/
				// version.json instead at closeBundle time — but that file is
				// written by a separate Vite "environment" build (client) than the
				// one whose closeBundle reads it (ssr), and per verified real-build
				// testing (nix build .#frontend failed 3/3 times with the client
				// output directory completely empty at that point, while identical
				// local builds always succeeded) that cross-environment ordering
				// isn't reliably guaranteed. The revision placeholder below gets
				// replaced with a real content hash of the built index.html by
				// scripts/patch-sw-fallback-revision.mjs, run as a separate step
				// *after* `vite build` fully exits (adapter included) — race-free
				// and, unlike a build timestamp (which broke `nix build`'s
				// reproducibility check), deterministic for the same input. See
				// arch-wff-frontend for the full investigation.
				navigateFallback: 'index.html',
				additionalManifestEntries: [{ url: 'index.html', revision: '__INDEX_HTML_REVISION__' }]
			}
		})
	]
});

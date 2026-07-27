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
				// The interface is German throughout. #615 fixed this on the HTML
				// element; the manifest declares it separately and had been left
				// at the default, so the installed app still announced itself as
				// English to the system.
				lang: 'de',
				start_url: '/',
				scope: '/',
				display: 'standalone',
				theme_color: '#0f766e',
				background_color: '#0f766e',
				icons: [
					{ src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' }
				],
				// Puts WFF in Android's share sheet, so a .fit can be sent straight
				// from SIGMA RIDE (#617). iOS does not support this at all — WebKit
				// bug 194593, open since 2019 — which is why the iPhone route is an
				// iOS Shortcut against the upload endpoint instead.
				//
				// The action is a real server route rather than something the
				// service worker intercepts: a file share is a multipart POST, and
				// catching that in the SW would mean switching the whole build to
				// injectManifest to hand-write one. WFF serves its own frontend from
				// its own Go binary, so the POST is simply handled there.
				share_target: {
					action: '/share-target',
					method: 'POST',
					enctype: 'multipart/form-data',
					params: {
						files: [
							{
								name: 'file',
								// Android matches on MIME type; .fit has no registered
								// one, so devices report it as a generic binary blob.
								// The extension is listed too because some send that.
								accept: ['application/octet-stream', '.fit']
							}
						]
					}
				}
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

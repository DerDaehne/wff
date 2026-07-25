import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
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
				globPatterns: ['**/*.{js,css,html,svg,png,ico,webmanifest}']
			},
			kit: {
				// adapter-static writes the final index.html *after* this plugin
				// builds the precache manifest, so without this the SW never
				// caches the SPA shell — offline navigation to "/" would 404.
				// adapterFallback must match adapter({ fallback: ... }) above.
				// See arch-wff-frontend for how this was actually verified.
				adapterFallback: 'index.html',
				spa: true
			}
		})
	]
});

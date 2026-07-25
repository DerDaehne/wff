# WFF Frontend (SvelteKit-PWA)

Installierbare Progressive Web App: Login (Passkey), Upload, Dashboard
(Form/Load), Ride-Liste, Ride-Detail (Karte + Kurven), Insights.

Reine SPA (`adapter-static`) — spricht das Go-Backend über `fetch()` an,
kein eigenes SSR/API. Siehe `arch-wff-frontend` (kabai-Wissensbasis) für
Setup-Entscheidungen (PWA-Plugin, Deploy-Architektur).

```sh
pnpm install
pnpm dev              # Dev-Server (Service Worker NICHT aktiv — s. u.)
pnpm build             # statischer Production-Build nach build/
```

**Wichtig:** der Service Worker registriert sich nur im Production-Build,
nicht im Dev-Modus (`vite dev`). Installierbarkeit/Offline-Verhalten NIE
gegen `pnpm dev` prüfen.

**`pnpm preview` (`vite preview`) ist für dieses Projekt NICHT geeignet, um
den echten `adapter-static`-Build zu testen** — SvelteKit nutzt dafür intern
seinen eigenen SSR-Emulations-Server, nicht die `build/`-Ausgabe (Details in
`arch-wff-frontend`, kabai-Wissensbasis). Zum lokalen Testen von
Installierbarkeit/Offline-Verhalten stattdessen die echte `build/`-Ausgabe
mit einem simplen statischen Server servieren, z. B.:

```sh
pnpm build
pnpm dlx serve build   # oder ein beliebiger anderer statischer Fileserver
```

Über Nix bauen (aus dem Projekt-Root): `nix build .#frontend`.

Zugehörige Epics: #552 (UI/UX & PWA), #549 (Karte, fortgeschrittener Rest-Scope).

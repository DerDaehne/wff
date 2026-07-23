# WFF — wir fahren Fahrrad

Self-hosted Radsport-Trainings-Tracker & -Coach. Aufgezeichnete Workouts (`.fit`)
werden hochgeladen, analysiert und über die Zeit ausgewertet — angereichert mit
Wetter/Wind, Strecken auf einer Karte und Trainings-Insights.

## Stack

Siehe **ADR 001** in der kabai-Wissensbasis (`adr-001-wff-tech-stack`).

- **Backend:** Go (`tormoder/fit` fürs `.fit`-Parsing) + PostgreSQL
- **Frontend:** SvelteKit als installierbare **PWA**
- **Wetter/Wind:** Open-Meteo (ERA5-History, kein API-Key, self-hostbar)
- **Karte:** MapLibre GL + self-hosted Tiles (PMTiles)
- **Build/Deploy:** Nix-Flake → Docker-Container

## Nutzung

Personal-Instanz für wenige feste Nutzer (leichte Auth, Mandantentrennung über
`user_id`, keine offene Registrierung). Datenquelle im MVP: `.fit`-Upload
(z. B. Sigma ROX 11.1 → Export via Data Center/Sigma Cloud).

## Entwicklung

```sh
nix develop        # Dev-Shell mit Go, Node/pnpm, PostgreSQL
```

Struktur:

- `backend/`  — Go-API, `.fit`-Ingestion, Analyse
- `frontend/` — SvelteKit-PWA
- `docs/`     — Zeiger auf die kabai-Wissensbasis (Notes/ADRs)

## Planung

Die Projektplanung liegt im kabai-Board (Projekt **WFF**, id 13):

- Epic **#545** „Projektinitialisierung" (Übersicht) mit 8 Child-Epics.
- Canvas **„WFF — Projektplanung"** (Phasen 0–3, `blocks`-Reihenfolge).
- Wissensbasis-Einstieg: Hub `wff-hub`.

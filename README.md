# WFF — wir fahren Fahrrad

Self-hosted Radsport-Trainings-Tracker & -Coach. Aufgezeichnete Workouts (`.fit`)
werden hochgeladen, analysiert und über die Zeit ausgewertet — angereichert mit
Wetter/Wind, Strecken auf einer Karte und Trainings-Insights.

## Stack

Siehe **ADR 001** in der kabai-Wissensbasis (`adr-001-wff-tech-stack`).

- **Backend:** Go (`tormoder/fit` fürs `.fit`-Parsing) + PostgreSQL
- **Frontend:** SvelteKit als installierbare **PWA**
- **Wetter/Wind:** Open-Meteo (ERA5-History, kein API-Key, self-hostbar)
- **Karte:** MapLibre GL — aktuell öffentliche, keyless OpenFreeMap-Vektor-Tiles;
  self-hosted PMTiles bleibt späteres Scope (Epic #549)
- **Build/Deploy:** Nix-Flake → ein Docker-Container (Frontend via `go:embed`
  im Go-Binary, kein separater Frontend-Service)

## Nutzung

Personal-Instanz für wenige feste Nutzer (leichte Auth, Mandantentrennung über
`user_id`, keine offene Registrierung). Datenquelle im MVP: `.fit`-Upload
(z. B. Sigma ROX 11.1 → Export via Data Center/Sigma Cloud).

## Entwicklung

```sh
nix develop        # Dev-Shell mit Go, Node/pnpm, PostgreSQL, docker-compose
```

## Build & Deploy

Über Nix (primärer/Referenz-Build-Weg):

```sh
nix build .#backend            # statisches Go-Binary (result/bin/wff), Frontend eingebettet
nix build .#docker              # distroless OCI-Image (nur x86_64/aarch64-linux)
docker load < result            # Image lokal als wff-backend:latest verfügbar machen
```

Ohne Nix (reines `docker build`, z. B. für einen Server ohne Nix):

```sh
docker build -t wff-backend:latest .
```

Beide Wege erzeugen dasselbe Image (`wff-backend:latest`), das `docker-compose.yml` erwartet:

```sh
cp .env.example .env && docker compose up   # App + PostgreSQL lokal starten
```

Ein Container liefert sowohl die API als auch die installierbare PWA aus (kein
separater Frontend-Service in `docker-compose.yml`).

Migrationen (`backend/migrations/`) sind per `go:embed` ins Binary eingebettet
und laufen bei jedem Start automatisch (`internal/db.Migrate`, no-op wenn das
Schema schon aktuell ist) — kein separater Migrationsschritt nötig.

Erster Nutzer (keine offene Registrierung — Einladungslink erzeugen):

```sh
docker compose exec app wff invite create <username> <anzeigename>
```

Gibt einen `/invite/<token>`-Link aus (gültig `auth.InviteTTL`), über den sich
der Nutzer per Passkey registriert.

Struktur:

- `backend/`  — Go-API, `.fit`-Ingestion, Analyse
- `frontend/` — SvelteKit-PWA
- `docs/`     — Zeiger auf die kabai-Wissensbasis (Notes/ADRs)

## Planung

Die Projektplanung liegt im kabai-Board (Projekt **WFF**, id 13):

- Epic **#545** „Projektinitialisierung" (Übersicht) mit 8 Child-Epics.
- Canvas **„WFF — Projektplanung"** (Phasen 0–3, `blocks`-Reihenfolge).
- Wissensbasis-Einstieg: Hub `wff-hub`.

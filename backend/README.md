# WFF Backend (Go)

Go-API-Server: `.fit`-Ingestion, Persistenz (PostgreSQL), Auth (Passkey/WebAuthn),
Anreicherung (Open-Meteo) und Analyse (NP/IF/TSS, CTL/ATL/TSB).

Modulpfad: `github.com/DerDaehne/wff`. Umgesetzt: `/healthz` (Rauchtest fürs
Nix-Packaging, Epic #553), Auth (Epic #551 — Passkey-Registrierung/-Login,
Sessions, Invite-CLI), Ingestion/Anreicherung/Analyse (#546/#548/#550), Frontend
eingebettet via `go:embed` (Epic #552/#573) — ein Binary liefert API und PWA.

```sh
cd backend
go run .                          # PORT (default 8080) via Env steuerbar
go build .
go run . invite create <username> <display-name>   # neuen Nutzer einladen
```

`internal/webui/dist/` enthält standardmäßig nur einen Platzhalter (git-getrackt,
damit das Modul auch ohne gebautes Frontend kompiliert). Für einen lokalen Test
mit dem echten Frontend: `cd ../frontend && pnpm build && cp -r build/* ../backend/internal/webui/dist/`
— oder direkt `nix build .#backend` aus dem Projekt-Root, das erledigt das automatisch.

Migrationen (via `golang-migrate`, siehe `nix develop`):

```sh
migrate -database "$DATABASE_URL" -path migrations up
```

Auth-Tests (Passkey-Flow gegen echte Postgres-Instanz, siehe [[arch-wff-auth]]):

```sh
DATABASE_URL=postgres://wff@/wff?host=<sockdir> go test ./internal/auth/... -run TestPasskeyFlow -v
```

Über Nix bauen (aus dem Projekt-Root):

```sh
nix build .#backend      # Binary unter result/bin/wff — Frontend automatisch eingebettet
nix build .#docker       # OCI-Image, nur x86_64/aarch64-linux
docker load < result && docker compose up
```

Zugehörige Epics: #546 (Ingestion), #547 (Datenmodell, erledigt),
#548 (Anreicherung), #550 (Analyse), #551 (Auth, erledigt), #552 (UI/PWA, erledigt),
#553 (Infrastruktur, erledigt).

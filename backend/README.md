# WFF Backend (Go)

Go-API-Server: `.fit`-Ingestion, Persistenz (PostgreSQL), Anreicherung
(Open-Meteo) und Analyse (NP/IF/TSS, CTL/ATL/TSB).

Modulpfad: `codeberg.org/danszek/wff`. Aktuell nur ein `/healthz`-Endpoint
als Rauchtest für das Nix-Packaging (Epic #553); Fachlogik folgt in
#546/#547/#548/#550/#551.

```sh
cd backend
go run .                 # PORT (default 8080) via Env steuerbar
go build .
go get github.com/tormoder/fit   # sobald das Ingestion-Epic startet
```

Über Nix bauen (aus dem Projekt-Root):

```sh
nix build .#backend      # Binary unter result/bin/wff
nix build .#docker       # OCI-Image, nur x86_64/aarch64-linux
docker load < result && docker compose up
```

Zugehörige Epics: #546 (Ingestion), #547 (Datenmodell), #548 (Anreicherung),
#550 (Analyse), #551 (Auth), #553 (Infrastruktur).

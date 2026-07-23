# WFF Backend (Go)

Go-API-Server: `.fit`-Ingestion, Persistenz (PostgreSQL), Anreicherung
(Open-Meteo) und Analyse (NP/IF/TSS, CTL/ATL/TSB).

Noch nicht initialisiert. Erster Schritt (Epic #546 / #553):

```sh
cd backend
go mod init <modulpfad>   # Modulpfad festlegen
go get github.com/tormoder/fit
```

Zugehörige Epics: #546 (Ingestion), #547 (Datenmodell), #548 (Anreicherung),
#550 (Analyse), #551 (Auth), #553 (Infrastruktur).

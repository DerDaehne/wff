{
  description = "WFF — wir fahren Fahrrad: self-hosted cycling training tracker/coach";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      linuxSystems = [ "x86_64-linux" "aarch64-linux" ];
      # TimescaleDB ist in nixpkgs als unfree markiert (bündelt Timescale-License-Anteile
      # neben dem Apache-2.0-Kern) — für lokale Postgres+Timescale-Dev-Instanz nötig.
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs {
        inherit system;
        config.allowUnfree = true;
      }));
    in
    {
      # Backend-Binary (PWA eingebettet via go:embed, #573): `nix build .#backend`;
      # Frontend-PWA-Static-Build allein: `nix build .#frontend`;
      # Docker-Image (nur Linux): `nix build .#docker`
      packages = forAllSystems (pkgs:
        let
          backend = pkgs.buildGoModule {
            pname = "wff-backend";
            version = "0.1.0";
            src = ./backend;
            vendorHash = "sha256-XsHVjCASBzBMRy/cbpHuC5Vj9nfauadFgS/HGHw71FQ=";

            # internal/webui/dist ships a placeholder (so the module always
            # compiles on its own) — overwrite it with the real static build
            # before `go build` runs, so the single binary serves both API
            # and PWA (#573).
            preBuild = ''
              rm -rf internal/webui/dist
              cp -r ${frontend} internal/webui/dist
            '';
          };

          frontend = pkgs.stdenvNoCC.mkDerivation (finalAttrs: {
            pname = "wff-frontend";
            version = "0.1.0";
            src = ./frontend;

            pnpmDeps = pkgs.fetchPnpmDeps {
              inherit (finalAttrs) pname version src;
              fetcherVersion = 4;
              hash = "sha256-9P+wZqzUwULtSQqfIq+CMUOvWswHJlUTGtdR10ENJB4=";
            };

            nativeBuildInputs = [ pkgs.nodejs_22 pkgs.pnpm pkgs.pnpmConfigHook ];

            buildPhase = ''
              runHook preBuild
              pnpm build
              runHook postBuild
            '';

            installPhase = ''
              runHook preInstall
              cp -r build "$out"
              runHook postInstall
            '';
          });
        in
        { inherit backend frontend; }
        // nixpkgs.lib.optionalAttrs (builtins.elem pkgs.system linuxSystems) {
          docker = pkgs.dockerTools.buildLayeredImage {
            name = "wff-backend";
            tag = "latest";
            contents = [ backend ];
            config = {
              Cmd = [ "${backend}/bin/wff" ];
              ExposedPorts = { "8080/tcp" = { }; };
            };
          };
        });

      # Entwicklungsumgebung: `nix develop`
      devShells = forAllSystems (pkgs:
        let
          # nixpkgs' go-migrate baut standardmäßig ALLE Treiber mit (inkl. snowflake),
          # dessen CA-Cert-Init in dieser Umgebung unconditional paniced. Wir brauchen
          # nur postgres/pgx5 — schlanker und tatsächlich lauffähig.
          go-migrate-postgres = pkgs.go-migrate.overrideAttrs (_: { tags = [ "postgres" "pgx5" ]; });
          # postgresql_16 + TimescaleDB-Extension für lokale Hypertable-Entwicklung ohne Docker.
          postgresqlWithTimescale = pkgs.postgresql_16.withPackages (ps: [ ps.timescaledb ]);
        in
        {
          default = pkgs.mkShell {
            name = "wff-dev";

            packages = with pkgs; [
              # Backend (Go) — siehe ADR 001
              go
              gopls
              gotools # goimports u. a.
              go-tools # staticcheck

              # Frontend (SvelteKit / PWA)
              nodejs_22
              pnpm

              # Datenbank (lokal, mit TimescaleDB-Extension — siehe #547 / arch-wff-datenmodell)
              postgresqlWithTimescale
              go-migrate-postgres

              # Hilfsmittel
              just
              git
              docker-compose
            ];

            shellHook = ''
              echo "WFF devShell — Go $(go version | awk '{print $3}'), Node $(node --version), $(psql --version)"
              # Lokale Postgres-Instanz im Projektordner (nicht eingecheckt)
              export PGDATA="$PWD/.pgdata"
              export DATABASE_URL="postgres://wff:wff@localhost:5432/wff?sslmode=disable"
            '';
          };
        });

      formatter = forAllSystems (pkgs: pkgs.nixpkgs-fmt);
    };
}

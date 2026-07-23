{
  description = "WFF — wir fahren Fahrrad: self-hosted cycling training tracker/coach";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      linuxSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in
    {
      # Backend-Binary: `nix build .#backend`; Docker-Image (nur Linux): `nix build .#docker`
      packages = forAllSystems (pkgs:
        let
          backend = pkgs.buildGoModule {
            pname = "wff-backend";
            version = "0.1.0";
            src = ./backend;
            vendorHash = null; # aktuell nur Go-Stdlib als Dependency
          };
        in
        { inherit backend; }
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
      devShells = forAllSystems (pkgs: {
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

            # Datenbank (lokal)
            postgresql_16

            # Hilfsmittel
            just
            git
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

{
  description = "WFF — wir fahren Fahrrad: self-hosted cycling training tracker/coach";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in
    {
      # Entwicklungsumgebung: `nix develop`
      # Packaging (buildGoModule), Docker-Image und docker-compose folgen in Epic #553.
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
            nodePackages.pnpm

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

{
  description = "note-searcher dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, gomod2nix }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ gomod2nix.overlays.default ];
        };
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gomod2nix.packages.${system}.default
            gopls
            gotools
            go-tools
            govulncheck
            just
            git
          ];

          shellHook = ''
            echo ""
            echo "🔧 note-searcher dev environment"
            echo "   Go:         $(go version | awk '{print $3}')"
            echo "   gomod2nix:  $(gomod2nix --version 2>/dev/null || echo 'available')"
            echo ""
            echo "📋 just targets:"
            echo "   just build      — compile the binary"
            echo "   just test       — run tests"
            echo "   just lint       — staticcheck + govulncheck"
            echo "   just fmt        — goimports + gofmt"
            echo "   just gomod2nix  — regenerate gomod2nix.toml"
            echo ""
          '';
        };
      }
    );
}

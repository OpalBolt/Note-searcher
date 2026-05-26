{
  description = "note-searcher development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            git
            python3
            # Ensure /dev/shm is available for multiprocessing semaphores
          ];
          
          shellHook = ''
            echo "🚀 note-searcher dev environment"
            echo "   Python: $(python3 --version)"
            echo "   Git: $(git --version)"
            
            # Ensure tmpdir has proper permissions for multiprocessing
            export TMPDIR=''${TMPDIR:-/tmp}
            mkdir -p "$TMPDIR"
          '';
        };
      }
    );
}

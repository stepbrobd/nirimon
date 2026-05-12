{
  outputs = inputs: inputs.parts.lib.mkFlake { inherit inputs; } {
    systems = import inputs.systems;

    perSystem = { pkgs, system, ... }: {
      _module.args.pkgs = import inputs.nixpkgs {
        inherit system;
        overlays = [ inputs.gomod2nix.overlays.default ];
      };

      packages.default = pkgs.callPackage ./default.nix { };

      devShells.default = pkgs.callPackage ./shell.nix { };

      formatter = pkgs.callPackage ./formatter.nix { };
    };
  };

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  inputs.systems.url = "github:nix-systems/triplet";
  inputs.parts.url = "github:hercules-ci/flake-parts";
  inputs.parts.inputs.nixpkgs-lib.follows = "nixpkgs";
  inputs.utils.url = "github:numtide/flake-utils";
  inputs.utils.inputs.systems.follows = "systems";
  inputs.gomod2nix.url = "github:nix-community/gomod2nix";
  inputs.gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.gomod2nix.inputs.flake-utils.follows = "utils";
}

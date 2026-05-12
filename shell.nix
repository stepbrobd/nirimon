{ mkShell
, deno
, go
, go-tools
, gomod2nix
, gopls
, nixpkgs-fmt
}:

mkShell {
  packages = [
    deno
    go
    go-tools
    gomod2nix
    gopls
    nixpkgs-fmt
  ];
}

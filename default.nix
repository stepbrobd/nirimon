{ lib
, buildGoApplication
, nix-gitignore
}:

buildGoApplication (lib.fix (finalAttrs: {
  pname = "nirimon";
  version = lib.fileContents ./version.txt;

  src = nix-gitignore.gitignoreSource [ ] ./.;

  modules = ./gomod2nix.toml;

  ldflags = [
    "-s"
    "-w"
    "-X main.Version=${finalAttrs.version}"
  ];
  meta = {
    description = "tui monitor configuration tool for niri with visual layout, drag-and-drop, and profile management";
    homepage = "https://github.com/stepbrobd/nirimon";
    license = lib.licenses.asl20;
    mainProgram = "nirimon";
    maintainers = with lib.maintainers; [ stepbrobd ];
    platforms = lib.platforms.linux;
  };
}))

# nirimon

Binary Cache:

- Cache: <https://cache.ysun.co>
- Key: `cache.ysun.co-1:WxPYwT5g3kt9XhUhHPpNLZKI9HIOsVVAuqSHpok8Qt4=`

nirimon is a fork of [hyprmon](https://github.com/erans/hyprmon) (Eran Sandler,
Apache 2.0) that's intended to only work for
[Niri](https://github.com/niri-wm/niri) just like hyprmon is only build for
Hyprland. All Hyprland specific code paths were stripped but the profile JSON
format is preserved so all existing hyprmon profiles will still work if you copy
`~/.config/hyprmon` to `~/.config/nirimon`. Read more info over at hyprmon's
repository, the feature is basically the same.

Note that unlike hyprmon, niromon does not write to `~/.config/niri/config.kdl`
or any other persistent Niri config file. Monitor application is via
`niri msg output ...` calls, which are runtime-temporary. Niri reload (config
edit, `niri msg action load-config-file`, or restart) reverts them. Persistence
belongs to the profile json files in `~/.config/nirimon/profiles/`.

<img width="1709" height="1392" alt="nirimon tui" src="https://github.com/user-attachments/assets/0d3f8475-6afe-48a5-b981-305cfd917b81" />

<img width="1709" height="1392" alt="nirimon resolution and refresh rate menu" src="https://github.com/user-attachments/assets/4621f74f-ec57-4ca3-bc55-0036a85d9c9a" />

<img width="1709" height="1392" alt="nirimon display settings" src="https://github.com/user-attachments/assets/ab494414-282d-43cb-af1d-4e99eab575fc" />

<img width="1709" height="1392" alt="nirimon profile selection menu" src="https://github.com/user-attachments/assets/3853ca67-529e-45ad-bc07-e9145e79c944" />

<img width="1709" height="1392" alt="nirimon cli" src="https://github.com/user-attachments/assets/8e6d7616-f625-40af-a127-15e207300a8c" />

## Installation

To run nirimon in an ephemeral environment:

```sh
nix run github:stepbrobd/nirimon
```

Or for persistent installation, check how its packaged without `gomod2nix` in
[my own configuration](https://github.com/stepbrobd/inc/blob/master/pkgs/nirimon/default.nix).

Or if you are not using Nix/NixOS, build from source:

```sh
git clone --depth=1 https://github.com/stepbrobd/nirimon
pushd nirimon
go build -ldflags="-s -w -X main.Version=$(cat version.txt)"
sudo mv nirimon /usr/local/bin/
popd
```

Or if you must:

```sh
go install -ldflags="-s -w -X main.Version=0-unstable-$(date -u +%Y-%m-%d)+go" ysun.co/nirimon@latest
```

## Usage

Basically the same as hyprmon but a few features are stripped or not yet
supported by Niri.

```sh
nirimon                    # main TUI
nirimon profiles           # profile selection menu
nirimon -profile <profile> # apply a saved profile directly
nirimon -list-profiles     # list profile names (active marked with *)
nirimon -active-profile    # print the name of the currently matching profile
```

### Keyboard

Main UI:

| Key               | Action                                      |
| ----------------- | ------------------------------------------- |
| Arrow keys / hjkl | Move selected monitor by the grid step      |
| Shift + arrows    | Move by 10x grid step                       |
| Tab / Shift-Tab   | Cycle through monitors                      |
| G                 | Cycle grid size (1, 8, 16, 32, 64 px)       |
| L                 | Cycle snap mode (Off, Edges, Centers, Both) |
| R                 | Open scale picker                           |
| F                 | Open mode (resolution + refresh) picker     |
| M                 | Open mirror configuration (needs wl-mirror) |
| C / D             | Open advanced display settings dialog       |
| Enter / Space     | Toggle the selected monitor on/off          |
| A                 | Apply the current layout to niri now        |
| Z                 | Revert to previous configuration            |
| O                 | Open the profiles page                      |
| P                 | Save current layout as a named profile      |
| ?                 | Show help                                   |
| Q / Ctrl-C        | Quit                                        |

### Mouse

| Action       | Effect                       |
| ------------ | ---------------------------- |
| Left click   | Select monitor               |
| Left drag    | Move monitor (with snapping) |
| Right click  | Toggle monitor on/off        |
| Scroll wheel | Adjust monitor scale         |

### Profiles

Profiles are json files in `~/.config/nirimon/profiles/`. They store the full
monitor layout (resolution, refresh, position, scale, transform, vrr, and
EDID-derived identifiers for stable matching across port reassignments).

```sh
nirimon -profile home
nirimon -profile work
nirimon -profile docked
nirimon profiles        # interactive menu
```

### Mirroring

Niri has
[no native output mirroring](https://github.com/niri-wm/niri/wiki/Screencasting)
the way Hyprland does. nirimon keeps hyprmon's mirror picker (press `M`) and the
exact same profile schema, but applies the mirror by spawning
[wl-mirror](https://github.com/Ferdi265/wl-mirror): for a monitor set to mirror
another, nirimon launches `wl-mirror --fullscreen-output <target> <source>`,
which captures the source output and shows it fullscreen on the target.

wl-mirror must be on `PATH`. The Nix package wraps it in automatically; if you
build from source, install wl-mirror yourself. When wl-mirror is missing the
mirror picker still works and the choice is saved to the profile json, but it is
not applied (the picker says so), so a profile stays portable to hyprmon.

Because the mirror is an ordinary fullscreen Wayland window and not a
compositor-level clone, it behaves differently from Hyprland's native mirror.
Keep these gotchas in mind:

- It is just a fullscreen window on the target output. niri does not pin you to
  it: you can switch workspaces, focus other windows, or move the wl-mirror
  window away, and the target stops showing the mirror until you switch back.
- The mirror process is detached and keeps running after nirimon exits (so a
  `nirimon -profile ...` from a keybind or hotplug hook leaves a working
  mirror). nirimon tracks it in `$XDG_RUNTIME_DIR/nirimon/mirrors.json` and
  tears it down on the next apply that disables the mirror. It is also cleared
  on logout, or you can `pkill wl-mirror` by hand.
- Scaling uses wl-mirror's `fit` mode: the whole source is always shown,
  letterboxed when the aspect ratios differ. wl-mirror cannot aspect-distort to
  fill the way Hyprland does, so expect black bars on mismatched ratios instead
  of stretching.
- If the source output is unplugged or turned off, wl-mirror exits; re-apply to
  restart the mirror once the source is back.
- Only active, non-mirrored monitors can be a source, and circular mirrors are
  prevented, same as hyprmon.

### Niri

For clamshell-style switching on lid open/close, bind these to your niri
keybinds in your niri config:

```kdl
binds {
    Mod+F1 { spawn "nirimon" "-profile" "home"; }
    Mod+F2 { spawn "nirimon" "-profile" "work"; }
}
```

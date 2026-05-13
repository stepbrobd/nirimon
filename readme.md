# nirimon

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

| Key               | Action                                        |
| ----------------- | --------------------------------------------- |
| Arrow keys / hjkl | Move selected monitor by the grid step        |
| Shift + arrows    | Move by 10x grid step                         |
| Tab / Shift-Tab   | Cycle through monitors                        |
| G                 | Cycle grid size (1, 8, 16, 32, 64 px)         |
| L                 | Cycle snap mode (Off, Edges, Centers, Both)   |
| R                 | Open scale picker                             |
| F                 | Open mode (resolution + refresh) picker       |
| M                 | Open mirror configuration (no effect on niri) |
| C / D             | Open advanced display settings dialog         |
| Enter / Space     | Toggle the selected monitor on/off            |
| A                 | Apply the current layout to niri now          |
| Z                 | Revert to previous configuration              |
| O                 | Open the profiles page                        |
| P                 | Save current layout as a named profile        |
| ?                 | Show help                                     |
| Q / Ctrl-C        | Quit                                          |

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

### Niri

For clamshell-style switching on lid open/close, bind these to your niri
keybinds in your niri config:

```kdl
binds {
    Mod+F1 { spawn "nirimon" "-profile" "home"; }
    Mod+F2 { spawn "nirimon" "-profile" "work"; }
}
```

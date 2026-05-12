# nirimon

nirimon is a TUI for arranging monitors under the niri Wayland compositor. It provides a visual "desk map" where you arrange outputs with the keyboard or mouse, and applies the layout live via niri's IPC.

nirimon is a niri-only fork of [hyprmon](https://github.com/erans/hyprmon) (Eran Sandler, Apache 2.0). All hyprland-specific code paths were stripped; profile JSON is preserved byte-for-byte so existing hyprmon profiles round-trip.

## Status

Apply-only. nirimon does not write to `~/.config/niri/config.kdl` or any other persistent niri config file. Monitor application is via `niri msg output ...` calls, which are runtime-temporary: a niri reload (config edit, `niri msg action load-config-file`, or restart) reverts them. Persistence belongs to the profile json files in `~/.config/nirimon/profiles/`; re-apply on niri reload with `nirimon -profile <name>`.

## Build

```bash
go build -o nirimon .
```

Requires Go 1.26+ and a running niri compositor with `niri` on PATH.

## Usage

```bash
nirimon                       # main TUI
nirimon profiles              # profile selection menu
nirimon -profile work         # apply a saved profile directly
nirimon --list-profiles       # list profile names (active marked with *)
nirimon --active-profile      # print the name of the currently matching profile
```

## Keyboard Controls

Main UI:

| Key | Action |
|-----|--------|
| Arrow keys / hjkl | Move selected monitor by the grid step |
| Shift + arrows | Move by 10x grid step |
| Tab / Shift-Tab | Cycle through monitors |
| G | Cycle grid size (1, 8, 16, 32, 64 px) |
| L | Cycle snap mode (Off, Edges, Centers, Both) |
| R | Open scale picker |
| F | Open mode (resolution + refresh) picker |
| M | Open mirror configuration (no effect on niri) |
| C / D | Open advanced display settings dialog |
| Enter / Space | Toggle the selected monitor on/off |
| A | Apply the current layout to niri now |
| Z | Revert to previous configuration |
| O | Open the profiles page |
| P | Save current layout as a named profile |
| ? | Show help |
| Q / Ctrl-C | Quit |

There is no "save to config" keybind; nirimon does not write to niri's KDL config. Use `P` to save a profile and `A` to apply it. After a niri reload, re-apply with `nirimon -profile <name>`.

## Mouse Controls

| Action | Effect |
|--------|--------|
| Left click | Select monitor |
| Left drag | Move monitor (with snapping) |
| Right click | Toggle monitor on/off |
| Scroll wheel | Adjust monitor scale |

## Profiles

Profiles are json files in `~/.config/nirimon/profiles/`. They store the full monitor layout (resolution, refresh, position, scale, transform, vrr, and EDID-derived identifiers for stable matching across port reassignments).

```bash
nirimon -profile home
nirimon -profile work
nirimon -profile docked
nirimon profiles            # interactive menu
```

For clamshell-style switching on lid open/close, bind these to your niri keybinds in your niri config:

```kdl
binds {
    Mod+F1 { spawn "nirimon" "-profile" "home"; }
    Mod+F2 { spawn "nirimon" "-profile" "work"; }
}
```

## What is not supported under niri

- **Config writes.** niri's main config (`config.kdl`) is one block-structured file with no include directive, and `niri msg action load-config-file --path X` replaces the active config wholesale rather than merging. nirimon stays out of that file entirely. Profile json is the only persistent state it owns.
- **Native mirror.** niri 26.04 has no equivalent of `monitor=...,mirror,<source>`. The mirror picker still saves the field to profile json so the data round-trips, but `applyMonitor` ignores it and logs a warning.
- **HDR / bitdepth / color management.** niri configures these via separate KDL syntax (`output { color-format ... }` etc.) that does not map cleanly to the hyprmon-era `monitor=...,bitdepth,cm,sdrbrightness` parameters. The fields stay on the Monitor struct for profile json round-trip but are not applied.

## Niri-specific quirks the apply path handles

- **Exact mode string snap.** niri silently no-ops on a non-matching mode string. The apply path re-fetches the live mode list and emits the exact "WxH@HHH.MMM" form (formatted from millihertz, not float) closest to the wanted refresh within 1 Hz. A larger delta returns an error rather than picking the wrong rate.
- **EDID-first naming.** niri accepts both `niri msg output DP-3 mode ...` and `niri msg output "LG Electronics LG ULTRAGEAR+ 601NTQDH7820" mode ...`. The latter survives connector reassignment, so nirimon prefers EDIDName at apply time and falls back to the connector Name. For monitors without a serial niri uses the literal word "Unknown" as the third segment.
- **Workspace migration.** niri evacuates workspaces from disabled outputs automatically, so the hyprmon-era pre/post snapshot dance and `moveworkspacetomonitor` calls are dropped.

## Profile format

```json
{
  "name": "home",
  "monitors": [
    {
      "name": "DP-3",
      "hardware_id": "LG Electronics/LG ULTRAGEAR+/601NTQDH7820",
      "make": "LG Electronics",
      "model": "LG ULTRAGEAR+",
      "PxW": 2560, "PxH": 1440, "Hz": 240.083, "Scale": 1.0,
      "X": 0, "Y": 1504,
      "Active": true,
      "EDIDName": "LG Electronics LG ULTRAGEAR+ 601NTQDH7820",
      "Modes": [{"W": 2560, "H": 1440, "Hz": 240.083}, ...],
      "Transform": 0, "VRR": 0
    }
  ],
  "created_at": "2025-11-15T15:36:56+01:00",
  "updated_at": "2026-05-11T22:00:00+02:00"
}
```

Matching across runs is by `hardware_id` (Make/Model/Serial), so port reassignment between reboots is handled transparently. The connector `name` and `EDIDName` are refreshed from live niri data at apply time.

## Acknowledgments

- [hyprmon](https://github.com/erans/hyprmon) by Eran Sandler, the codebase nirimon forked from
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) for the TUI framework
- [niri](https://github.com/YaLTeR/niri) by Ivan Molodetskikh

## License

Apache 2.0 (inherited from hyprmon). See LICENSE.

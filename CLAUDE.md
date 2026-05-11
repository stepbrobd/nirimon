# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Project Overview

nirimon is a TUI tool for arranging monitors under the niri Wayland compositor. It is a niri-only fork of hyprmon by Eran Sandler. All hyprctl call-sites were replaced with `niri msg ...` invocations; all on-disk config writing was removed. Profile json is the only persistent state nirimon owns.

## Apply-only, no config writes

niri has no include directive in its KDL config, and `niri msg action load-config-file --path` replaces the active config wholesale rather than merging. The user's `~/.config/niri/config.kdl` is also NixOS-managed via home-manager (clobbered on switch). For both reasons, nirimon does not write to any niri config file. Monitor changes are applied via `niri msg output ...` calls (runtime-temporary, lost on niri reload); profile json is the source of truth, and the user re-applies after niri restart with `nirimon -profile <name>`.

There is no Save keybind. Apply (A) is the only application path, P saves a profile, O opens the profiles page.

## Build and Development Commands

```bash
make build            # build to ./bin/nirimon
make run              # build and launch the TUI
make test             # go test ./...
make fmt              # go fmt ./...
make fmt-check        # fail if anything needs formatting
make vet              # go vet ./...
make lint             # golangci-lint if installed, otherwise warn
make hooks            # install pre-commit + pre-push hooks
make install          # install to /usr/local/bin
./nirimon --active-profile    # print currently active profile (or empty)
./nirimon --list-profiles     # one per line, active marked "*"
```

## Architecture

| File | Role |
|------|------|
| `main.go` | CLI flags + Bubble Tea entry point |
| `niri.go` | Everything that talks to niri ipc: readMonitors, applyMonitor, snapMode, transform mapping, the `execNiri`/`execNiriJSON` wrappers, and the rollback state |
| `models.go` | Monitor struct, world/grid types, Bubble Tea model and messages, drag/snap math |
| `profiles.go` | Profile json io and the profile-selection menu |
| `hardware_id.go` | buildHardwareID, disambiguateHardwareIDs, resolveProfileMonitors, profile migration helpers, DisplayLabel |
| `update.go` | Bubble Tea Update for the main UI |
| `view.go` | Bubble Tea View for the main UI |
| `advanced_settings.go` | The C/D dialog (bit depth, color mode, SDR sliders, VRR, transform). Most fields are kept for profile json round-trip but have no effect on a niri apply since niri configures these via separate KDL syntax |
| `mode_picker.go` | F key: resolution + refresh selection |
| `scale_picker.go` | R key: DPI scale selection |
| `mirror_picker.go` | M key: mirror configuration. niri has no native mirror, so this dialog records the choice in profile json but applyMonitor ignores it and logs a warning |
| `profile_input.go` | P key: name-input modal for new profiles |
| `version.go` | Build-time version metadata via -ldflags |

## Monitor identity

| Field | Stability | Used for |
|-------|-----------|----------|
| `Name` | Unstable. Kernel-assigned (DP-1, eDP-1). Changes across replug/reboot/cable flip. | Display in TUI; fallback at apply when EDIDName is empty |
| `HardwareID` | Stable. `<make>/<model>/<serial>` or `<make>/<model>` if no serial. Disambiguated with `/#N` for duplicates. | Profile matching across runs (the primary key) |
| `EDIDName` | Stable. `<make> <model> <serial>` (or `<make> <model> Unknown` when serial is null). Matches the exact identifier niri itself uses. | Apply-time output identifier; preferred over Name |
| `Alias` | User-set | Optional display label in the TUI |

Both `Name` and `EDIDName` are refreshed from live niri data inside `resolveProfileMonitors` at apply time, so a port rename between save and apply is handled.

## niri ipc contract

`niri msg --json outputs` returns a JSON **object** keyed by output name (not an array). Notable nullable fields:
- `serial` is null when EDID has no serial (laptop panels)
- `current_mode` is null when the output is disabled (otherwise an int index into the same output's `modes` array)
- `logical` is null when the output is disabled (otherwise has `x, y, width, height, scale, transform`)

The IPC emits transform as TitleCase strings ("Normal", "Flipped-90"). The CLI accepts the lowercase kebab-case form ("normal", "flipped-90"). `parseNiriTransform` reads case-insensitively; `niriTransformString` emits the lowercase form for apply.

`refresh_rate` is in millihertz. The apply path formats mode strings directly from the integer (`%d.%03d`) so the result byte-matches what niri reports - niri silently no-ops on a non-exact mode string.

## Mode-string snap

`snapMode(modes, w, h, wantedHz)`:
1. Filter modes by exact W and H match
2. Pick the one with smallest |hz - wantedHz|
3. Return an error if no match is within 1 Hz

This is the riskiest piece of the apply path; a returned error surfaces in the TUI rather than silently leaving the monitor at the wrong refresh.

## Profile json structure

```json
{
  "name": "home",
  "monitors": [
    {"name": "DP-3", "hardware_id": "...", "make": "...", "model": "...",
     "serial": "...", "PxW": 2560, "PxH": 1440, "Hz": 240.083, "Scale": 1.0,
     "X": 0, "Y": 0, "Active": true, "EDIDName": "...", "Modes": [...],
     "Transform": 0, "VRR": 0, "IsMirrored": false, "MirrorSource": "",
     "MirrorTargets": []}
  ],
  "created_at": "2025-11-15T15:36:56+01:00",
  "updated_at": "2026-05-11T22:00:00+02:00"
}
```

Compatible with hyprmon-era profile json. Round-trip identity (except for `updated_at`) is a validation requirement.

## Important File Paths

- Profiles: `~/.config/nirimon/profiles/` (overridden by `-cfg <dir>`)
- Profile ordering: `~/.config/nirimon/profiles/.profile_order`

There is no nirimon settings file, no config file at `~/.config/niri/*` is written, and there are no backup files.

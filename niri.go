package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// niriTimeout bounds every `niri msg` invocation
	niriTimeout = 5 * time.Second

	// file permissions
	configFileMode  = 0600
	profileDirMode  = 0700
	profileFileMode = 0600

	// default world dimensions
	defaultWorldWidth  = 3840
	defaultWorldHeight = 2160
	defaultWorldScale  = 1.0

	worldPaddingPx      = 500
	desktopBorderMargin = 3
	desktopFooterHeight = 10
)

// isValidMonitorName guards against shell injection when the connector name is
// embedded in a `niri msg output <name>` invocation
func isValidMonitorName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isSpecial := r == '-' || r == '_' || r == '.'
		if !isLower && !isUpper && !isDigit && !isSpecial {
			return false
		}
	}
	return true
}

// isValidColorMode validates a hyprmon-era ColorMode value; the field is
// preserved on the Monitor struct for profile JSON round-trip but has no
// effect on a niri apply
func isValidColorMode(mode string) bool {
	validModes := map[string]bool{
		"auto":    true,
		"srgb":    true,
		"wide":    true,
		"edid":    true,
		"hdr":     true,
		"hdredid": true,
		"":        true,
	}
	return validModes[mode]
}

// sanitizeDesc trims and validates an EDID description string; retained so the
// advanced-settings dialog can still surface the "write as desc:" affordance
// even though niri natively keys by EDID and the flag is a no-op at apply time
func sanitizeDesc(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, r := range s {
		if r == ',' || r == '"' || r == '\n' || r < 0x20 {
			return ""
		}
	}
	return s
}

// canUseDescFormat mirrors the hyprmon-era check; the toggle now has no
// runtime effect under niri but is retained for profile JSON compatibility
func canUseDescFormat(m Monitor) bool {
	if m.HardwareID == "" {
		return false
	}
	if strings.Contains(m.HardwareID, "/#") {
		return false
	}
	return sanitizeDesc(m.EDIDName) != ""
}

// applyMonitorPrefs merges per-monitor preferences from settings.json into the
// monitor slice; the only preference today is the inert UseDescFormat flag
func applyMonitorPrefs(monitors []Monitor, s *Settings) {
	if s == nil {
		return
	}
	for i := range monitors {
		if monitors[i].HardwareID == "" {
			continue
		}
		pref := getMonitorPref(s, monitors[i].HardwareID)
		monitors[i].UseDescFormat = pref.UseDescFormat
	}
}

// parseMode accepts both the hyprland "WxH@Hz" form and the niri
// "WxH@HHH.HHH" form, including an optional trailing "Hz" suffix
func parseMode(modeStr string) *Mode {
	parts := strings.Split(modeStr, "@")
	if len(parts) != 2 {
		return nil
	}
	resParts := strings.Split(parts[0], "x")
	if len(resParts) != 2 {
		return nil
	}
	w, err := strconv.ParseUint(resParts[0], 10, 32)
	if err != nil {
		return nil
	}
	h, err := strconv.ParseUint(resParts[1], 10, 32)
	if err != nil {
		return nil
	}
	hzStr := strings.TrimSuffix(parts[1], "Hz")
	hz, err := strconv.ParseFloat(hzStr, 32)
	if err != nil {
		return nil
	}
	return &Mode{W: uint32(w), H: uint32(h), Hz: float32(hz)}
}

// execNiri runs `niri msg <args>` with a bounded timeout and returns stdout
func execNiri(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), niriTimeout)
	defer cancel()
	full := append([]string{"msg"}, args...)
	cmd := exec.CommandContext(ctx, "niri", full...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute niri msg %v: %w", args, err)
	}
	return output, nil
}

// execNiriJSON runs `niri msg --json <args>` and decodes the stdout JSON
func execNiriJSON(result any, args ...string) error {
	full := append([]string{"--json"}, args...)
	output, err := execNiri(full...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(output, result); err != nil {
		return fmt.Errorf("failed to parse JSON from niri msg %v: %w", args, err)
	}
	return nil
}

// niriOutput mirrors the schema returned by `niri msg --json outputs`; the
// command emits a JSON object keyed by output name, not an array, and several
// fields are nullable when the output is unconfigured (current_mode, logical)
// or when EDID metadata is absent (serial on a laptop panel)
type niriOutput struct {
	Name         string       `json:"name"`
	Make         string       `json:"make"`
	Model        string       `json:"model"`
	Serial       *string      `json:"serial"`
	PhysicalSize [2]int       `json:"physical_size"`
	Modes        []niriMode   `json:"modes"`
	CurrentMode  *int         `json:"current_mode"`
	IsCustomMode bool         `json:"is_custom_mode"`
	VRRSupported bool         `json:"vrr_supported"`
	VRREnabled   bool         `json:"vrr_enabled"`
	Logical      *niriLogical `json:"logical"`
}

type niriMode struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	RefreshRate int  `json:"refresh_rate"` // in millihertz
	IsPreferred bool `json:"is_preferred"`
}

type niriLogical struct {
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Scale     float64 `json:"scale"`
	Transform string  `json:"transform"`
}

// parseNiriTransform maps niri's JSON transform strings (TitleCase, e.g.
// "Normal", "Flipped-90") to hyprmon's int codes 0-7. Case and the optional
// hyphen are tolerated since the IPC and CLI capitalize differently
func parseNiriTransform(s string) int {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	switch s {
	case "normal":
		return 0
	case "90":
		return 1
	case "180":
		return 2
	case "270":
		return 3
	case "flipped":
		return 4
	case "flipped90":
		return 5
	case "flipped180":
		return 6
	case "flipped270":
		return 7
	default:
		return 0
	}
}

// niriModeToMode converts a single niri mode entry; millihertz is divided by
// 1000 to land in the float32 Hz field the rest of the codebase uses
func niriModeToMode(m niriMode) Mode {
	return Mode{
		W:  uint32(m.Width),
		H:  uint32(m.Height),
		Hz: float32(float64(m.RefreshRate) / 1000.0),
	}
}

// buildEDIDName produces the space-separated "make model serial" form niri
// uses as an output identifier; when serial is absent the literal "Unknown"
// is substituted to match niri's own formatting (niri msg output silently
// treats "BOE NE135A1M-NY1" as a different, unconnected output while
// "BOE NE135A1M-NY1 Unknown" matches the actual laptop panel)
func buildEDIDName(make_, model string, serial *string) string {
	mk := strings.TrimSpace(make_)
	md := strings.TrimSpace(model)
	sr := ""
	if serial != nil {
		sr = strings.TrimSpace(*serial)
	}
	if sr == "" {
		sr = "Unknown"
	}
	parts := []string{}
	if mk != "" {
		parts = append(parts, mk)
	}
	if md != "" {
		parts = append(parts, md)
	}
	parts = append(parts, sr)
	return strings.Join(parts, " ")
}

func readMonitors() ([]Monitor, error) {
	var outputs map[string]niriOutput
	if err := execNiriJSON(&outputs, "outputs"); err != nil {
		return nil, err
	}

	// sort by name for deterministic ordering across runs
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)

	monitors := make([]Monitor, 0, len(names))
	for _, name := range names {
		out := outputs[name]

		modes := make([]Mode, 0, len(out.Modes))
		for _, m := range out.Modes {
			modes = append(modes, niriModeToMode(m))
		}

		serial := ""
		if out.Serial != nil {
			serial = *out.Serial
		}

		monitor := Monitor{
			Name:       out.Name,
			Make:       out.Make,
			Model:      out.Model,
			Serial:     serial,
			HardwareID: buildHardwareID(out.Make, out.Model, serial),
			EDIDName:   buildEDIDName(out.Make, out.Model, out.Serial),
			Modes:      modes,
			Active:     out.Logical != nil,
			VRR: func() int {
				if out.VRREnabled {
					return 1
				}
				return 0
			}(),
			// mirror fields are inert under niri; preserved for profile JSON
			// round-trip and updated only via the TUI mirror picker
			IsMirrored:    false,
			MirrorSource:  "",
			MirrorTargets: []string{},
		}

		// current_mode is an index into the modes array when the output is
		// enabled; when null, fall back to the preferred mode so the TUI has
		// sensible default dimensions to render even for a disabled output
		modeIdx := -1
		if out.CurrentMode != nil {
			modeIdx = *out.CurrentMode
		} else {
			for i, m := range out.Modes {
				if m.IsPreferred {
					modeIdx = i
					break
				}
			}
		}
		if modeIdx >= 0 && modeIdx < len(out.Modes) {
			monitor.PxW = uint32(out.Modes[modeIdx].Width)
			monitor.PxH = uint32(out.Modes[modeIdx].Height)
			monitor.Hz = float32(float64(out.Modes[modeIdx].RefreshRate) / 1000.0)
		}

		if out.Logical != nil {
			monitor.X = int32(out.Logical.X)
			monitor.Y = int32(out.Logical.Y)
			monitor.Scale = float32(out.Logical.Scale)
			monitor.Transform = parseNiriTransform(out.Logical.Transform)
		} else {
			// disabled outputs still need a usable scale for the TUI; default
			// to 1.0 rather than leaving the field zero which would divide by
			// zero in the world-bounds math
			monitor.Scale = 1.0
		}

		monitors = append(monitors, monitor)
	}

	disambiguateHardwareIDs(monitors)

	if s, err := loadSettings(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load nirimon settings: %v\n", err)
	} else {
		applyMonitorPrefs(monitors, s)
	}

	return monitors, nil
}

// niriOutputName picks the EDID-style identifier when available; niri keys
// outputs by the EDID string and that survives connector reassignment across
// reboots, while the bare connector Name is unstable. The connector Name is
// the fallback for legacy profiles that lack an EDIDName
func niriOutputName(m Monitor) string {
	if s := strings.TrimSpace(m.EDIDName); s != "" {
		return s
	}
	return m.Name
}

// snapMode picks the niri mode whose width and height match exactly and whose
// refresh rate is closest to wantedHz (in float Hz). The returned string is
// formatted directly from the millihertz integer so it byte-matches what niri
// reports in `niri msg --json outputs`. A delta beyond 1 Hz is rejected since
// niri silently no-ops on a non-matching mode string
func snapMode(modes []niriMode, w, h uint32, wantedHz float32) (string, error) {
	bestIdx := -1
	bestDelta := math.Inf(1)
	for i := range modes {
		if uint32(modes[i].Width) != w || uint32(modes[i].Height) != h {
			continue
		}
		hz := float64(modes[i].RefreshRate) / 1000.0
		d := math.Abs(hz - float64(wantedHz))
		if d < bestDelta {
			bestDelta = d
			bestIdx = i
		}
	}
	if bestIdx < 0 || bestDelta > 1.0 {
		return "", fmt.Errorf("no niri mode matches %dx%d@%.3f within 1Hz", w, h, wantedHz)
	}
	mhz := modes[bestIdx].RefreshRate
	return fmt.Sprintf("%dx%d@%d.%03d", modes[bestIdx].Width, modes[bestIdx].Height, mhz/1000, mhz%1000), nil
}

// niriTransformString maps the hyprmon Transform int code 0-7 to the lowercase
// kebab-case form niri's CLI accepts (the IPC JSON returns TitleCase, see
// parseNiriTransform for the inverse)
func niriTransformString(t int) string {
	switch t {
	case 0:
		return "normal"
	case 1:
		return "90"
	case 2:
		return "180"
	case 3:
		return "270"
	case 4:
		return "flipped"
	case 5:
		return "flipped-90"
	case 6:
		return "flipped-180"
	case 7:
		return "flipped-270"
	default:
		return "normal"
	}
}

// applyVRR runs the right vrr subcommand for hyprmon's VRR enum
// 0 = off, 1 = on, 2 = on-demand (hyprland called this "fullscreen-only")
func applyVRR(output string, vrr int) error {
	switch vrr {
	case 1:
		_, err := execNiri("output", output, "vrr", "on")
		return err
	case 2:
		_, err := execNiri("output", output, "vrr", "on", "--on-demand")
		return err
	default:
		_, err := execNiri("output", output, "vrr", "off")
		return err
	}
}

func applyMonitor(m Monitor) error {
	output := niriOutputName(m)

	// off path: a single command, no further configuration
	if !m.Active {
		if _, err := execNiri("output", output, "off"); err != nil {
			return fmt.Errorf("disable %s: %w", output, err)
		}
		return nil
	}

	// re-fetch live modes for exact-string snapping; the wanted mode in the
	// profile is float Hz but niri keys modes by exact millihertz, and the
	// available set may shift between save and apply
	var outputs map[string]niriOutput
	if err := execNiriJSON(&outputs, "outputs"); err != nil {
		return fmt.Errorf("list outputs while applying %s: %w", output, err)
	}
	out, ok := outputs[m.Name]
	if !ok {
		// fall back to scanning by EDID-derived identifier; the connector
		// Name may have shifted since the profile was saved
		for _, o := range outputs {
			oSerial := (*string)(nil)
			if o.Serial != nil {
				s := *o.Serial
				oSerial = &s
			}
			if buildEDIDName(o.Make, o.Model, oSerial) == output {
				out = o
				ok = true
				break
			}
		}
	}
	if !ok {
		return fmt.Errorf("output %q is not currently connected", output)
	}

	if _, err := execNiri("output", output, "on"); err != nil {
		return fmt.Errorf("enable %s: %w", output, err)
	}

	if m.IsMirrored {
		fmt.Fprintf(os.Stderr, "warning: niri has no native mirror; %s applied as a normal output\n", output)
	}

	modeStr, err := snapMode(out.Modes, m.PxW, m.PxH, m.Hz)
	if err != nil {
		return fmt.Errorf("snap mode for %s: %w", output, err)
	}
	if _, err := execNiri("output", output, "mode", modeStr); err != nil {
		return fmt.Errorf("set mode for %s: %w", output, err)
	}

	if _, err := execNiri("output", output, "scale", fmt.Sprintf("%g", m.Scale)); err != nil {
		return fmt.Errorf("set scale for %s: %w", output, err)
	}

	if _, err := execNiri("output", output, "position", "set",
		strconv.Itoa(int(m.X)), strconv.Itoa(int(m.Y))); err != nil {
		return fmt.Errorf("set position for %s: %w", output, err)
	}

	if _, err := execNiri("output", output, "transform", niriTransformString(m.Transform)); err != nil {
		return fmt.Errorf("set transform for %s: %w", output, err)
	}

	if err := applyVRR(output, m.VRR); err != nil {
		return fmt.Errorf("set vrr for %s: %w", output, err)
	}

	return nil
}

func applyMonitors(monitors []Monitor) error {
	for _, m := range monitors {
		if err := applyMonitor(m); err != nil {
			return fmt.Errorf("failed to apply monitor %s: %w", m.Name, err)
		}
	}
	return nil
}

// getAvailableModes returns the mode list for one output formatted as
// "WxH@Hz.HHH" so the existing mode picker (which speaks the hyprland string
// form) can consume them without further conversion
func getAvailableModes(monitorName string) ([]string, error) {
	var outputs map[string]niriOutput
	if err := execNiriJSON(&outputs, "outputs"); err != nil {
		return nil, err
	}
	out, ok := outputs[monitorName]
	if !ok {
		return nil, fmt.Errorf("monitor %s not found", monitorName)
	}
	modes := make([]string, 0, len(out.Modes))
	for _, m := range out.Modes {
		hz := float64(m.RefreshRate) / 1000.0
		// the trailing "Hz" suffix matches what hyprctl emitted and is what
		// mode_picker.go's parser regex expects; without it parseDisplayModes
		// returns an empty slice and the mode picker view panics
		modes = append(modes, fmt.Sprintf("%dx%d@%.3fHz", m.Width, m.Height, hz))
	}
	return modes, nil
}

// rollback state
var previousMonitors []Monitor

func saveRollback(monitors []Monitor) {
	previousMonitors = make([]Monitor, len(monitors))
	copy(previousMonitors, monitors)
}

func rollback() error {
	if previousMonitors == nil {
		return fmt.Errorf("no previous state to rollback to")
	}
	return applyMonitors(previousMonitors)
}

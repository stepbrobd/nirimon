package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	backupFileMode  = 0600

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
func execNiriJSON(result interface{}, args ...string) error {
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

// readMonitors is implemented in a follow-up commit; this stub keeps callers
// compiling so the rest of the renames can land first
func readMonitors() ([]Monitor, error) {
	return nil, fmt.Errorf("readMonitors not yet implemented for niri")
}

// applyMonitor is implemented in a follow-up commit
func applyMonitor(m Monitor) error {
	return fmt.Errorf("applyMonitor not yet implemented for %s", m.Name)
}

func applyMonitors(monitors []Monitor) error {
	for _, m := range monitors {
		if err := applyMonitor(m); err != nil {
			return fmt.Errorf("failed to apply monitor %s: %w", m.Name, err)
		}
	}
	return nil
}

// getAvailableModes will be implemented alongside readMonitors
func getAvailableModes(monitorName string) ([]string, error) {
	return nil, fmt.Errorf("getAvailableModes not yet implemented for %s", monitorName)
}

// the following are hyprmon-era call-site stubs; nirimon is apply-only so
// these no-op pending the cleanup that removes the callers in profiles.go
// and update.go

func getCurrentMonitorNames() ([]string, error) {
	return nil, nil
}

// migrateOrphanedWorkspaces is a no-op under niri, which evacuates workspaces
// automatically when an output disables
func migrateOrphanedWorkspaces(_, _ []string) error {
	return nil
}

// writeConfig is a no-op under nirimon; profile JSON is the source of truth
// and runtime application is via `niri msg output`
func writeConfig(_ []Monitor) error {
	return nil
}

// reloadConfig is a no-op under nirimon; niri auto-reloads its own config and
// `niri msg output` changes are runtime-temporary
func reloadConfig() error {
	return nil
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

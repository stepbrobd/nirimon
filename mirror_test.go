package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestWouldCreateCircularMirror(t *testing.T) {
	tests := []struct {
		name             string
		currentMonitor   string
		sourceMonitor    string
		allMonitors      []Monitor
		expectedCircular bool
	}{
		{
			name:           "No circular mirror - simple case",
			currentMonitor: "HDMI-A-1",
			sourceMonitor:  "eDP-1",
			allMonitors: []Monitor{
				{Name: "HDMI-A-1", Active: true},
				{Name: "eDP-1", Active: true},
			},
			expectedCircular: false,
		},
		{
			name:           "Direct circular mirror",
			currentMonitor: "HDMI-A-1",
			sourceMonitor:  "eDP-1",
			allMonitors: []Monitor{
				{Name: "HDMI-A-1", Active: true},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedCircular: true,
		},
		{
			name:           "Indirect circular mirror (3 monitors)",
			currentMonitor: "HDMI-A-1",
			sourceMonitor:  "eDP-1",
			allMonitors: []Monitor{
				{Name: "HDMI-A-1", Active: true},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
				{Name: "DP-1", Active: true, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedCircular: true,
		},
		{
			name:           "No circular mirror - chain but not circular",
			currentMonitor: "HDMI-A-1",
			sourceMonitor:  "eDP-1",
			allMonitors: []Monitor{
				{Name: "HDMI-A-1", Active: true},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
				{Name: "DP-1", Active: true},
			},
			expectedCircular: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wouldCreateCircularMirror(tt.currentMonitor, tt.sourceMonitor, tt.allMonitors)
			if result != tt.expectedCircular {
				t.Errorf("wouldCreateCircularMirror() = %v, want %v", result, tt.expectedCircular)
			}
		})
	}
}

func TestValidateMirrorConfiguration(t *testing.T) {
	tests := []struct {
		name             string
		monitors         []Monitor
		expectedWarnings int
	}{
		{
			name: "No warnings - valid configuration",
			monitors: []Monitor{
				{Name: "HDMI-A-1", Active: true, PxW: 1920, PxH: 1080},
				{Name: "eDP-1", Active: true, PxW: 1920, PxH: 1080, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedWarnings: 0,
		},
		{
			name: "Resolution mismatch warning",
			monitors: []Monitor{
				{Name: "HDMI-A-1", Active: true, PxW: 1920, PxH: 1080},
				{Name: "eDP-1", Active: true, PxW: 3840, PxH: 2160, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedWarnings: 1,
		},
		{
			name: "Disabled source monitor warning",
			monitors: []Monitor{
				{Name: "HDMI-A-1", Active: false, PxW: 1920, PxH: 1080},
				{Name: "eDP-1", Active: true, PxW: 1920, PxH: 1080, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedWarnings: 1,
		},
		{
			name: "Multiple warnings",
			monitors: []Monitor{
				{Name: "HDMI-A-1", Active: false, PxW: 1920, PxH: 1080},
				{Name: "eDP-1", Active: true, PxW: 3840, PxH: 2160, IsMirrored: true, MirrorSource: "HDMI-A-1"},
			},
			expectedWarnings: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validateMirrorConfiguration(tt.monitors)
			if len(warnings) != tt.expectedWarnings {
				t.Errorf("validateMirrorConfiguration() returned %d warnings, want %d. Warnings: %v",
					len(warnings), tt.expectedWarnings, warnings)
			}
		})
	}
}

func sortSpecs(s []mirrorSpec) []mirrorSpec {
	out := append([]mirrorSpec(nil), s...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func sortProcs(p []mirrorProc) []mirrorProc {
	out := append([]mirrorProc(nil), p...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func TestDesiredMirrors(t *testing.T) {
	tests := []struct {
		name     string
		monitors []Monitor
		want     []mirrorSpec
	}{
		{
			name: "active mirror with active source produces a spec",
			monitors: []Monitor{
				{Name: "DP-1", Active: true},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
			},
			want: []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
		},
		{
			name: "disabled source is dropped (wl-mirror would exit)",
			monitors: []Monitor{
				{Name: "DP-1", Active: false},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
			},
			want: nil,
		},
		{
			name: "dangling source (no such monitor) is dropped",
			monitors: []Monitor{
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "HDMI-A-9"},
			},
			want: nil,
		},
		{
			name: "self-mirror is dropped",
			monitors: []Monitor{
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "eDP-1"},
			},
			want: nil,
		},
		{
			name: "disabled target produces no spec",
			monitors: []Monitor{
				{Name: "DP-1", Active: true},
				{Name: "eDP-1", Active: false, IsMirrored: true, MirrorSource: "DP-1"},
			},
			want: nil,
		},
		{
			name: "two targets mirroring one source, sorted by target",
			monitors: []Monitor{
				{Name: "DP-1", Active: true},
				{Name: "HDMI-A-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
				{Name: "eDP-1", Active: true, IsMirrored: true, MirrorSource: "DP-1"},
			},
			want: []mirrorSpec{
				{Target: "HDMI-A-1", Source: "DP-1"},
				{Target: "eDP-1", Source: "DP-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := desiredMirrors(tt.monitors)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("desiredMirrors() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffMirrors(t *testing.T) {
	// aliveSet builds an isAlive stub from a set of pids treated as live
	aliveSet := func(pids ...int) func(int) bool {
		live := make(map[int]bool, len(pids))
		for _, p := range pids {
			live[p] = true
		}
		return func(pid int) bool { return live[pid] }
	}

	tests := []struct {
		name      string
		desired   []mirrorSpec
		running   []mirrorProc
		isAlive   func(int) bool
		wantStart []mirrorSpec
		wantKeep  []mirrorProc
		wantStop  []mirrorProc
	}{
		{
			name:    "empty desired and running yields empty plan",
			isAlive: aliveSet(),
		},
		{
			name:      "new desired with nothing running is started",
			desired:   []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
			isAlive:   aliveSet(),
			wantStart: []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
		},
		{
			name:     "running, desired, alive, same source is kept untouched",
			desired:  []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
			running:  []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
			isAlive:  aliveSet(100),
			wantKeep: []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
		},
		{
			name:     "running but no longer desired is stopped",
			running:  []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
			isAlive:  aliveSet(100),
			wantStop: []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
		},
		{
			name:      "desired but the tracked process died is respawned and corpse stopped",
			desired:   []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
			running:   []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
			isAlive:   aliveSet(), // pid 100 is dead
			wantStart: []mirrorSpec{{Target: "eDP-1", Source: "DP-1"}},
			wantStop:  []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
		},
		{
			name:     "dead and undesired is cleaned up without a restart",
			running:  []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
			isAlive:  aliveSet(), // dead
			wantStop: []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
		},
		{
			name:      "source changed on the same target stops the old and starts the new",
			desired:   []mirrorSpec{{Target: "eDP-1", Source: "HDMI-A-1"}},
			running:   []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
			isAlive:   aliveSet(100),
			wantStart: []mirrorSpec{{Target: "eDP-1", Source: "HDMI-A-1"}},
			wantStop:  []mirrorProc{{Target: "eDP-1", Source: "DP-1", PID: 100}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := diffMirrors(tt.desired, tt.running, tt.isAlive)
			if !reflect.DeepEqual(sortSpecs(plan.Start), sortSpecs(tt.wantStart)) {
				t.Errorf("Start = %+v, want %+v", plan.Start, tt.wantStart)
			}
			if !reflect.DeepEqual(sortProcs(plan.Keep), sortProcs(tt.wantKeep)) {
				t.Errorf("Keep = %+v, want %+v", plan.Keep, tt.wantKeep)
			}
			if !reflect.DeepEqual(sortProcs(plan.Stop), sortProcs(tt.wantStop)) {
				t.Errorf("Stop = %+v, want %+v", plan.Stop, tt.wantStop)
			}
		})
	}
}

func TestMirrorStateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// missing file is the clean first-run case: empty slice, no error
	got, err := loadMirrorState(dir)
	if err != nil {
		t.Fatalf("loadMirrorState on missing file: unexpected error %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("loadMirrorState on missing file = %+v, want empty", got)
	}

	procs := []mirrorProc{
		{Target: "eDP-1", Source: "DP-1", PID: 100},
		{Target: "HDMI-A-1", Source: "DP-1", PID: 101},
	}
	if err := saveMirrorState(dir, procs); err != nil {
		t.Fatalf("saveMirrorState: %v", err)
	}
	got, err = loadMirrorState(dir)
	if err != nil {
		t.Fatalf("loadMirrorState after save: %v", err)
	}
	if !reflect.DeepEqual(got, procs) {
		t.Errorf("round-trip = %+v, want %+v", got, procs)
	}

	// malformed json surfaces an error rather than silently dropping state
	if err := os.WriteFile(filepath.Join(dir, mirrorStateFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write malformed state: %v", err)
	}
	if _, err := loadMirrorState(dir); err == nil {
		t.Error("loadMirrorState on malformed json: expected an error, got nil")
	}
}

func TestMirrorStateDirNotUnderConfig(t *testing.T) {
	// with XDG_RUNTIME_DIR set the path is ephemeral under it
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	if got, want := mirrorStateDir(), filepath.Join("/run/user/4242", "nirimon"); got != want {
		t.Errorf("mirrorStateDir() = %q, want %q", got, want)
	}

	// unset must fall back to a temp path, never under ~/.config (apply-only:
	// the runtime pid file must stay ephemeral, not persistent config)
	t.Setenv("XDG_RUNTIME_DIR", "")
	got := mirrorStateDir()
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		rel, err := filepath.Rel(cfg, got)
		if err == nil && rel != ".." && !startsWithDotDot(rel) {
			t.Errorf("mirrorStateDir() = %q must not be under user config dir %q", got, cfg)
		}
	}
}

// startsWithDotDot reports whether a filepath.Rel result escapes the base dir
func startsWithDotDot(rel string) bool {
	return rel == ".." || (len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator))
}

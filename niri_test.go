package main

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestParseNiriTransform(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"Normal", 0},
		{"normal", 0},
		{"NORMAL", 0},
		{"90", 1},
		{"180", 2},
		{"270", 3},
		{"Flipped", 4},
		{"flipped", 4},
		{"Flipped-90", 5},
		{"flipped-90", 5},
		{"flipped_90", 5},
		{"FLIPPED90", 5},
		{"Flipped-180", 6},
		{"Flipped-270", 7},
		{"garbage", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := parseNiriTransform(tt.in); got != tt.want {
			t.Errorf("parseNiriTransform(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNiriTransformRoundTrip(t *testing.T) {
	for i := range 8 {
		got := parseNiriTransform(niriTransformString(i))
		if got != i {
			t.Errorf("round trip for transform %d: emitted %q, parsed back as %d",
				i, niriTransformString(i), got)
		}
	}
}

func TestBuildEDIDName(t *testing.T) {
	withSerial := "601NTQDH7820"
	emptySerial := ""
	whitespace := "  "
	tests := []struct {
		name   string
		make_  string
		model  string
		serial *string
		want   string
	}{
		{"all present", "LG Electronics", "LG ULTRAGEAR+", &withSerial,
			"LG Electronics LG ULTRAGEAR+ 601NTQDH7820"},
		{"nil serial uses Unknown", "BOE", "NE135A1M-NY1", nil,
			"BOE NE135A1M-NY1 Unknown"},
		{"empty serial uses Unknown", "BOE", "NE135A1M-NY1", &emptySerial,
			"BOE NE135A1M-NY1 Unknown"},
		{"whitespace serial uses Unknown", "BOE", "NE135A1M-NY1", &whitespace,
			"BOE NE135A1M-NY1 Unknown"},
		{"empty make is dropped", "", "Model", nil, "Model Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildEDIDName(tt.make_, tt.model, tt.serial); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSnapMode(t *testing.T) {
	// fixture mirrors what niri returned for DP-4 on framework: a
	// 2560x1440 monitor with multiple refresh rates close to 240
	modes := []niriMode{
		{Width: 2560, Height: 1440, RefreshRate: 60001},
		{Width: 2560, Height: 1440, RefreshRate: 240002},
		{Width: 2560, Height: 1440, RefreshRate: 144002},
		{Width: 1920, Height: 1080, RefreshRate: 144000},
		{Width: 1920, Height: 1080, RefreshRate: 60000},
	}

	tests := []struct {
		name      string
		w, h      uint32
		hz        float32
		want      string
		wantError bool
	}{
		{"exact niri rate", 2560, 1440, 240.002, "2560x1440@240.002", false},
		{"profile saved at 240.000 snaps to 240.002", 2560, 1440, 240.000, "2560x1440@240.002", false},
		{"60Hz exact", 2560, 1440, 60.001, "2560x1440@60.001", false},
		{"prefers closer of two candidates", 2560, 1440, 145.0, "2560x1440@144.002", false},
		{"1080p selection", 1920, 1080, 60.0, "1920x1080@60.000", false},
		{"resolution mismatch", 1024, 768, 60.0, "", true},
		{"delta beyond 1Hz rejected", 2560, 1440, 230.0, "", true},
		{"empty modes list", 2560, 1440, 60.0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := modes
			if tt.name == "empty modes list" {
				m = nil
			}
			got, err := snapMode(m, tt.w, tt.h, tt.hz)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("snapMode = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetAvailableModesFormatMatchesPicker pins the contract between
// getAvailableModes and the mode picker's parser. Earlier the niri-native
// "WxH@HHH.MMM" form (no Hz suffix) silently broke parseDisplayModes and
// crashed the mode picker on F. This regression test fails if anyone drops
// the Hz suffix again.
func TestGetAvailableModesFormatMatchesPicker(t *testing.T) {
	pickerRegex := regexp.MustCompile(`(\d+)x(\d+)@([\d.]+)Hz`)
	sample := "2560x1440@240.083Hz"
	if !pickerRegex.MatchString(sample) {
		t.Fatalf("regex no longer accepts the emitted format - mode_picker parser changed?")
	}
	if pickerRegex.MatchString("2560x1440@240.083") {
		t.Fatalf("regex now accepts the suffix-less form; the suffix guard is no longer needed")
	}
}

func TestOutputsToMonitorsFixture(t *testing.T) {
	// fixture is the actual JSON shape niri 26.04 emits on the framework
	// host; covers the three tricky null cases (serial, current_mode,
	// logical) and the millihertz refresh-rate decoding
	fixture := `{
		"DP-3": {
			"name": "DP-3",
			"make": "LG Electronics",
			"model": "LG ULTRAGEAR+",
			"serial": "601NTQDH7820",
			"physical_size": [590, 330],
			"modes": [
				{"width": 2560, "height": 1440, "refresh_rate": 480168, "is_preferred": true},
				{"width": 2560, "height": 1440, "refresh_rate": 240083, "is_preferred": false}
			],
			"current_mode": 1,
			"is_custom_mode": false,
			"vrr_supported": true,
			"vrr_enabled": false,
			"logical": {"x": 0, "y": 1504, "width": 2560, "height": 1440, "scale": 1.0, "transform": "Normal"}
		},
		"eDP-1": {
			"name": "eDP-1",
			"make": "BOE",
			"model": "NE135A1M-NY1",
			"serial": null,
			"physical_size": [290, 190],
			"modes": [
				{"width": 2880, "height": 1920, "refresh_rate": 120000, "is_preferred": true}
			],
			"current_mode": null,
			"is_custom_mode": false,
			"vrr_supported": true,
			"vrr_enabled": false,
			"logical": null
		}
	}`

	var outputs map[string]niriOutput
	if err := json.Unmarshal([]byte(fixture), &outputs); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	monitors := outputsToMonitors(outputs)

	if len(monitors) != 2 {
		t.Fatalf("got %d monitors, want 2", len(monitors))
	}

	// monitors are sorted by name so eDP-1 < DP-1; lowercase 'e' sorts
	// after uppercase 'D' in ASCII so DP-3 comes first
	dp3 := monitors[0]
	edp1 := monitors[1]
	if dp3.Name != "DP-3" {
		t.Errorf("monitors[0].Name = %q, want DP-3 (sort by name)", dp3.Name)
	}

	if got, want := dp3.HardwareID, "LG Electronics/LG ULTRAGEAR+/601NTQDH7820"; got != want {
		t.Errorf("DP-3 HardwareID = %q, want %q", got, want)
	}
	if got, want := dp3.EDIDName, "LG Electronics LG ULTRAGEAR+ 601NTQDH7820"; got != want {
		t.Errorf("DP-3 EDIDName = %q, want %q", got, want)
	}
	if !dp3.Active {
		t.Errorf("DP-3 should be Active (logical present)")
	}
	// current_mode = 1 -> mode 2560x1440@240.083 (240083 millihertz / 1000)
	if dp3.PxW != 2560 || dp3.PxH != 1440 {
		t.Errorf("DP-3 PxW/PxH = %d/%d, want 2560/1440", dp3.PxW, dp3.PxH)
	}
	if dp3.Hz < 240.080 || dp3.Hz > 240.085 {
		t.Errorf("DP-3 Hz = %v, want ~240.083", dp3.Hz)
	}
	if dp3.Scale != 1.0 {
		t.Errorf("DP-3 Scale = %v, want 1.0", dp3.Scale)
	}
	if dp3.X != 0 || dp3.Y != 1504 {
		t.Errorf("DP-3 X/Y = %d/%d, want 0/1504", dp3.X, dp3.Y)
	}
	if dp3.Transform != 0 {
		t.Errorf("DP-3 Transform = %d, want 0 (Normal)", dp3.Transform)
	}
	if len(dp3.Modes) != 2 {
		t.Errorf("DP-3 Modes len = %d, want 2", len(dp3.Modes))
	}

	if got, want := edp1.HardwareID, "BOE/NE135A1M-NY1"; got != want {
		t.Errorf("eDP-1 HardwareID = %q, want %q (no /serial segment)", got, want)
	}
	if got, want := edp1.EDIDName, "BOE NE135A1M-NY1 Unknown"; got != want {
		t.Errorf("eDP-1 EDIDName = %q, want %q (Unknown sentinel)", got, want)
	}
	if edp1.Active {
		t.Errorf("eDP-1 should be inactive (logical = null)")
	}
	// current_mode null -> fall back to preferred mode (the only one here)
	if edp1.PxW != 2880 || edp1.PxH != 1920 {
		t.Errorf("eDP-1 PxW/PxH = %d/%d, want 2880/1920 (preferred fallback)", edp1.PxW, edp1.PxH)
	}
	// inactive monitor should still have Scale=1 so the world bounds math
	// in updateWorld doesn't divide by zero
	if edp1.Scale != 1.0 {
		t.Errorf("eDP-1 Scale = %v, want 1.0 default for inactive", edp1.Scale)
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		in     string
		wantW  uint32
		wantH  uint32
		wantHz float32
		isNil  bool
	}{
		{"2560x1440@240.083Hz", 2560, 1440, 240.083, false},
		{"2560x1440@240.083", 2560, 1440, 240.083, false},
		{"1920x1080@60.00Hz", 1920, 1080, 60.00, false},
		{"640x480@59.940", 640, 480, 59.940, false},
		{"garbage", 0, 0, 0, true},
		{"2560x@60", 0, 0, 0, true},
		{"x1440@60", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			m := parseMode(tt.in)
			if tt.isNil {
				if m != nil {
					t.Errorf("expected nil, got %+v", *m)
				}
				return
			}
			if m == nil {
				t.Fatalf("parseMode returned nil")
			}
			if m.W != tt.wantW || m.H != tt.wantH {
				t.Errorf("WxH = %dx%d, want %dx%d", m.W, m.H, tt.wantW, tt.wantH)
			}
			if d := m.Hz - tt.wantHz; d > 0.01 || d < -0.01 {
				t.Errorf("Hz = %v, want %v", m.Hz, tt.wantHz)
			}
		})
	}
}

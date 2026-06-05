package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// niri has no native output mirroring the way hyprland does with
// `monitor=...,mirror,SOURCE`; the niri wiki's screencasting page instead
// points at wl-mirror, a wayland client that captures one output and shows it
// fullscreen on another. nirimon emulates the hyprmon mirror feature by
// spawning one detached wl-mirror per mirrored output and supervising those
// processes across applies. see https://github.com/Ferdi265/wl-mirror
const wlMirrorBinary = "wl-mirror"

// wlMirrorScaling is the wl-mirror scaling mode used for every mirror. "fit"
// preserves the source aspect ratio and always shows the whole source image,
// letterboxing when aspect ratios differ. that is the closest match to
// hyprland's mirror semantics (the entire source is always visible; hyprland
// merely distorts it to fill, and wl-mirror has no distorting-stretch mode).
// alternatives are "cover" (crop to fill) and "exact" (integer multiples only)
const wlMirrorScaling = "fit"

// mirrorStateFileName is the runtime bookkeeping file under mirrorStateDir
const mirrorStateFileName = "mirrors.json"

var (
	wlMirrorOnce  sync.Once
	wlMirrorFound bool
)

// mirrorAvailable reports whether the wl-mirror binary is resolvable in PATH.
// the lookup is cached so the picker can call it cheaply. when false niri has
// no way to mirror, so the feature degrades to the prior no-op: a selection is
// still saved to the profile json, it just is not applied
func mirrorAvailable() bool {
	wlMirrorOnce.Do(func() {
		_, err := exec.LookPath(wlMirrorBinary)
		wlMirrorFound = err == nil
	})
	return wlMirrorFound
}

// mirrorSpec is one desired mirror relationship: the Target output should
// display the contents of the Source output. both are niri CONNECTOR names
// (Monitor.Name, e.g. "DP-2"), never the EDID identifier from niriOutputName,
// because wl-mirror addresses outputs by their wl_output name which niri sets
// to the connector
type mirrorSpec struct {
	Target string
	Source string
}

// mirrorProc is a tracked, running wl-mirror process recorded in the runtime
// state file so a later nirimon invocation can find and manage it by pid
type mirrorProc struct {
	Target string `json:"target"`
	Source string `json:"source"`
	PID    int    `json:"pid"`
}

// mirrorPlan is the result of reconciling desired specs against running procs
type mirrorPlan struct {
	Start []mirrorSpec // desired specs with no live matching proc: spawn these
	Keep  []mirrorProc // live procs still desired with the same source: untouched
	Stop  []mirrorProc // procs to terminate: stale, source-changed, or dead corpses
}

// desiredMirrors derives the mirror specs that should be running for a monitor
// set. a spec is included only when the target is an active mirroring monitor
// with a named source, and that source exists and is active in the same set.
// self-mirrors and dangling or disabled sources are dropped so we never spawn
// a wl-mirror that would immediately exit (it bails if either output is
// absent). the result is sorted by target for deterministic diffing
func desiredMirrors(monitors []Monitor) []mirrorSpec {
	active := make(map[string]bool, len(monitors))
	for _, m := range monitors {
		if m.Active {
			active[m.Name] = true
		}
	}

	var specs []mirrorSpec
	for _, m := range monitors {
		if !m.Active || !m.IsMirrored || m.MirrorSource == "" {
			continue
		}
		if m.MirrorSource == m.Name {
			continue // a monitor mirroring itself is nonsensical
		}
		if !active[m.MirrorSource] {
			continue // source missing or disabled: wl-mirror would just exit
		}
		specs = append(specs, mirrorSpec{Target: m.Name, Source: m.MirrorSource})
	}

	sort.Slice(specs, func(i, j int) bool { return specs[i].Target < specs[j].Target })
	return specs
}

// diffMirrors classifies desired specs and currently-tracked procs into a
// start/keep/stop plan. it is pure: process liveness is supplied via isAlive
// so the policy is fully testable without spawning anything.
//
// policy, keyed on the target output (at most one mirror per target):
//   - a tracked proc whose target is still desired with the SAME source and is
//     alive is kept untouched, so unchanged mirrors never flicker on re-apply
//   - a tracked proc on a target that is still desired but whose source changed,
//     or whose process has died, is stopped and the desired spec is (re)started
//   - a tracked proc on a target that is no longer desired is stopped
//   - a desired spec with no tracked proc on its target is started
func diffMirrors(desired []mirrorSpec, running []mirrorProc, isAlive func(int) bool) mirrorPlan {
	var plan mirrorPlan

	runningByTarget := make(map[string]mirrorProc, len(running))
	for _, p := range running {
		runningByTarget[p.Target] = p
	}
	desiredByTarget := make(map[string]bool, len(desired))
	for _, s := range desired {
		desiredByTarget[s.Target] = true
	}

	for _, s := range desired {
		p, ok := runningByTarget[s.Target]
		if ok && p.Source == s.Source && isAlive(p.PID) {
			plan.Keep = append(plan.Keep, p)
			continue
		}
		if ok {
			// stale source on this target, or a dead corpse: clear it first
			plan.Stop = append(plan.Stop, p)
		}
		plan.Start = append(plan.Start, s)
	}

	for _, p := range running {
		if !desiredByTarget[p.Target] {
			plan.Stop = append(plan.Stop, p)
		}
	}

	return plan
}

// mirrorStateDir returns the directory holding nirimon's wl-mirror process
// state. it lives under XDG_RUNTIME_DIR (tmpfs, cleared on logout) because the
// tracked processes are ephemeral RUNTIME state, not configuration: nirimon is
// apply-only and the profile json remains the sole persistent state. the
// fallback keeps the path ephemeral and, crucially, never under ~/.config
func mirrorStateDir() string {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, "nirimon")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("nirimon-%d", os.Getuid()))
}

// loadMirrorState reads the tracked procs from dir; a missing file is the
// clean first-run case and returns an empty slice with no error
func loadMirrorState(dir string) ([]mirrorProc, error) {
	data, err := os.ReadFile(filepath.Join(dir, mirrorStateFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var procs []mirrorProc
	if err := json.Unmarshal(data, &procs); err != nil {
		return nil, fmt.Errorf("parse mirror state: %w", err)
	}
	return procs, nil
}

// saveMirrorState atomically writes procs to dir/mirrors.json (temp file then
// rename) with the same 0700 dir / 0600 file modes the profile store uses
func saveMirrorState(dir string, procs []mirrorProc) error {
	if err := os.MkdirAll(dir, profileDirMode); err != nil {
		return err
	}
	data, err := json.MarshalIndent(procs, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, mirrorStateFileName+".tmp")
	if err := os.WriteFile(tmp, data, profileFileMode); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, mirrorStateFileName))
}

// pidAlive reports whether pid is a live process via the signal-0 probe (kill
// with signal 0 runs the permission/existence check without delivering a
// signal). this is the single impure aliveness primitive injected into
// diffMirrors
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the pid exists but is owned by another user; treat as alive
	return err == syscall.EPERM
}

// procIsWlMirror guards against pid reuse: a recorded pid may have been
// recycled by an unrelated process after a crash, and we must never SIGTERM a
// stranger. it confirms /proc/<pid>/cmdline references the wl-mirror binary
// before we signal it; a read failure is treated as "not ours" and skipped
func procIsWlMirror(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	// cmdline is a NUL-separated argv; match argv[0]'s basename exactly rather
	// than substring-scanning the whole line, so a recycled pid whose arguments
	// merely mention wl-mirror is never mistaken for one of our processes and
	// SIGTERMed
	argv0, _, _ := strings.Cut(string(data), "\x00")
	return filepath.Base(argv0) == wlMirrorBinary
}

// startMirror spawns a detached wl-mirror that captures spec.Source and
// displays it fullscreen on spec.Target. the child is fully detached: a new
// session (Setsid) so it outlives both the short-lived `--profile` apply and
// the TUI, with stdio sent to /dev/null so a closing terminal cannot SIGPIPE
// or block it. the connector names flow straight through to wl-mirror, which
// addresses outputs by wl_output name (do not substitute the EDID identifier)
func startMirror(spec mirrorSpec) (mirrorProc, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return mirrorProc{}, err
	}
	defer devNull.Close()

	cmd := exec.Command(wlMirrorBinary,
		"--scaling", wlMirrorScaling,
		"--fullscreen-output", spec.Target,
		spec.Source,
	)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return mirrorProc{}, fmt.Errorf("start wl-mirror for %s from %s: %w", spec.Target, spec.Source, err)
	}

	// reap the child if it dies while nirimon is still running, so a wl-mirror
	// that exits (e.g. its source output was unplugged) does not linger as a
	// zombie. if nirimon exits first, the detached session is reparented to
	// init, which reaps it instead
	go func() { _ = cmd.Wait() }()

	return mirrorProc{Target: spec.Target, Source: spec.Source, PID: cmd.Process.Pid}, nil
}

// stopMirror terminates a tracked wl-mirror with SIGTERM (wl-mirror installs no
// signal handlers, so the default disposition ends it cleanly). a pid that is
// already gone is not an error, and the reuse guard ensures we only signal a
// process that is genuinely wl-mirror
func stopMirror(proc mirrorProc) error {
	if !pidAlive(proc.PID) {
		return nil
	}
	if !procIsWlMirror(proc.PID) {
		return nil // pid was recycled by something else; leave it alone
	}
	if err := syscall.Kill(proc.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("stop wl-mirror pid %d: %w", proc.PID, err)
	}
	return nil
}

// withMirrorLock runs fn while holding an exclusive advisory lock on a lockfile
// in dir, serializing the reconcile read-modify-write across concurrent nirimon
// invocations: the TUI apply and a headless `--profile` apply (e.g. a hotplug
// or login hook) share one runtime dir, and without this two overlapping
// reconciles could each spawn a wl-mirror for the same target and then drop one
// pid on a last-writer-wins save, leaking an untrackable fullscreen overlay.
// the lock is best-effort: if it cannot be acquired fn still runs, since a
// mirror hiccup must never wedge an apply
func withMirrorLock(dir string, fn func() error) error {
	if err := os.MkdirAll(dir, profileDirMode); err != nil {
		return fn()
	}
	f, err := os.OpenFile(filepath.Join(dir, "mirrors.lock"), os.O_CREATE|os.O_RDWR, profileFileMode)
	if err != nil {
		return fn()
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fn()
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// reconcileMirrors brings the running wl-mirror processes in line with the
// mirror relationships described by monitors. it is wired into applyMonitors so
// the TUI apply and the headless `--profile` apply drive it through one path.
// errors are aggregated and returned non-fatally: a layout that applied cleanly
// must not be rolled back just because a mirror could not be (re)started
func reconcileMirrors(monitors []Monitor) error {
	dir := mirrorStateDir()
	desired := desiredMirrors(monitors)

	// wl-mirror missing: we can neither start mirrors nor reliably reason about
	// processes. best-effort tear down whatever we tracked, and warn only if the
	// user actually wanted a mirror this apply. SIGTERM is idempotent and we
	// only ever write an empty state here, so this path needs no lock
	if !mirrorAvailable() {
		state, _ := loadMirrorState(dir)
		for _, p := range state {
			_ = stopMirror(p)
		}
		if len(desired) > 0 {
			fmt.Fprintf(os.Stderr,
				"warning: niri has no native mirroring and wl-mirror was not found in PATH; "+
					"install wl-mirror to enable it (%d mirror(s) not applied)\n", len(desired))
		}
		if len(state) > 0 {
			return saveMirrorState(dir, nil)
		}
		return nil
	}

	// fast path: nothing desired and nothing tracked means no process work and
	// no state to clear, so skip the lock and the runtime dir creation entirely
	// (the common case for users who never mirror)
	if len(desired) == 0 {
		if state, _ := loadMirrorState(dir); len(state) == 0 {
			return nil
		}
	}

	return withMirrorLock(dir, func() error {
		// the authoritative state load happens under the lock; a corrupt file
		// must not wedge applies, so fall back to empty and respawn what's wanted
		state, err := loadMirrorState(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read mirror state: %v\n", err)
			state = nil
		}

		plan := diffMirrors(desired, state, pidAlive)

		var errs []string
		for _, p := range plan.Stop {
			if err := stopMirror(p); err != nil {
				errs = append(errs, err.Error())
			}
		}

		live := append([]mirrorProc{}, plan.Keep...)
		for _, s := range plan.Start {
			p, err := startMirror(s)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			live = append(live, p)
		}

		sort.Slice(live, func(i, j int) bool { return live[i].Target < live[j].Target })
		if len(live) > 0 || len(state) > 0 {
			if err := saveMirrorState(dir, live); err != nil {
				errs = append(errs, fmt.Sprintf("save mirror state: %v", err))
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	})
}

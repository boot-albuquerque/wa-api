package core

// The first production boot path of this module: restoring an
// already-paired profile to a defined READY, or failing with a classified
// cause and a deterministic teardown.
//
// This is RESTORATION ONLY (CAP-05, slice 1). QR pairing is a separate,
// human-authorised slice: nothing here shows a QR code, waits for one, or
// mutates an unpaired profile. A page that is not already APP_READY after
// navigation is a boot failure here, not a state this path knows how to
// advance.
//
// Every step below composes an existing, individually tested mechanism from
// engine/ and spa/ (CLAUDE.md: "compose, do not reinvent"). What is new here
// is the SEQUENCE and the STATE THAT OUTLIVES A COMMAND — the ownership guard
// and the Session handle — which is exactly what core/doc.go already
// promised this package would hold.
//
//	Start
//	  -> ownership guard (one profile, one active owner in this process)
//	  -> engine.Launcher.Launch (profile reclaim, process, DevTools endpoint)
//	  -> engine.OpenTab (which primes the target — see engine/prime.go)
//	  -> navigate to the SPA
//	  -> spa.Probe / spa.Classify
//	  -> spa.VerifyInventory(spa.RequiredAtStartup)
//	  -> READY
//
// On failure at ANY stage from Launch onward, engine.CleanStop tears the
// browser down deterministically, recording StopVia and marking the profile
// suspect when the stop went dirty (engine/shutdown.go, engine/suspect.go).
//
// HANDOFF-INICIATIVA.md section 6, invariant 2, requires more than that for a
// profile that is ALREADY suspect when this path starts: "a sessão então é
// SUSPEITA — a ser verificada em vez de presumida boa" ["the session is then
// SUSPECT — to be verified rather than presumed good"]. StartSession reads
// that marker before launching and, when this boot reaches a verified READY
// (structural probe AND module inventory both pass), clears it — the boot
// path itself IS the verification the invariant asks for. A profile whose
// marker cannot even be read is refused before a browser is started, on the
// same "cannot declare healthy what cannot be read" principle written on
// engine.SessionSuspect's own doc comment.
import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// BootStage names where in the boot sequence a failure happened. Invariant
// 11 requires every session death to carry a classified cause; this is that
// classification for the boot path.
type BootStage string

const (
	// StageConfig means the caller's StartConfig was incomplete.
	StageConfig BootStage = "config"
	// StageOwnership means another active session in this process already
	// owns this profile.
	StageOwnership BootStage = "ownership"
	// StageSuspectRead means the profile's suspect marker could not be read.
	StageSuspectRead BootStage = "suspect_read"
	// StageLaunch means the browser process never came up.
	StageLaunch BootStage = "launch"
	// StageOpenTab means the tab could not be attached/primed.
	StageOpenTab BootStage = "open_tab"
	// StageNavigate means navigation to the SPA failed.
	StageNavigate BootStage = "navigate"
	// StageNotReady means the page answered but did not classify APP_READY.
	// Restoration only: a QR/login-required/conflict/error page is a failure
	// of THIS boot path, not a state it advances through.
	StageNotReady BootStage = "not_ready"
	// StageInventory means the required SPA module inventory did not resolve.
	StageInventory BootStage = "inventory"
)

// ErrProfileAlreadyOwned is returned when a profile already has an active
// session owner in this process.
//
// "1 WhatsApp profile = 1 active browser/session owner" is the invariant
// measured 3/3 in HANDOFF-INICIATIVA.md section 2.3. This is the in-process
// half of it: the cross-process half is engine.ReclaimProfile refusing to
// launch on a profile whose SingletonLock names a live pid (engine/profile.go).
// Guarding only at that layer leaves a window HERE: two goroutines in the
// same process could both pass ReclaimProfile before either process starts,
// because the lock is created by Chromium itself only once the process
// actually launches. The guard below closes that window with the resource
// core/ was always meant to hold — state that outlives a single command.
var ErrProfileAlreadyOwned = errors.New("core: profile already has an active session owner in this process")

var (
	ownershipMu   sync.Mutex
	ownedProfiles = map[string]bool{}
)

// acquireOwnership claims exclusive in-process ownership of an (absolute)
// profile directory, or reports that somebody already holds it.
//
// The returned release func MUST be called exactly once, on every exit path
// — success or failure — once ownership is no longer needed. It is deliberately
// not idempotent by itself; Session.Stop makes it idempotent for callers.
func acquireOwnership(profileDir string) (release func(), err error) {
	ownershipMu.Lock()
	defer ownershipMu.Unlock()
	if ownedProfiles[profileDir] {
		return nil, fmt.Errorf("%w: %s", ErrProfileAlreadyOwned, profileDir)
	}
	ownedProfiles[profileDir] = true
	return func() {
		ownershipMu.Lock()
		delete(ownedProfiles, profileDir)
		ownershipMu.Unlock()
	}, nil
}

// BootFailure is a boot that did not reach READY.
//
// It always names the stage (invariant 11: no session death without a
// classified cause) and, once a browser process was actually started,
// how it went down.
type BootFailure struct {
	Stage      BootStage
	Cause      error
	StoppedVia engine.StopVia
	// WasSuspect records whether the profile's suspect marker was set before
	// this boot attempt started.
	WasSuspect bool
}

func (e *BootFailure) Error() string {
	if e.StoppedVia != "" {
		return fmt.Sprintf("core: boot failed at %s (stopped_via=%s): %v", e.Stage, e.StoppedVia, e.Cause)
	}
	return fmt.Sprintf("core: boot failed at %s: %v", e.Stage, e.Cause)
}

// Unwrap lets callers errors.As/errors.Is through to the underlying cause —
// e.g. *spa.ErrModulesMissing at StageInventory.
func (e *BootFailure) Unwrap() error { return e.Cause }

// StartConfig is everything StartSession needs to restore an already-paired
// profile to a READY session.
type StartConfig struct {
	// BinaryPath is the Chromium executable.
	BinaryPath string
	// ProfileDir is the persistent, already-paired profile to restore.
	ProfileDir string
	// DebuggingPort is the DevTools port this browser will listen on.
	DebuggingPort int
	// UserAgent overrides the browser's identity. Required against the real
	// target (engine/flags.go LaunchConfig.UserAgent doc).
	UserAgent string
	// NavigateURL is the SPA to restore into (web.whatsapp.com in production;
	// a local fixture in tests, per the module's own convention of never
	// touching the real target from ordinary tests).
	NavigateURL string
	// RequiredModules overrides spa.RequiredAtStartup. Nil uses the default.
	RequiredModules []spa.Module
	// Runner overrides the default engine.Runner (measured deadlines, tracing
	// on). Nil builds one with engine.NewRunner.
	Runner *engine.Runner
	// Hostname and AllowForeignProfile pass through to engine.Launcher; see
	// their docs on engine.ReclaimOptions. Leave AllowForeignProfile false
	// unless single ownership is proven by a lease outside this package.
	Hostname            string
	AllowForeignProfile bool
}

// Session is a live, READY headless session — the state that outlives a
// single command, per core/doc.go.
type Session struct {
	browser    *engine.Browser
	tab        *engine.Tab
	runner     *engine.Runner
	profileDir string
	release    func()

	mu      sync.Mutex
	stopped bool
}

// Browser exposes the underlying browser handle, e.g. for Browser.PID().
func (s *Session) Browser() *engine.Browser { return s.browser }

// Tab exposes the attached tab, for a capability layer to drive it under the
// caller's own Runner.Do budgets.
func (s *Session) Tab() *engine.Tab { return s.tab }

// ProfileDir is the absolute profile directory this session owns.
func (s *Session) ProfileDir() string { return s.profileDir }

// Stop tears the session down through the module's one shutdown path
// (engine.CleanStop) and releases ownership of the profile.
//
// Safe to call more than once and from any goroutine; only the first call
// performs the stop. A second call returns engine.StopViaNoop, matching what
// CleanStop itself does for a nil process.
func (s *Session) Stop(ctx context.Context) engine.StopVia {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return engine.StopViaNoop
	}
	s.stopped = true
	if s.tab != nil {
		s.tab.Close()
	}
	via := engine.CleanStop(ctx, s.runner, s.browser)
	if s.release != nil {
		s.release()
	}
	return via
}

// StartSession restores an already-paired profile to a defined READY, or
// fails with a classified *BootFailure and a deterministic teardown.
//
// QR pairing is explicitly out of scope: a page that is not APP_READY after
// navigation is StageNotReady, full stop. There is no fallback to a pairing
// flow here — that is a separate, human-authorised slice.
func StartSession(ctx context.Context, cfg StartConfig) (*Session, error) {
	if cfg.BinaryPath == "" {
		return nil, &BootFailure{Stage: StageConfig, Cause: fmt.Errorf("core: BinaryPath is required")}
	}
	if cfg.ProfileDir == "" {
		return nil, &BootFailure{Stage: StageConfig, Cause: fmt.Errorf("core: ProfileDir is required")}
	}
	if cfg.NavigateURL == "" {
		return nil, &BootFailure{Stage: StageConfig, Cause: fmt.Errorf("core: NavigateURL is required")}
	}

	absProfile, err := filepath.Abs(cfg.ProfileDir)
	if err != nil {
		return nil, &BootFailure{Stage: StageConfig, Cause: fmt.Errorf("core: resolving ProfileDir: %w", err)}
	}

	release, err := acquireOwnership(absProfile)
	if err != nil {
		return nil, &BootFailure{Stage: StageOwnership, Cause: err}
	}

	// Invariant 2: a profile marked suspect by a prior dirty stop must be
	// VERIFIED, not presumed good. A marker we cannot read is a profile we
	// cannot declare healthy either way (engine.SessionSuspect's own rule),
	// so that failure is refused here, before any browser is started.
	wasSuspect, _, suspErr := engine.SessionSuspect(cfg.ProfileDir)
	if suspErr != nil {
		release()
		return nil, &BootFailure{
			Stage: StageSuspectRead,
			Cause: fmt.Errorf("core: reading the session-suspect marker: %w", suspErr),
		}
	}

	runner := cfg.Runner
	if runner == nil {
		runner = engine.NewRunner()
	}

	launcher := &engine.Launcher{
		BinaryPath:          cfg.BinaryPath,
		Runner:              runner,
		Hostname:            cfg.Hostname,
		AllowForeignProfile: cfg.AllowForeignProfile,
	}

	browser, err := launcher.Launch(ctx, engine.LaunchConfig{
		ProfileDir:    cfg.ProfileDir,
		DebuggingPort: cfg.DebuggingPort,
		UserAgent:     cfg.UserAgent,
	})
	if err != nil {
		release()
		// Launch itself already ran CleanStop internally on a half-started
		// process (engine/launcher.go); there is no live process left here
		// for this path to tear down a second time.
		return nil, &BootFailure{Stage: StageLaunch, Cause: err, WasSuspect: wasSuspect}
	}

	// From here on, a browser process exists. Every remaining failure path
	// MUST go through the same teardown: engine.CleanStop, which records
	// StopVia and marks the profile suspect on a dirty stop. fail is the one
	// place that happens, so no later branch can forget it.
	fail := func(stage BootStage, cause error) (*Session, error) {
		via := engine.CleanStop(context.Background(), runner, browser)
		release()
		return nil, &BootFailure{Stage: stage, Cause: cause, StoppedVia: via, WasSuspect: wasSuspect}
	}

	tab, err := engine.OpenTab(ctx, browser)
	if err != nil {
		return fail(StageOpenTab, fmt.Errorf("core: opening tab: %w", err))
	}

	if err := tab.Navigate(runner, cfg.NavigateURL, "core/start/navigate"); err != nil {
		tab.Close()
		return fail(StageNavigate, fmt.Errorf("core: navigating to the SPA: %w", err))
	}

	// A single post-navigate probe fires as early as T+4.7s against the real
	// SPA, well before the page finishes mounting (ARMADILHAS.md, boot
	// classifies once). spa.WaitForReady replaces that single shot with a
	// Go-side settle loop, bounded by spa.DefaultSettleBudget and by ctx
	// itself — whichever expires first — so a page that is still mounting is
	// waited for instead of rejected, while a page that answers with a
	// terminal class (QR, session conflict, ...) still fails fast.
	snap, class := spa.WaitForReady(ctx, runner, tab.Evaluate, spa.DefaultSettleBudget, "core/start/probe")
	if class != spa.ClassAppReady {
		tab.Close()
		return fail(StageNotReady, fmt.Errorf(
			"core: page classified %q, want %q; this boot path is restoration-only — "+
				"pairing is a separate, human-authorised slice (snapshot url=%q dom_nodes=%d)",
			class, spa.ClassAppReady, snap.URL, snap.DOMNodes))
	}

	required := cfg.RequiredModules
	if required == nil {
		required = spa.RequiredAtStartup
	}
	if err := spa.VerifyInventory(ctx, runner, tab.Evaluate, required); err != nil {
		tab.Close()
		return fail(StageInventory, err)
	}

	// READY, verified: structural probe AND module inventory both passed on
	// THIS boot. That is what discharges invariant 2 for a suspect profile —
	// clearing the marker is safe exactly because the check above just
	// happened, not because time passed.
	if wasSuspect {
		_ = engine.ClearSessionSuspect(cfg.ProfileDir)
		// A failure to clear is not a boot failure: the session IS ready and
		// verified. Worst case the marker survives and the next boot verifies
		// again, which is strictly safe, only slightly redundant.
	}

	return &Session{
		browser:    browser,
		tab:        tab,
		runner:     runner,
		profileDir: absProfile,
		release:    release,
	}, nil
}

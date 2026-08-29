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
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
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
	// PageClass is what the page looked like when the boot gave up, from spa's
	// closed vocabulary, and it is EMPTY when the boot died before there was a
	// page to classify.
	//
	// IT IS NOT DERIVABLE FROM Stage. A boot that dies at StageNotReady against a
	// QR screen and one that dies at StageNotReady against an unresponsive page
	// carry the same stage for opposite reasons — and the first is what the
	// upstream calls an authentication failure. The class was inside Cause's
	// message, where nothing structured could read it.
	PageClass spa.PageClass
	// WasSuspect records whether the profile's suspect marker was set before
	// this boot attempt started.
	WasSuspect bool
	// PID is the browser process this attempt launched, or zero when it failed
	// before launching one.
	//
	// FOR CORRELATION, NEVER FOR ACTION — and the distinction is what keeps this
	// field from contradicting H23, which REFUSED to hand out a pid once it
	// stopped meaning anything. That refusal was about a pid a caller might
	// SIGNAL: operating systems reuse pids, so acting on a stale one reaches
	// whatever inherited the number.
	//
	// This pid is stale BY CONSTRUCTION — a BootFailure only exists after the
	// browser was torn down — and it is here so a failure can be matched
	// against the operating system's own record of that process. Nothing in
	// this module signals it, and a caller that does has misread the field.
	PID int
}

func (e *BootFailure) Error() string {
	// The pid is omitted when it is zero rather than printed as "pid=0". A
	// failure before launch has no process, and "pid=0" reads like one.
	var pid string
	if e.PID != 0 {
		pid = fmt.Sprintf(" pid=%d", e.PID)
	}
	if e.StoppedVia != "" {
		return fmt.Sprintf("core: boot failed at %s (stopped_via=%s%s): %v",
			e.Stage, e.StoppedVia, pid, e.Cause)
	}
	return fmt.Sprintf("core: boot failed at %s%s: %v", e.Stage, pid, e.Cause)
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
	// SettleBudget overrides spa.DefaultSettleBudget: how long the boot waits
	// for the SPA to finish mounting before giving up. Zero uses the default.
	//
	// It exists for the negative paths. A test that proves "this page never
	// becomes ready" has to pay the whole budget to prove it, and at the
	// 60s default that one property was most of the package's runtime (H20:
	// the suite went from 28s to 201s after the settle loop landed). The
	// budget being a PARAMETER also makes the better property testable —
	// that the loop respects the budget it was GIVEN, rather than that it
	// happens to wait 60s.
	//
	// Production callers should leave this zero. It is not an SLA, and a
	// caller wanting a shorter boot should bound ctx instead, which the loop
	// already honours and which does not hide a slow mount as "not ready".
	SettleBudget time.Duration
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
	// OnLifecycle receives this session's lifecycle facts. Nil is the ordinary
	// case; see core/lifecycle.go for why this is a callback and not a bus.
	OnLifecycle LifecycleObserver
	// AcceptClasses overrides which spa.PageClass values this boot accepts as
	// READY instead of the restoration-only default of {ClassAppReady}. Nil
	// or empty keeps that default — every existing StartSession caller is
	// unaffected. StartPairingSession is the only caller that sets this.
	//
	// It exists so ONE boot sequence (ownership guard, launch, tab, navigate,
	// settle, teardown-on-failure) serves both restoration and pairing,
	// instead of a second copy of ~300 lines diverging from this one the
	// first time either is touched. What differs between the two is only
	// WHICH classes are a success — never the mechanics of getting there.
	AcceptClasses []spa.PageClass
}

// acceptsClass reports whether class is one of accepted, or ClassAppReady
// when accepted is empty (the restoration-only default).
func acceptsClass(class spa.PageClass, accepted []spa.PageClass) bool {
	for _, c := range acceptedOrDefault(accepted) {
		if class == c {
			return true
		}
	}
	return false
}

// acceptedOrDefault names the set acceptsClass checks against, for both the
// check itself and the failure message — one definition, so the error a
// caller reads can never name a different set than the one that refused it.
func acceptedOrDefault(accepted []spa.PageClass) []spa.PageClass {
	if len(accepted) == 0 {
		return []spa.PageClass{spa.ClassAppReady}
	}
	return accepted
}

// Session is a live, READY headless session — the state that outlives a
// single command, per core/doc.go.
type Session struct {
	browser    *engine.Browser
	tab        *engine.Tab
	runner     *engine.Runner
	profileDir string
	release    func()

	// cancelSession ends the context the tab was opened under. It is NOT the
	// caller's boot context — see the sessionCtx block in StartSession — so
	// the only thing that ends a live session is Stop.
	cancelSession context.CancelFunc

	// wasSuspect records that this profile carried a suspect marker when the
	// boot that produced this session started. It is reported with the ready
	// fact, because a ready that RECOVERED a suspect profile is a different
	// thing from an ordinary one.
	wasSuspect bool
	// onLifecycle is the observer this session was started with, so a Stop
	// reaches the same listener a ready did.
	onLifecycle LifecycleObserver

	mu      sync.Mutex
	stopped bool
}

// Browser exposes the underlying browser handle, e.g. for Browser.PID().
func (s *Session) Browser() *engine.Browser { return s.browser }

// Tab exposes the attached tab, for a capability layer to drive it under the
// caller's own Runner.Do budgets.
func (s *Session) Tab() *engine.Tab { return s.tab }

// ProcessAlive reports whether the browser PROCESS this session owns is still
// running. It answers nothing else, and the name is deliberately the narrowest
// true thing rather than Healthy or Alive.
//
// Item 12 of this initiative's briefing is the reason for that narrowness:
// process, target, service worker, socket, SPA, session and identity are
// DIFFERENT signals, and this module has already paid for conflating them. A
// live process can host a hung renderer, a socket in OPENING, or a page that
// logged itself out. A caller that needs one of those must ask for that one.
//
// What it IS good for is the cheapest, least ambiguous negative: if the process
// is gone, nothing above it can be true, so a holder can refuse to hand out the
// session without probing anything. That is the case it exists for (H21).
//
// A stopped session reports false: Stop clears the browser handle, and a
// session that has been stopped has no process by definition.
func (s *Session) ProcessAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.browser == nil {
		return false
	}
	return engine.ProcessAlive(s.browser.PID())
}

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
	if s.cancelSession != nil {
		s.cancelSession()
	}
	if s.release != nil {
		s.release()
	}
	obs := s.onLifecycle
	s.mu.Unlock()
	// OUTSIDE THE LOCK, and the manual Unlock above is why the deferred one is
	// not used on this path. An observer that calls back into Stop — which a
	// "the session died, tear everything down" handler does on its first day —
	// would deadlock against the mutex this function still held.
	emit(obs, stopFact(via))
	s.mu.Lock()
	return via
}

// StartSession restores an already-paired profile to a defined READY, or
// fails with a classified *BootFailure and a deterministic teardown.
//
// QR pairing is explicitly out of scope: a page that is not APP_READY after
// navigation is StageNotReady, full stop. There is no fallback to a pairing
// flow here — that is a separate, human-authorised slice.
func StartSession(ctx context.Context, cfg StartConfig) (*Session, error) {
	sess, err := startSession(ctx, cfg)
	if err != nil {
		// EVERY FAILURE PATH REPORTS, and this wrapper is why. The boot has a
		// dozen returns and a fact emitted at each of them would be a dozen
		// places to forget one — which is the same reasoning that put the
		// teardown behind a single fail().
		f := LifecycleFact{Phase: PhaseBootFailed}
		var bf *BootFailure
		if errors.As(err, &bf) {
			f.Reason, f.WasSuspect = string(bf.Stage), bf.WasSuspect
			f.PageClass = string(bf.PageClass)
		}
		emit(cfg.OnLifecycle, f)
		return nil, err
	}
	sess.onLifecycle = cfg.OnLifecycle
	emit(cfg.OnLifecycle, LifecycleFact{Phase: PhaseReady, WasSuspect: sess.wasSuspect})
	return sess, nil
}

// StartPairingSession is StartSession's counterpart for the human-authorised
// pairing slice StartSession's own doc comment names and refuses: it boots a
// profile that may show a QR code instead of restoring an already-paired one.
//
// It is additive, not a relaxation of StartSession: every existing caller of
// StartSession is unaffected (cfg.AcceptClasses stays nil there, so the
// restoration-only check is unchanged), and this function is the ONLY place
// in the package that widens it. A caller reaches for this deliberately —
// there is no flag on StartSession that turns it on by accident.
//
// Accepted page classes are spa.ClassAppReady (the profile turned out to
// already be paired — a fine outcome, not just a fallback), spa.
// ClassLoginRequired (a QR is on screen) and spa.ClassPairingLoading (the
// pairing screen mounted but the QR has not rendered yet — spa.settleTerminal
// treats this as terminal too, so WaitForReady returns here rather than
// spending the whole settle budget spinning on it; a caller reading the QR
// polls the page itself for when Conn.ref populates, same as the reference's
// own change:ref listener does asynchronously).
//
// cfg.RequiredModules defaults to an EMPTY (non-nil) slice here, not
// spa.RequiredAtStartup: the modules that list names are what an APP_READY
// chat surface needs, and a QR screen has none of them mounted yet.
// spa.VerifyInventory treats an empty list as trivially satisfied — see its
// own len(modules)==0 check — which is the correct answer for "nothing is
// required yet", not the loosened one.
func StartPairingSession(ctx context.Context, cfg StartConfig) (*Session, error) {
	if len(cfg.AcceptClasses) == 0 {
		cfg.AcceptClasses = []spa.PageClass{spa.ClassAppReady, spa.ClassLoginRequired, spa.ClassPairingLoading}
	}
	if cfg.RequiredModules == nil {
		cfg.RequiredModules = []spa.Module{}
	}
	sess, err := startSession(ctx, cfg)
	if err != nil {
		f := LifecycleFact{Phase: PhaseBootFailed}
		var bf *BootFailure
		if errors.As(err, &bf) {
			f.Reason, f.WasSuspect = string(bf.Stage), bf.WasSuspect
			f.PageClass = string(bf.PageClass)
		}
		emit(cfg.OnLifecycle, f)
		return nil, err
	}
	sess.onLifecycle = cfg.OnLifecycle
	emit(cfg.OnLifecycle, LifecycleFact{Phase: PhaseReady, WasSuspect: sess.wasSuspect})
	return sess, nil
}

func startSession(ctx context.Context, cfg StartConfig) (*Session, error) {
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
	// failClass is what the page looked like, set only where there IS a page to
	// classify. It is a variable rather than a parameter because fail() has a
	// dozen callers and eleven of them have no class to pass; threading an empty
	// string through all of them would put the noise where the information is not.
	var failClass spa.PageClass
	fail := func(stage BootStage, cause error) (*Session, error) {
		// Read before the stop for readability, NOT for correctness — and
		// saying so is a correction. This comment first claimed the order was
		// load-bearing; the negative control that inverted it PASSED, which is
		// what exposed the claim as wrong. engine.Browser.PID() returns the
		// same number after a stop, which is precisely what H23 measured and
		// why that finding exists at all.
		pid := browser.PID()
		via := engine.CleanStop(context.Background(), runner, browser)
		release()
		return nil, &BootFailure{
			Stage: stage, Cause: cause, StoppedVia: via,
			WasSuspect: wasSuspect, PID: pid, PageClass: failClass,
		}
	}

	// THE SESSION'S LIFETIME IS NOT THE BOOT'S LIFETIME.
	//
	// engine.OpenTab derives the chromedp allocator from the context it is
	// given, so whatever context goes in here becomes the tab's — and
	// therefore the session's — lifetime. Passing the caller's ctx made the
	// returned *Session die the moment the caller released its BOOT deadline,
	// which is what every correct Go caller does with `defer cancel()`. It was
	// measured against the real paired profile: every probe on a successfully
	// booted session answered "context canceled" for 76s straight
	// (ARMADILHAS.md; TestStartSession_SessionOutlivesItsBootContext).
	//
	// A caller that grants a 60s budget is asking for a 60s BOOT, not a 60s
	// SESSION. So the tab is opened under a context this package owns, and the
	// only thing that ends it is Stop.
	sessionCtx, cancelSession := context.WithCancel(context.Background())

	// Boot-time cancellation is still honoured, and must be: until the boot
	// returns, the caller's ctx is the abort signal for the whole attempt, and
	// tab.Navigate below runs under the TAB's context rather than under ctx,
	// so without this watcher a cancelled boot would keep navigating. The
	// watcher is torn down by bootDone on every exit path, successful or not,
	// which is what stops it from following ctx after the handover.
	bootDone := make(chan struct{})
	defer close(bootDone)
	go func() {
		select {
		case <-ctx.Done():
			cancelSession()
		case <-bootDone:
		}
	}()

	// fail() and the terminal tab.Close() paths below already tear the browser
	// down; cancelSession here releases the allocator with them, so no failed
	// boot leaks the context it opened.
	failTab := func(stage BootStage, cause error) (*Session, error) {
		cancelSession()
		return fail(stage, cause)
	}

	// THE INJECTED POLICY HAS TO GOVERN THE WHOLE BOOT, and this line used to be
	// the hole in that.
	//
	// engine.OpenTab hard-codes DefaultDeadlines.For(OpBoot), so a caller that
	// raised Runner.Policy.Boot — which is exactly what the test harness does,
	// and what harnessbudget_test.go documents — had every step of the boot
	// honour it EXCEPT the tab priming. Under host contention that step is
	// precisely the one that runs long: F100 recorded eight gate failures, and
	// the ninth named the cause, failing with "Boot(open_tab/prime): deadline of
	// 30s exceeded" on a session configured for ninety.
	//
	// OpenTabWithin is the same function with the bound passed in rather than
	// assumed. Production behaviour is unchanged: engine.NewRunner starts from
	// DefaultDeadlines, so a caller that sets nothing gets the same 30s.
	tab, err := engine.OpenTabWithin(sessionCtx, browser, runner.Policy.For(engine.OpBoot))
	if err != nil {
		cancelSession()
		return fail(StageOpenTab, fmt.Errorf("core: opening tab: %w", err))
	}

	if err := tab.Navigate(runner, cfg.NavigateURL, "core/start/navigate"); err != nil {
		tab.Close()
		return failTab(StageNavigate, fmt.Errorf("core: navigating to the SPA: %w", err))
	}

	// A single post-navigate probe fires as early as T+4.7s against the real
	// SPA, well before the page finishes mounting (ARMADILHAS.md, boot
	// classifies once). spa.WaitForReady replaces that single shot with a
	// Go-side settle loop, bounded by spa.DefaultSettleBudget and by ctx
	// itself — whichever expires first — so a page that is still mounting is
	// waited for instead of rejected, while a page that answers with a
	// terminal class (QR, session conflict, ...) still fails fast.
	settleBudget := cfg.SettleBudget
	if settleBudget <= 0 {
		settleBudget = spa.DefaultSettleBudget
	}
	snap, class := spa.WaitForReady(ctx, runner, tab.Evaluate, settleBudget, "core/start/probe")
	if !acceptsClass(class, cfg.AcceptClasses) {
		tab.Close()
		failClass = class
		return failTab(StageNotReady, fmt.Errorf(
			"core: page classified %q, want one of %v; this boot path accepts only that set "+
				"(snapshot url=%q dom_nodes=%d)",
			class, acceptedOrDefault(cfg.AcceptClasses), snap.URL, snap.DOMNodes))
	}

	required := cfg.RequiredModules
	if required == nil {
		required = spa.RequiredAtStartup
	}
	if err := spa.VerifyInventory(ctx, runner, tab.Evaluate, required); err != nil {
		tab.Close()
		return failTab(StageInventory, err)
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
		browser:       browser,
		tab:           tab,
		runner:        runner,
		cancelSession: cancelSession,
		profileDir:    absProfile,
		release:       release,
		wasSuspect:    wasSuspect,
	}, nil
}

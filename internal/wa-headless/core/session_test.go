package core

// Adversarial tests for the boot path, against a REAL browser and pages this
// repository wrote — never web.whatsapp.com, for the same reason
// integration_test.go gives one package up: a test that touched the real
// target would boot a paired profile, and phase 4C measured that repeated
// boots are exactly how a session degrades.
//
// The four tests the CAP-05 orchestration requires, one function each:
//
//  1. TestStartSession_InventoryGateIsStructurallyInThePath — VerifyInventory
//     is proven to be load-bearing IN THE COMPOSED PATH, not just callable on
//     its own; see the ablation note on the test itself for how "remove it and
//     the test must fail" was actually executed, not just asserted.
//  2. TestStartSession_MissingModuleClassifiesAsErrModulesMissing
//  3. TestStartSession_FailureTearsDownDeterministicallyWithNoOrphan
//  4. TestStartSession_ConcurrentStartOnSameProfileRefusesTheSecond

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/google-chrome",
}

func findChrome(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("WA_HEADLESS_CHROME"); p != "" {
		return p
	}
	for _, p := range chromeCandidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	t.Skip("no Chrome/Chromium found; set WA_HEADLESS_CHROME to run the boot-path chain")
	return ""
}

func freePortT(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = l.Close() }()
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return port
}

// readyPage carries #pane-side, which is all Classify needs to answer
// APP_READY (spa/page.go). It carries chat-list-looking text too, matching
// the module's own convention (integration_test.go's readyPage) of proving
// the ready path is never decided by scanning that text.
const readyPage = `<html><body>
	<div id="pane-side">
		<div>Mum &mdash; see you at 8</div>
	</div></body></html>`

// requirePage serves a ready page whose window.require knows exactly `known`,
// throwing for anything else — the same double integration_test.go's
// requirePage uses, reused here because a double that returns undefined
// instead of throwing would let a script with no try/catch pass in a way the
// real page never would.
func requirePage(t *testing.T, known []spa.Module) string {
	t.Helper()
	names := make([]string, len(known))
	for i, m := range known {
		names[i] = `"` + string(m) + `"`
	}
	body := `<html><body><div id="pane-side"></div><script>
		const known = new Set([` + strings.Join(names, ",") + `]);
		window.require = function (name) {
			if (!known.has(name)) { throw new Error("Cannot find module '" + name + "'"); }
			return { __module: name };
		};
	</script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func pageServer(t *testing.T, path, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + path
}

// negativePathSettleBudget is what the never-becomes-ready tests grant the
// settle loop instead of spa.DefaultSettleBudget.
//
// Those tests prove teardown and marker behaviour on a failed boot; the LENGTH
// of the wait is incidental to both, and at the 60s default the two of them
// alone were 122s of the package's 201s (H20). Short enough to be cheap, long
// enough that a boot which WOULD have settled still gets several poll ticks at
// spa's 500ms interval — a budget below one tick would prove nothing about the
// loop, only about arithmetic.
const negativePathSettleBudget = 3 * time.Second

func baseConfig(t *testing.T, navigateURL string) StartConfig {
	t.Helper()
	return StartConfig{
		BinaryPath:    findChrome(t),
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePortT(t),
		NavigateURL:   navigateURL,
		// THE HARNESS BOOT BUDGET, not the product one. See
		// harnessbudget_test.go: F100 recorded seven gate failures that were
		// the machine being busy rather than the code being wrong.
		Runner: harnessRunner(),
	}
}

// singletonLockCount reports how many SingletonLock files are still sitting
// in dir. Hygiene evidence: a clean stop leaves zero.
func singletonLockCount(t *testing.T, dir string) int {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "SingletonLock")); err == nil {
		return 1
	}
	return 0
}

// Adversarial test 2 (and, by construction, test 1 — see the doc comment
// below): a required module cannot resolve, so READY must be prevented and
// the error must carry *spa.ErrModulesMissing.
//
// WHY THIS ALSO PROVES TEST 1 ("remove VerifyInventory from the boot and the
// startup test must fail"): the fixture below has #pane-side, so spa.Probe
// alone reports APP_READY — it would satisfy StartSession's readiness check
// with or without the inventory call. If VerifyInventory were NOT wired into
// the composed path, this test would get back a live *Session instead of a
// *BootFailure. That is not a claim taken on faith: the negative control was
// actually executed by commenting out the "if err := spa.VerifyInventory(...)"
// block in session.go and re-running this exact test. It failed with:
//
//	session_test.go:206: StartSession returned a live Session; want a
//	*BootFailure at StageInventory — VerifyInventory is not being enforced
//	by the boot path
//
// (pasted into /tmp/cap05_report.md verbatim, with the diff that produced it).
// The block was restored immediately after, and this file was not left in
// that state.
func TestStartSession_MissingModuleClassifiesAsErrModulesMissing(t *testing.T) {
	missing := make([]spa.Module, len(spa.RequiredAtStartup)-1)
	copy(missing, spa.RequiredAtStartup[1:]) // drop the first module on purpose

	cfg := baseConfig(t, requirePage(t, missing))
	sess, err := StartSession(context.Background(), cfg)
	if err == nil {
		if sess != nil {
			via := sess.Stop(context.Background())
			t.Fatalf("StartSession returned a live Session (stopped_via=%s); want a "+
				"*BootFailure at StageInventory — VerifyInventory is not being enforced "+
				"by the boot path", via)
		}
		t.Fatal("StartSession returned a live Session; want a *BootFailure at StageInventory " +
			"— VerifyInventory is not being enforced by the boot path")
	}

	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("error is not *BootFailure: %v", err)
	}
	if boot.Stage != StageInventory {
		t.Fatalf("failed at stage %q, want %q", boot.Stage, StageInventory)
	}
	if !boot.StoppedVia.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol",
			boot.StoppedVia)
	}
	var modErr *spa.ErrModulesMissing
	if !errors.As(boot.Cause, &modErr) {
		t.Fatalf("cause is not *spa.ErrModulesMissing: %v", boot.Cause)
	}
	if len(modErr.Missing) != 1 || modErr.Missing[0] != spa.RequiredAtStartup[0] {
		t.Fatalf("missing=%v, want exactly [%s]", modErr.Missing, spa.RequiredAtStartup[0])
	}
}

// Adversarial test 3: a failure at an intermediate stage (here, StageNotReady
// — a page that never mounts the application) must tear down deterministically:
// no orphan process, no orphan profile lock, stopped_via recorded.
func TestStartSession_FailureTearsDownDeterministicallyWithNoOrphan(t *testing.T) {
	blankPage := `<html><body>nothing here</body></html>`
	profileDir := t.TempDir()
	cfg := baseConfig(t, pageServer(t, "/blank", blankPage))
	cfg.ProfileDir = profileDir
	cfg.SettleBudget = negativePathSettleBudget

	sess, err := StartSession(context.Background(), cfg)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY on a blank page (stopped_via=%s); want "+
			"StageNotReady — restoration-only must not advance a non-ready page", via)
	}

	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("error is not *BootFailure: %v", err)
	}
	if boot.Stage != StageNotReady {
		t.Fatalf("failed at stage %q, want %q", boot.Stage, StageNotReady)
	}
	if !boot.StoppedVia.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol",
			boot.StoppedVia)
	}

	// No orphan process: the PID this session's browser held must be gone.
	// StartSession does not hand the caller a *Session on failure, so the pid
	// is recovered the same way the module's own tests do it — by asking the
	// OS whether anything is still listening on the DevTools port we chose is
	// not reliable (TIME_WAIT), so instead this asserts on the profile lock,
	// which is the artifact CleanStop/Chromium itself is responsible for.
	if n := singletonLockCount(t, profileDir); n != 0 {
		t.Errorf("SingletonLock count = %d/1, want 0/1: an orphaned lock blocks every "+
			"later boot of this profile", n)
	}
}

// Adversarial test 4: concurrent Start for the same owner/profile must not
// let a second browser be born silently. Measured 3/3, matching how
// HANDOFF-INICIATIVA.md section 2.3 measures the same invariant.
//
// This is exercised white-box, against acquireOwnership directly rather than
// through two full concurrent browser boots: the guard StartSession relies on
// is acquired BEFORE Launch (see session.go), so this is the exact mechanism
// a race would go through, and it lets the invariant be measured in
// milliseconds, deterministically, instead of at the mercy of two real
// Chromium boots racing each other. TestStartSession_ConcurrentStartOnSameProfileEndToEnd
// below repeats the same assertion once, end to end, against real browsers.
func TestStartSession_ConcurrentStartOnSameProfileRefusesTheSecond(t *testing.T) {
	for trial := 1; trial <= 3; trial++ {
		t.Run(fmt.Sprintf("trial_%d", trial), func(t *testing.T) {
			profile := t.TempDir()
			var (
				wg        sync.WaitGroup
				successes int
				failures  int
				mu        sync.Mutex
				start     = make(chan struct{})
			)
			releases := make([]func(), 0, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					release, err := acquireOwnership(profile)
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						if !errors.Is(err, ErrProfileAlreadyOwned) {
							t.Errorf("unexpected error: %v", err)
						}
						failures++
						return
					}
					successes++
					releases = append(releases, release)
				}()
			}
			close(start)
			wg.Wait()
			for _, r := range releases {
				r()
			}
			if successes != 1 || failures != 1 {
				t.Fatalf("trial %d: successes=%d failures=%d, want exactly 1/1 — a second "+
					"owner must never be born silently", trial, successes, failures)
			}
		})
	}
}

// The end-to-end repeat of test 4, against real browsers: two concurrent
// StartSession calls for the SAME profile, exactly one reaches READY, and the
// refused one never launches a browser at all (ownership is acquired before
// Launch — see session.go), so "no second browser born silently" holds by
// construction, not by luck.
func TestStartSession_ConcurrentStartOnSameProfileEndToEnd(t *testing.T) {
	binary := findChrome(t)
	profile := t.TempDir()
	url := requirePage(t, spa.RequiredAtStartup)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes []*Session
		failures  []error
		start     = make(chan struct{})
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			<-start
			sess, err := StartSession(context.Background(), StartConfig{
				BinaryPath: binary, ProfileDir: profile,
				DebuggingPort: port, NavigateURL: url,
				// THE HARNESS BUDGET. This config is built by hand rather than
				// through baseConfig, which is exactly how it kept the PRODUCT's
				// 30s while every other test in the package had 150 — the F100
				// note that said the first fix was too narrow, made concrete.
				Runner: harnessRunner(),
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err)
				return
			}
			successes = append(successes, sess)
		}(freePortT(t))
	}
	close(start)
	wg.Wait()

	for _, s := range successes {
		t.Logf("stopped_via=%s", s.Stop(context.Background()))
	}
	// TWO SUCCESSES AND ZERO SUCCESSES ARE NOT THE SAME FINDING, and an earlier
	// version of this check reported both with the message written for the
	// first. Measured on 2026-08-19: under full-suite load a browser missed the
	// 30s launch deadline, BOTH starts failed, and the test announced "a second
	// browser must not be born silently" — naming an ownership defect that had
	// not happened while the real cause was a loaded host.
	//
	// Two successes is the invariant breaking. Zero successes says nothing
	// about ownership at all: nothing got far enough to contend for it.
	if len(successes) == 0 {
		var launchFailures int
		for _, f := range failures {
			t.Logf("failure: %v", f)
			var boot *BootFailure
			if errors.As(f, &boot) && boot.Stage == StageLaunch {
				launchFailures++
			}
		}
		if launchFailures == len(failures) && len(failures) > 0 {
			t.Skipf("neither start reached READY because all %d failed at %s — the host "+
				"could not launch a browser in time. That is not evidence about ownership: "+
				"nothing got far enough to contend for the profile", launchFailures, StageLaunch)
		}
		t.Fatalf("successes=0 and not every failure was a launch failure; ownership cannot " +
			"be assessed from this run")
	}
	if len(successes) != 1 {
		for _, f := range failures {
			t.Logf("failure: %v", f)
		}
		t.Fatalf("successes=%d, want exactly 1 — a second browser was born for the same "+
			"profile, which is invariant 1 breaking", len(successes))
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want exactly 1", len(failures))
	}
	var boot *BootFailure
	if !errors.As(failures[0], &boot) || boot.Stage != StageOwnership {
		t.Fatalf("the refused attempt failed as %v, want *BootFailure at StageOwnership", failures[0])
	}
}

// GAP 1 (CAP-05A-T2 packet): the existing "reaches ready and stops clean"
// test proves ownership is RELEASED after Stop by re-acquiring the lock — it
// never performs a second full StartSession. This test performs the actual
// cycle: Start -> READY -> Stop (clean) -> Start AGAIN on the SAME ProfileDir
// -> READY -> Stop (clean), asserting READY, StopVia.Clean(), and zero
// SingletonLock after each of the two cycles.
//
// A fresh DebuggingPort is used for the second cycle: nothing in this
// package requires that (the browser process from cycle 1 is fully gone
// before cycle 2 starts, so the same port is free again), but reusing the
// same port would leave a TIME_WAIT-vs-reuse question on the table for no
// reason — freePortT is cheap and this removes the ambiguity outright.
//
// WHAT THIS DOES NOT PROVE (state this plainly, per the packet): this is two
// cycles against a local fixture serving a static page, run back-to-back in
// one test process. It is not the canonical observable named in the
// orchestration ("N ciclos dormir/acordar sem degradação"): it does not run
// N cycles, it does not run against a real WhatsApp account, and it measures
// no degradation signal (memory, timing drift, DOM state decay) between
// cycles — it only checks that the boot/stop machinery itself is reusable.
// A passing result here must not be read as the degradation observable
// being met; it closes a narrower gap ("does the lifecycle cycle at all")
// that is a precondition for ever measuring the real one.
func TestStartSession_CyclesTwice(t *testing.T) {
	binary := findChrome(t)
	profileDir := t.TempDir()
	url := requirePage(t, spa.RequiredAtStartup)

	for cycle := 1; cycle <= 2; cycle++ {
		cfg := StartConfig{
			BinaryPath:    binary,
			ProfileDir:    profileDir,
			DebuggingPort: freePortT(t),
			NavigateURL:   url,
			Runner:        harnessRunner(),
		}
		sess, err := StartSession(context.Background(), cfg)
		if err != nil {
			t.Fatalf("cycle %d: StartSession: %v", cycle, err)
		}
		if sess.Browser().PID() <= 0 {
			t.Errorf("cycle %d: no PID recorded for the started browser", cycle)
		}
		via := sess.Stop(context.Background())
		if !via.Clean() {
			t.Errorf("cycle %d: stopped_via=%s, want a clean stop", cycle, via)
		}
		if n := singletonLockCount(t, profileDir); n != 0 {
			t.Errorf("cycle %d: SingletonLock count = %d/1, want 0/1", cycle, n)
		}
	}
}

// A control on the whole chain: a page that fully satisfies both the probe
// AND the inventory reaches a live, verified Session, and Stop tears it down
// cleanly. Not one of the four adversarial tests, but without this passing
// none of the negative ones would mean anything — a boot path that always
// fails would pass every adversarial test above for the wrong reason.
func TestStartSession_ReachesReadyAndStopsClean(t *testing.T) {
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))
	sess, err := StartSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.Browser().PID() <= 0 {
		t.Error("no PID recorded for the started browser")
	}
	via := sess.Stop(context.Background())
	if !via.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol", via)
	}
	// Stop is idempotent.
	if second := sess.Stop(context.Background()); second != engine.StopViaNoop {
		t.Errorf("second Stop returned %s, want %s", second, engine.StopViaNoop)
	}
	if n := singletonLockCount(t, cfg.ProfileDir); n != 0 {
		t.Errorf("SingletonLock count = %d/1, want 0/1", n)
	}
	// Ownership must be released so the same profile can be started again.
	release, err := acquireOwnership(cfg.ProfileDir)
	if err != nil {
		t.Fatalf("ownership was not released after Stop: %v", err)
	}
	release()
}

// GAP 2 (CAP-05A-T2 packet, H19): every existing core test starts from a
// fresh, never-marked-suspect profile, so the composed read-then-clear
// sequence in StartSession — engine.SessionSuspect on entry,
// engine.ClearSessionSuspect after a verified READY — has no test of its
// own. The fixture below pre-writes the marker with the production
// engine.MarkSessionSuspect, not a hand-rolled file write: the marker's
// format is the production code's business, not the test's.
//
// (b) proves the boot READS it: WasSuspect is surfaced on failure paths, and
// a successful boot's own precondition is that reading it did not error.
// (c) proves a SUCCESSFUL boot CLEARS it.
func TestStartSession_SuspectMarkerComposedPath_ClearedOnSuccess(t *testing.T) {
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))
	if err := engine.MarkSessionSuspect(cfg.ProfileDir, engine.StopViaDirtySignalExitTimeout); err != nil {
		t.Fatalf("priming the fixture with the production marker: %v", err)
	}
	suspectBefore, via, err := engine.SessionSuspect(cfg.ProfileDir)
	if err != nil || !suspectBefore {
		t.Fatalf("fixture setup: SessionSuspect = (%v, %v, %v), want (true, %s, nil)",
			suspectBefore, via, err, engine.StopViaDirtySignalExitTimeout)
	}

	sess, err := StartSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartSession on a suspect-but-otherwise-healthy profile: %v", err)
	}
	via = sess.Stop(context.Background())
	if !via.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol", via)
	}

	suspectAfter, _, err := engine.SessionSuspect(cfg.ProfileDir)
	if err != nil {
		t.Fatalf("SessionSuspect after a verified boot: %v", err)
	}
	if suspectAfter {
		t.Error("marker still present after a boot that reached verified READY; " +
			"ClearSessionSuspect did not run, or did not run on the profile the boot actually used")
	}
}

// GAP 2, item (d), MANDATORY MUTATION: move the clear to run BEFORE the boot
// has actually succeeded — here, immediately after the suspect read, before
// Launch. If VerifyInventory (or Probe) then fails, a policy-honoring boot
// path must NOT have cleared the marker, because nothing was ever verified.
// This test asserts exactly that on the real, unmutated session.go — proving
// the current placement (clear only after VerifyInventory succeeds, per
// session.go's own comment at the READY return) already satisfies the
// invariant. The mutation itself, executed against a temporary copy of the
// clear call moved earlier, is reported below in the prose report with its
// failing output pasted, per the packet's instruction not to leave the
// production file mutated.
func TestStartSession_SuspectMarker_NotClearedOnFailedBoot(t *testing.T) {
	blankPage := `<html><body>nothing here</body></html>`
	cfg := baseConfig(t, pageServer(t, "/blank", blankPage))
	cfg.SettleBudget = negativePathSettleBudget
	if err := engine.MarkSessionSuspect(cfg.ProfileDir, engine.StopViaDirtySignalCloseRefused); err != nil {
		t.Fatalf("priming the fixture with the production marker: %v", err)
	}

	sess, err := StartSession(context.Background(), cfg)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY on a blank page (stopped_via=%s); "+
			"want StageNotReady", via)
	}
	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("error is not *BootFailure: %v", err)
	}
	if boot.Stage != StageNotReady {
		t.Fatalf("failed at stage %q, want %q", boot.Stage, StageNotReady)
	}
	if !boot.WasSuspect {
		t.Error("BootFailure.WasSuspect = false, want true — the marker was primed before this boot")
	}

	suspectAfter, _, err := engine.SessionSuspect(cfg.ProfileDir)
	if err != nil {
		t.Fatalf("SessionSuspect after a failed boot: %v", err)
	}
	if !suspectAfter {
		t.Error("marker cleared after a boot that FAILED to verify; a profile that went down " +
			"dirty and then failed to boot must still be suspect on the next attempt " +
			"(HANDOFF-INICIATIVA.md section 6, invariant 2: verified, not presumed good)")
	}
}

// lateMountDelay is how long the fixture below withholds #pane-side after the
// page is first served. It is NOT a guess: the field failure this test
// reproduces was measured against the real, paired SPA at T+4.708s
// (core: boot failed at not_ready (stopped_via=browser.close): core: page
// classified "OTHER", want "APP_READY" — snapshot dom_nodes=224), which is
// when session.go's single, un-retried spa.Probe fired and gave up. 6s is
// comfortably above that 4.708s firing time, and comfortably below both the
// per-call context budget this test grants (20s) and HANDOFF F2's own
// measured ceiling for a real SPA to finish mounting (recovery p50 10.4s,
// app-ready observed out to 15.8s) — so a boot that actually WAITED for the
// mount would have room to succeed inside the budget, and only a boot that
// classifies once and gives up can fail here.
const lateMountDelay = 6 * time.Second

// lateMountingPage is a fixture that mounts LATE, the way the real SPA does,
// instead of instantly like every other fixture in this file.
//
// ARMADILHA (ARMADILHAS.md, "dublê mais rápido que a produção esconde o
// defeito"): readyPage and requirePage above serve #pane-side already present
// in the initial HTML, so by the time tab.Navigate returns, the DOM already
// satisfies Classify. Against such a fixture, session.go:303's single
// post-navigate spa.Probe always sees APP_READY on the first and only look,
// and a missing settle/retry loop can never be observed — the double was not
// more permissive in ITS RULES, it was FASTER than the real target, and speed
// was the dimension that hid the bug. This fixture serves a shell with NO
// readiness marker at all, then injects #pane-side into a live DOM after
// lateMountDelay via a page-authored script — exactly what session.go must
// tolerate and currently does not.
// lateMountingPageURL serves a page whose window.require answers exactly
// `known` (the same double requirePage above uses, so the settle loop is
// proven against a page that clears VerifyInventory too, not just Classify),
// and that only mounts #pane-side lateMountDelay after it is first served.
//
// This was widened from a bare HTML const during this fix: the original
// fixture had no window.require stub at all, which meant that once the
// settle loop actually started waiting instead of giving up on the first
// probe, StartSession reached APP_READY and then failed one stage later, at
// StageInventory, on a gap in the fixture rather than in production code —
// VerifyInventory correctly refused a page that never answers `require` at
// all. That is not the defect under test, so the fixture now stubs the
// required modules the same way requirePage does.
func lateMountingPageURL(t *testing.T, known []spa.Module) string {
	t.Helper()
	names := make([]string, len(known))
	for i, m := range known {
		names[i] = `"` + string(m) + `"`
	}
	body := `<html><head><title>WhatsApp</title></head><body>
	<div id="app-shell">loading…</div>
	<script>
		const known = new Set([` + strings.Join(names, ",") + `]);
		window.require = function (name) {
			if (!known.has(name)) { throw new Error("Cannot find module '" + name + "'"); }
			return { __module: name };
		};
		setTimeout(function () {
			var pane = document.createElement('div');
			pane.id = 'pane-side';
			document.body.appendChild(pane);
		}, ` + "6000" + `);
	</script>
</body></html>`
	return pageServer(t, "/late", body)
}

// This is the test of the DEFECT, not the fix. It asserts the outcome a
// CORRECT boot path owes a page that is genuinely going to become ready —
// StartSession reaches READY once the fixture's setTimeout mounts #pane-side,
// inside the 20s budget that comfortably contains lateMountDelay.
//
// It MUST fail against the current, unmodified session.go, and it does: the
// boot path (session.go:303) probes exactly once, immediately after Navigate
// returns — far before lateMountDelay elapses — classifies the still-loading
// shell as ClassOther ("OTHER", not "APP_READY"), and gives up with a
// *BootFailure at StageNotReady. There is no settle loop to wait out the
// remaining delay, so this assertion (err == nil, a live *Session) cannot be
// met by the current code, only by a fix that looks more than once.
func TestStartSession_LateMountingSPA_SingleProbeFailsBeforePageFinishesMounting(t *testing.T) {
	profileDir := t.TempDir()
	cfg := baseConfig(t, lateMountingPageURL(t, spa.RequiredAtStartup))
	cfg.ProfileDir = profileDir

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Now()
	sess, err := StartSession(ctx, cfg)
	elapsed := time.Since(start)

	if err != nil {
		var boot *BootFailure
		if errors.As(err, &boot) && boot.Stage == StageNotReady && elapsed < lateMountDelay {
			t.Fatalf("StartSession gave up at StageNotReady after only %s, before "+
				"lateMountDelay=%s had elapsed and #pane-side was ever mounted — this is the "+
				"defect under test: session.go:303 calls spa.Probe exactly once, right after "+
				"Navigate returns, and never looks again. A boot path that instead settled/"+
				"retried until its context budget would have reached APP_READY once the "+
				"fixture's setTimeout fired (cause=%v)", elapsed, lateMountDelay, boot.Cause)
		}
		t.Fatalf("StartSession failed (elapsed=%s): %v", elapsed, err)
	}

	via := sess.Stop(context.Background())
	if !via.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol", via)
	}
	if elapsed < lateMountDelay {
		t.Errorf("StartSession reached READY after only %s, before lateMountDelay=%s — "+
			"the fixture may be mounting too early to exercise the defect", elapsed, lateMountDelay)
	}
}

// Required test, immediate-ready: a page that is already mounted by the time
// Navigate returns must not pay any part of spa.DefaultSettleBudget —
// StartSession must reach READY on the settle loop's first look, the same
// way it always did against readyPage/requirePage before this fix.
func TestStartSession_ImmediateReadyPage_DoesNotPayTheSettleBudget(t *testing.T) {
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))

	start := time.Now()
	sess, err := StartSession(context.Background(), cfg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("StartSession on an already-mounted page: %v", err)
	}
	via := sess.Stop(context.Background())
	if !via.Clean() {
		t.Errorf("stopped_via=%s: a browser this test owns must go down through the protocol", via)
	}
	// Generous relative to spa.DefaultSettleBudget (60s): an already-ready
	// page must settle on essentially the first probe, not after polling.
	const wantUnder = 10 * time.Second
	if elapsed >= wantUnder {
		t.Errorf("elapsed=%s: an already-mounted page took long enough to suggest it "+
			"polled instead of returning on its first look", elapsed)
	}
}

// Required test, terminal class: a page already showing the QR must
// terminate FAST with StageNotReady and the LOGIN_REQUIRED cause preserved —
// not after waiting out any part of the settle budget, and this restoration-
// only path never waits for a human to scan it.
const qrPage = `<html><body>
	<canvas aria-label="Scan me, mate, to log in"></canvas>
</body></html>`

func TestStartSession_QRPage_TerminatesFastWithSpecificCause(t *testing.T) {
	cfg := baseConfig(t, pageServer(t, "/qr", qrPage))

	start := time.Now()
	sess, err := StartSession(context.Background(), cfg)
	elapsed := time.Since(start)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY on a QR page (stopped_via=%s); want StageNotReady", via)
	}
	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("error is not *BootFailure: %v", err)
	}
	if boot.Stage != StageNotReady {
		t.Fatalf("failed at stage %q, want %q", boot.Stage, StageNotReady)
	}
	if !strings.Contains(boot.Cause.Error(), string(spa.ClassLoginRequired)) {
		t.Fatalf("cause = %q, want it to name %q — the specific class must survive, "+
			"not collapse into a generic not-ready", boot.Cause.Error(), spa.ClassLoginRequired)
	}
	// Generous relative to spa.DefaultSettleBudget (60s): a terminal class
	// must be reported fast, not after waiting out any meaningful fraction of
	// the budget.
	const wantUnder = 10 * time.Second
	if elapsed >= wantUnder {
		t.Errorf("elapsed=%s: a terminal LOGIN_REQUIRED page took long enough to "+
			"suggest StartSession waited it out instead of failing fast", elapsed)
	}
}

// Required test, final snapshot preserved: a boot that fails at StageNotReady
// must still carry the LAST structural snapshot actually observed — the
// BootFailure.Cause message that already names url and dom_nodes (see the
// StageNotReady branch in session.go) must not go missing just because the
// failure now comes from a settle loop instead of a single probe.
func TestStartSession_NotReadyFailure_PreservesFinalSnapshot(t *testing.T) {
	blankPage := `<html><body>nothing here, ever</body></html>`
	cfg := baseConfig(t, pageServer(t, "/blank-snapshot", blankPage))

	// THE CALLER'S CONTEXT STAYS SOVEREIGN OVER THE SETTLE BUDGET — that is the
	// property. How it is expressed had to change.
	//
	// It used to be three seconds for the WHOLE boot, on the reasoning that a
	// short ctx keeps the test fast. That silently assumed a machine on which
	// launching Chrome and opening a tab fit inside three seconds. Under -race,
	// alongside every other package, it does not: this failed at stage launch,
	// open_tab and navigate on three consecutive runs, never reaching the settle
	// loop it exists to test. Same family as F100 — a fixed short deadline that
	// encodes a machine's speed.
	//
	// The property is preserved by making the two budgets far apart instead of
	// making the ctx small: the settle budget is minutes, the ctx is a fraction
	// of it, so a failure at StageNotReady can only mean the CONTEXT cut the
	// settle short. The generous half of the pair is the one that is safe to
	// grow.
	cfg.SettleBudget = 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	sess, err := StartSession(ctx, cfg)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY on a page that never mounts (stopped_via=%s); "+
			"want StageNotReady", via)
	}
	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("error is not *BootFailure: %v", err)
	}
	if boot.Stage != StageNotReady {
		t.Fatalf("failed at stage %q, want %q", boot.Stage, StageNotReady)
	}
	cause := boot.Cause.Error()
	if !strings.Contains(cause, "dom_nodes=") {
		t.Errorf("cause = %q, lost the final snapshot's dom_nodes", cause)
	}
	if !strings.Contains(cause, "url=") {
		t.Errorf("cause = %q, lost the final snapshot's url", cause)
	}
}

// TestStartSession_SessionOutlivesItsBootContext is the BEFORE_FIX evidence for
// the defect measured against the real paired profile on 2026-08-18.
//
// The measurement: the N-cycle test bounded StartSession with a 60s boot
// deadline, called cancel() as any correct Go caller does, and then every
// identity probe on the returned *Session came back "context canceled" for a
// full 76s window. The session was dead the instant its BOOT deadline was
// released.
//
// The cause, in engine/tab.go:35: OpenTab builds the chromedp allocator from
// the PARENT context, and core.StartSession passes the caller's boot context as
// that parent. So the session's lifetime IS the boot deadline's lifetime. A
// caller that gives StartSession a 60s budget has not asked for a 60s session —
// it has asked for a 60s BOOT — but that is what it gets.
//
// Why the whole suite was blind to it: every other test in this file calls
// StartSession with context.Background(), which is never cancelled and never
// expires, so the coupling can never bite. This is the same armadilha as the
// late-mount one, in a new dimension — the double did not diverge from
// production in its RULES, it diverged in its CONTEXT LIFETIME. See
// ARMADILHAS.md.
func TestStartSession_SessionOutlivesItsBootContext(t *testing.T) {
	runner := engine.NewRunner()
	cfg := baseConfig(t, pageServer(t, "/ready", readyPage))
	cfg.Runner = runner
	cfg.RequiredModules = []spa.Module{}

	// A BOOT budget, released as soon as the boot returns. This is what a
	// production caller does; it is not an unusual or hostile pattern.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 30*time.Second)
	sess, err := StartSession(bootCtx, cfg)
	cancelBoot()
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer sess.Stop(context.Background())

	// The session must still be usable. Anything else means StartSession
	// returned a handle that was already dead.
	var got string
	evalErr := runner.Do(context.Background(), engine.OpStateProbe, "post-boot/probe",
		func(ctx context.Context) error { return sess.Tab().Evaluate(ctx, "String(1+1)", &got) })
	if evalErr != nil {
		t.Fatalf("the session did not outlive its boot context: probing the returned "+
			"*Session after cancelling the BOOT context failed with %v. StartSession "+
			"handed back a session whose lifetime is its boot deadline — this is the "+
			"defect under test (engine/tab.go:35 derives the allocator from the parent "+
			"context, and core/session.go passes the caller's boot ctx as that parent)",
			evalErr)
	}
	if got != "2" {
		t.Fatalf("probe returned %q, want \"2\"", got)
	}
}

// slowPageServer serves body only after delay, so the boot spends that whole
// delay inside tab.Navigate. That is the point: Navigate runs under the TAB's
// context, never under ctx, so it is the ONLY window in which the watcher
// goroutine is what carries the caller's cancellation. A fast fixture makes
// this test pass with the watcher deleted — measured, see ARMADILHAS.md — and
// a test that passes with the mechanism removed is not testing the mechanism.
func slowPageServer(t *testing.T, delay time.Duration, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/slow"
}

// TestStartSession_CancelledBootStillAborts is the other half of the lifetime
// fix, and the reason it is not simply "pass context.Background() to OpenTab".
//
// Decoupling the session from the boot context must not cost the caller its
// ability to abort the boot. tab.Navigate runs under the TAB's context, not
// under ctx, so once the tab stops being a child of ctx there is nothing left
// tying navigation to the caller's cancellation except the watcher goroutine
// StartSession installs. This test cancels WHILE Navigate is still waiting on
// a deliberately slow server, and requires the attempt to end with a failure
// and a clean teardown — never a live session, never a hang.
func TestStartSession_CancelledBootStillAborts(t *testing.T) {
	const serverDelay = 30 * time.Second

	profile := t.TempDir()
	cfg := baseConfig(t, slowPageServer(t, serverDelay, readyPage))
	cfg.ProfileDir = profile

	bootCtx, cancelBoot := context.WithCancel(context.Background())
	defer cancelBoot()

	done := make(chan struct{})
	var sess *Session
	var err error
	start := time.Now()
	go func() {
		sess, err = StartSession(bootCtx, cfg)
		close(done)
	}()

	// Cancel once the boot is certainly inside Navigate, waiting on the server.
	time.Sleep(3 * time.Second)
	cancelBoot()

	select {
	case <-done:
	case <-time.After(serverDelay):
		t.Fatal("StartSession did not return within the server delay after its boot " +
			"context was cancelled — the caller lost the ability to abort a boot, " +
			"which is the regression the session-lifetime fix must not introduce")
	}
	elapsed := time.Since(start)

	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession returned a live Session (stopped_via=%s) despite its boot "+
			"context being cancelled mid-navigate", via)
	}
	if sess != nil {
		t.Fatalf("StartSession returned both an error (%v) and a non-nil Session", err)
	}
	// The abort must come from the cancellation, not from the server finally
	// answering. Without this bound the test would pass on a boot that simply
	// waited out the slow server and failed later for an unrelated reason.
	if elapsed >= serverDelay {
		t.Fatalf("StartSession took %s, at or beyond the %s server delay: it waited the "+
			"navigation out instead of aborting on cancellation", elapsed, serverDelay)
	}
	if n := singletonLockCount(t, profile); n != 0 {
		t.Fatalf("SingletonLock survived an aborted boot (count=%d); the teardown is not clean", n)
	}
	t.Logf("aborted %s into a %s navigation (err=%v)", elapsed, serverDelay, err)
}

// TestStartSession_SessionSurvivesBootDeadlineExpiry is the second shape of the
// same invariant, and it is not the same test as
// TestStartSession_SessionOutlivesItsBootContext.
//
// That one cancels EXPLICITLY, right after the boot returns. This one lets a
// short boot DEADLINE expire on its own, later, while the session is being
// used. The distinction matters because the two arrive by different routes: a
// caller writes `defer cancel()` on purpose, but a deadline expiring under a
// long-lived session is something a caller creates by accident — it is the
// shape the real holder of these sessions will produce, and the module has no
// such holder yet (runtime/ is doc-only, and nothing outside this module
// imports it), so nothing else in the repository exercises it.
//
// The invariant, stated once so the next resilience layer is audited against a
// rule instead of rediscovering it:
//
//	A session's lifetime is ended by Stop, and by nothing else.
//	No boot deadline, no caller cancellation after the boot has returned,
//	and no context the caller happened to pass in may end it.
func TestStartSession_SessionSurvivesBootDeadlineExpiry(t *testing.T) {
	const bootBudget = 25 * time.Second

	runner := engine.NewRunner()
	cfg := baseConfig(t, pageServer(t, "/ready", readyPage))
	cfg.Runner = runner
	cfg.RequiredModules = []spa.Module{}

	bootCtx, cancelBoot := context.WithTimeout(context.Background(), bootBudget)
	defer cancelBoot()

	start := time.Now()
	sess, err := StartSession(bootCtx, cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer sess.Stop(context.Background())
	bootTook := time.Since(start)

	// Hold the session past the boot deadline, the way a caller that keeps a
	// session across commands does. Nothing here re-derives from bootCtx.
	<-bootCtx.Done()
	if bootCtx.Err() == nil {
		t.Fatal("the boot context did not expire; this test proves nothing")
	}

	var got string
	evalErr := runner.Do(context.Background(), engine.OpStateProbe, "post-deadline/probe",
		func(ctx context.Context) error { return sess.Tab().Evaluate(ctx, "String(6*7)", &got) })
	if evalErr != nil {
		t.Fatalf("the session died when its BOOT deadline expired (%v), %s after the boot "+
			"itself had already returned successfully in %s. A caller granting a boot "+
			"budget is not granting a session lifetime; only Stop may end a session",
			evalErr, bootBudget-bootTook, bootTook)
	}
	if got != "42" {
		t.Fatalf("probe returned %q, want \"42\"", got)
	}
	t.Logf("boot returned in %s; session still answering %s after its boot deadline expired",
		bootTook, bootBudget-bootTook)
}

// TestStartSession_SettleLoopRespectsTheBudgetItWasGiven is the property H20
// argued was better than the one the slow tests were accidentally proving.
//
// "The boot waits 60s before giving up" is a fact about a default. "The boot
// gives up at the budget it was handed" is a fact about the MECHANISM, and it
// is the one that keeps meaning something when the default changes. Without
// it, making SettleBudget configurable would be a pure speed-up with nothing
// asserting the knob is connected to anything.
func TestStartSession_SettleLoopRespectsTheBudgetItWasGiven(t *testing.T) {
	const budget = 4 * time.Second

	blankPage := `<html><body>nothing here</body></html>`
	cfg := baseConfig(t, pageServer(t, "/blank", blankPage))
	cfg.SettleBudget = budget

	start := time.Now()
	sess, err := StartSession(context.Background(), cfg)
	elapsed := time.Since(start)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY on a blank page (stopped_via=%s)", via)
	}

	var boot *BootFailure
	if !errors.As(err, &boot) || boot.Stage != StageNotReady {
		t.Fatalf("want *BootFailure at StageNotReady, got %v", err)
	}
	// Upper bound: the loop must not fall back to the 60s default. The margin
	// covers launch and navigate, which happen before the settle loop starts.
	if elapsed >= spa.DefaultSettleBudget {
		t.Fatalf("boot took %s, at or beyond the %s DEFAULT budget despite being given "+
			"%s — SettleBudget is not reaching the settle loop",
			elapsed, spa.DefaultSettleBudget, budget)
	}
	// Lower bound: the loop must actually WAIT the budget, not give up early.
	// This is what stops a single-shot regression from passing the test above
	// — a boot that classifies once and quits would also finish well under the
	// default, and would be indistinguishable without this assertion.
	if elapsed < budget {
		t.Fatalf("boot gave up after %s, sooner than the %s budget it was given — the "+
			"settle loop is not waiting out its budget", elapsed, budget)
	}
	t.Logf("gave up after %s on a %s budget (default is %s)", elapsed, budget, spa.DefaultSettleBudget)
}

// TestStartSession_OwnerIdentityModuleIsEnforced proves the boot demands the
// module the OWNER IDENTITY lives in, specifically.
//
// TestStartSession_MissingModuleClassifiesAsErrModulesMissing already proves
// that *a* missing module fails the boot — it drops whichever module happens to
// be first in the list. That is not the same claim: it would keep passing if
// spa.ModuleUserPrefsMeUser were quietly dropped from RequiredAtStartup, which
// is exactly the regression this test exists to catch, because that module was
// ADDED to the inventory on 2026-08-19 and adding it is the whole reason
// capabilities/owner can rely on it being there.
//
// The distinction matters beyond bookkeeping: WAWebUserPrefsMeUser and
// WAWebUserPrefsInfoStore differ by one word, and the second was already in the
// list. A rename or a careless edit that collapsed them would leave the boot
// verifying a module the owner identity does not live in.
func TestStartSession_OwnerIdentityModuleIsEnforced(t *testing.T) {
	var withoutIdentity []spa.Module
	for _, m := range spa.RequiredAtStartup {
		if m == spa.ModuleUserPrefsMeUser {
			continue
		}
		withoutIdentity = append(withoutIdentity, m)
	}
	if len(withoutIdentity) == len(spa.RequiredAtStartup) {
		t.Fatalf("spa.ModuleUserPrefsMeUser (%q) is not in RequiredAtStartup at all; "+
			"capabilities/owner reads that module and the boot no longer guarantees it "+
			"is there", spa.ModuleUserPrefsMeUser)
	}

	// A page that exposes every required module EXCEPT the identity one.
	cfg := baseConfig(t, requirePage(t, withoutIdentity))
	cfg.SettleBudget = negativePathSettleBudget

	sess, err := StartSession(context.Background(), cfg)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("StartSession reached READY (stopped_via=%s) on a page missing %q; "+
			"the boot must fail high with a named cause instead of letting a capability "+
			"discover it later, mid-operation", via, spa.ModuleUserPrefsMeUser)
	}
	var modErr *spa.ErrModulesMissing
	if !errors.As(err, &modErr) {
		t.Fatalf("err = %v, want a *spa.ErrModulesMissing", err)
	}
	// The error must name THIS module, not merely report a count. CAP-06's
	// observable is that the message names the cause.
	var named bool
	for _, m := range modErr.Missing {
		if m == spa.ModuleUserPrefsMeUser {
			named = true
		}
	}
	if !named {
		t.Fatalf("the failure lists %v, which does not include %q: a boot that fails "+
			"without naming the module nobody can act on", modErr.Missing, spa.ModuleUserPrefsMeUser)
	}
}

// TestBootFailurePIDIsRenderedForCorrelation covers H18 WITHOUT launching a
// browser: BootFailure is a value, and what it renders is a property of the
// value, not of the boot that produced it.
//
// Doing it this way is not only cheaper. A test that failed a real boot would
// depend on which stage happened to fail and on the machine having a spare
// Chrome, and it would exercise the FORMATTING only incidentally — the thing a
// future reader actually consumes.
func TestBootFailurePIDIsRenderedForCorrelation(t *testing.T) {
	cases := []struct {
		name       string
		fail       BootFailure
		wantPID    string
		wantNoText string
	}{
		{
			name: "a launched browser is named, so the failure can be matched " +
				"against the operating system's own record of that process",
			fail:    BootFailure{Stage: StageNotReady, StoppedVia: engine.StopViaBrowserClose, PID: 4242, Cause: errors.New("boom")},
			wantPID: "pid=4242",
		},
		{
			name: "a failure BEFORE launch has no process, and must not print pid=0 — " +
				"a zero reads like a real process id to whoever greps for it",
			fail:       BootFailure{Stage: StageOwnership, PID: 0, Cause: errors.New("boom")},
			wantNoText: "pid=",
		},
		{
			name:    "the pid survives even when there is no StoppedVia to hang it on",
			fail:    BootFailure{Stage: StageLaunch, PID: 77, Cause: errors.New("boom")},
			wantPID: "pid=77",
		},
	}

	for _, c := range cases {
		got := c.fail.Error()
		if c.wantPID != "" && !strings.Contains(got, c.wantPID) {
			t.Errorf("%s:\n got  %q\n want it to contain %q", c.name, got, c.wantPID)
		}
		if c.wantNoText != "" && strings.Contains(got, c.wantNoText) {
			t.Errorf("%s:\n got  %q\n want it NOT to contain %q", c.name, got, c.wantNoText)
		}
		// The pid must never cost the diagnosis that was already there.
		if !strings.Contains(got, string(c.fail.Stage)) {
			t.Errorf("%s: the stage disappeared from %q", c.name, got)
		}
		if !strings.Contains(got, "boom") {
			t.Errorf("%s: the cause disappeared from %q", c.name, got)
		}
	}
}

// NOTE: a TestBootFailurePIDIsNotAnInvitationToSignal was written here on
// 2026-08-20 and REMOVED the same day, because its negative control did not
// bite.
//
// It scanned session.go for "ProcessAlive(pid)", "syscall.Kill" and "Signal(",
// meaning to enforce that nothing acts on BootFailure.PID — the boundary that
// keeps this field from contradicting H23. A mutation that added
// engine.ProcessAlive(e.PID) passed straight through: the spelling differed.
//
// Widening it is not available either. session.go LEGITIMATELY calls
// engine.ProcessAlive for Session.ProcessAlive (H21), so the identifier cannot
// be forbidden, and a text scan cannot tell "the live session's pid" from "the
// BootFailure's stale one". That is the H32 conclusion again: text does not
// read code structure. An AST guard could, and costs more than one field
// justifies.
//
// So the boundary is held by the doc comment on BootFailure.PID and by review,
// NOT by a test — stated here because a test that does not bite is worse than
// no test: it is false assurance, and finding this one silent was luck.

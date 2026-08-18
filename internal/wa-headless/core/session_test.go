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

func baseConfig(t *testing.T, navigateURL string) StartConfig {
	t.Helper()
	return StartConfig{
		BinaryPath:    findChrome(t),
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePortT(t),
		NavigateURL:   navigateURL,
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
	if len(successes) != 1 {
		for _, f := range failures {
			t.Logf("failure: %v", f)
		}
		t.Fatalf("successes=%d, want exactly 1 — a second browser must not be born "+
			"silently for the same profile", len(successes))
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

	// A short external ctx keeps this test fast: it stays sovereign over
	// spa.DefaultSettleBudget (60s), so the boot fails once THIS deadline
	// runs out rather than the full internal budget.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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

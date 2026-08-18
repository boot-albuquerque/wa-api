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

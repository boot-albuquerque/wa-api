package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/liveness"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/usr/bin/google-chrome",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
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
	t.Skip("no Chrome/Chromium found; set WA_HEADLESS_CHROME to run the holder chain")
	return ""
}

func freePort(t *testing.T) int {
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

// readyPage carries #pane-side, the marker spa.Classify keys on for
// ClassAppReady. Copied rather than imported because core's fixtures are
// unexported test helpers; the selector is the production one either way.
const readyPage = `<html><body><div id="pane-side"><div>a chat</div></div></body></html>`

func pageServer(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/ready"
}

// holderConfig builds a StartConfig against a local fixture. RequiredModules is
// empty because the module inventory is core's contract (CAP-06) and is proven
// there; repeating it here would test core through a second door instead of
// testing the Holder.
func holderConfig(t *testing.T, profileDir string) core.StartConfig {
	t.Helper()
	r := engine.NewRunner()
	// THE HARNESS BOOT BUDGET, not the product one. engine.DefaultDeadlines.Boot
	// stays at 30s because that is a product decision backed by a measurement;
	// these tests boot dozens of browsers under -race while the rest of the gate
	// runs, and F100 recorded seven failures that were the machine being busy.
	// core/harnessbudget_test.go carries the reasoning and the control that
	// keeps a raised ceiling from becoming no ceiling.
	// 150s, matching core/harnessbudget_test.go. It is a duplicated literal and
	// that is deliberate: the two packages cannot share an unexported test
	// constant, and exporting one would put a harness concern into production
	// code. What keeps them from drifting is that they fail the same way, in the
	// same run, on the same contention.
	r.Policy.Boot = 150 * time.Second
	return core.StartConfig{
		BinaryPath:      findChrome(t),
		ProfileDir:      profileDir,
		DebuggingPort:   freePort(t),
		NavigateURL:     pageServer(t, readyPage),
		RequiredModules: []spa.Module{},
		Runner:          r,
	}
}

// TestHolder_BootsOnceAndIsReusedAcrossCommands is the Holder's reason to
// exist: a session that survives the command that created it.
func TestHolder_BootsOnceAndIsReusedAcrossCommands(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	defer h.Stop(context.Background())

	first, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("first Session: %v", err)
	}
	firstPID := first.Browser().PID()

	second, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("second Session: %v", err)
	}

	if first != second {
		t.Fatalf("the Holder booted a SECOND session for the same profile: %p then %p. "+
			"A holder that re-boots per call is not holding anything, and it breaks "+
			"invariant 13 by owning the same profile twice", first, second)
	}
	if pid := second.Browser().PID(); pid != firstPID {
		t.Fatalf("second call answered with browser pid %d, first was %d — a new browser "+
			"was launched", pid, firstPID)
	}
}

// TestHolder_SessionSurvivesTheBootContextOfTheCallThatCreatedIt is invariant 15
// (HANDOFF §6) exercised by a REAL holder for the first time.
//
// core's own tests prove the invariant against a caller they construct inside
// the test. This proves it for the shape production actually has: a command
// arrives with its own deadline, boots the session, returns, and its context
// dies — and a LATER command must still find the session alive. That gap is
// what hid the defect measured on 2026-08-18 (ARMADILHAS.md).
func TestHolder_SessionSurvivesTheBootContextOfTheCallThatCreatedIt(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	defer h.Stop(context.Background())

	// Command 1: boots under its own budget, then that budget is released.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 30*time.Second)
	sess, err := h.Session(bootCtx)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	cancelBoot()

	// Command 2, arriving later with a context of its own.
	laterCtx, cancelLater := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelLater()
	again, err := h.Session(laterCtx)
	if err != nil {
		t.Fatalf("second command could not get the held session: %v", err)
	}

	var got string
	runner := engine.NewRunner()
	evalErr := runner.Do(laterCtx, engine.OpStateProbe, "holder/later-command",
		func(ctx context.Context) error { return again.Tab().Evaluate(ctx, "String(6*7)", &got) })
	if evalErr != nil {
		t.Fatalf("the held session was dead when a later command used it (%v). The first "+
			"command's boot deadline ended the session — invariant 15 says only Stop may "+
			"do that, and a Holder is exactly the caller that makes this visible", evalErr)
	}
	if got != "42" {
		t.Fatalf("probe returned %q, want \"42\"", got)
	}
	_ = sess
}

// TestHolder_ConcurrentFirstUseBootsExactlyOneSession is invariant 13 under the
// race that actually threatens it: several commands arriving at once, before
// any session exists.
func TestHolder_ConcurrentFirstUseBootsExactlyOneSession(t *testing.T) {
	const callers = 4

	h := NewHolder(holderConfig(t, t.TempDir()))
	defer h.Stop(context.Background())

	var wg sync.WaitGroup
	sessions := make([]*core.Session, callers)
	errs := make([]error, callers)
	start := make(chan struct{})

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			sessions[i], errs[i] = h.Session(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()

	pids := map[int]int{}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d failed: %v — concurrent callers asking for THE session "+
				"must not see a spurious ownership error; that is one logical request "+
				"from several goroutines", i, errs[i])
		}
		if sessions[i] == nil {
			t.Fatalf("caller %d got a nil session and a nil error", i)
		}
		pids[sessions[i].Browser().PID()]++
	}
	if len(pids) != 1 {
		t.Fatalf("%d distinct browser pids across %d concurrent callers (%v); exactly one "+
			"session must exist for one profile (invariant 13)", len(pids), callers, pids)
	}
	for i := 1; i < callers; i++ {
		if sessions[i] != sessions[0] {
			t.Fatalf("caller %d got a different *Session than caller 0", i)
		}
	}
}

// TestHolder_SecondHolderOnTheSameProfileIsRefused is invariant 13 across
// holders. The Holder does not implement this itself — core.StartSession's
// ownership check does — and the test exists to prove the Holder does not
// somehow bypass it.
func TestHolder_SecondHolderOnTheSameProfileIsRefused(t *testing.T) {
	profile := t.TempDir()

	first := NewHolder(holderConfig(t, profile))
	defer first.Stop(context.Background())
	if _, err := first.Session(context.Background()); err != nil {
		t.Fatalf("first holder: %v", err)
	}

	second := NewHolder(holderConfig(t, profile))
	sess, err := second.Session(context.Background())
	if err == nil {
		via := second.Stop(context.Background())
		t.Fatalf("a SECOND holder booted the same profile (stopped_via=%s); one profile "+
			"must have one active session owner (invariant 13)", via)
	}
	if sess != nil {
		t.Fatalf("second holder returned an error (%v) and a non-nil session", err)
	}
	if !errors.Is(err, core.ErrProfileAlreadyOwned) {
		t.Fatalf("second holder failed with %v; want core.ErrProfileAlreadyOwned. The "+
			"refusal must name the cause — a generic boot failure here would be "+
			"indistinguishable from Chrome simply failing to launch", err)
	}
}

// TestHolder_StopReleasesTheProfileForANewHolder proves Stop actually gives the
// profile back. Without it, a Holder that stopped cleanly would still poison
// the profile for the rest of the process, and the failure would look like
// invariant 13 working correctly.
func TestHolder_StopReleasesTheProfileForANewHolder(t *testing.T) {
	profile := t.TempDir()

	first := NewHolder(holderConfig(t, profile))
	if _, err := first.Session(context.Background()); err != nil {
		t.Fatalf("first holder: %v", err)
	}
	if via := first.Stop(context.Background()); !via.Clean() {
		t.Fatalf("first holder stopped via %s, want a clean stop", via)
	}

	second := NewHolder(holderConfig(t, profile))
	defer second.Stop(context.Background())
	if _, err := second.Session(context.Background()); err != nil {
		t.Fatalf("a new holder could not take the profile after the first stopped "+
			"cleanly: %v. Stop must release ownership, not just tear the browser down", err)
	}
}

// TestHolder_StoppedHolderRefusesToBootAgain locks the decision documented on
// ErrHolderStopped: a stopped Holder is finished, not idle.
func TestHolder_StoppedHolderRefusesToBootAgain(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	if _, err := h.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}
	h.Stop(context.Background())

	sess, err := h.Session(context.Background())
	if !errors.Is(err, ErrHolderStopped) {
		if sess != nil {
			h.Stop(context.Background())
		}
		t.Fatalf("a stopped Holder booted again (err=%v); it must refuse with "+
			"ErrHolderStopped so ownership of the profile is a decision, not a race", err)
	}
}

// TestHolder_RefusesAHeldSessionWhoseProcessDied is H21's reason to exist, and
// it is built so the detector can actually FAIL (briefing item 15).
//
// The browser is killed from OUTSIDE, the way a crash or an OOM kill arrives:
// nothing in this module calls Stop, so every piece of in-process bookkeeping
// still says the session is fine. A Holder that trusts its own bookkeeping
// hands out a dead handle and the caller finds out as a CDP error in the middle
// of a business operation. A detector that always answered "alive" would pass
// every other test in this file and fail only this one.
func TestHolder_RefusesAHeldSessionWhoseProcessDied(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	defer h.Stop(context.Background())

	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	pid := sess.Browser().PID()
	if !sess.ProcessAlive() {
		t.Fatal("the session reports its process dead immediately after a successful " +
			"boot; the detector is broken in the direction that makes this test vacuous")
	}

	// Kill from outside, then wait for the OS to actually reap it. Asserting
	// straight after the signal would race the kernel and make the test flaky
	// in the direction that hides a real defect.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("killing the browser from outside: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for engine.ProcessAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if engine.ProcessAlive(pid) {
		t.Fatalf("pid %d still alive 10s after SIGKILL; the fixture, not the code, "+
			"is what failed here", pid)
	}

	got, err := h.Session(context.Background())
	if !errors.Is(err, ErrSessionDied) {
		if got != nil {
			t.Fatalf("the Holder handed out a session whose process (pid %d) is gone, "+
				"with err=%v; want ErrSessionDied. The caller would meet this as a CDP "+
				"failure inside a business operation instead of an invalid session",
				pid, err)
		}
		t.Fatalf("want ErrSessionDied, got %v", err)
	}
	if got != nil {
		t.Fatalf("the Holder returned ErrSessionDied AND a non-nil session")
	}
}

// TestHolder_ProcessAliveIsFalseAfterStop locks the other end of the same
// signal. Without it, ProcessAlive could be a constant true for every state the
// tests above reach, since they only ever ask about a running browser.
func TestHolder_ProcessAliveIsFalseAfterStop(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if via := h.Stop(context.Background()); !via.Clean() {
		t.Fatalf("stopped via %s, want clean", via)
	}
	if sess.ProcessAlive() {
		t.Fatal("a stopped session still reports its process alive; a stopped session " +
			"has no process by definition, and a caller holding an old handle would " +
			"read this as permission to use it")
	}
}

// TestBrowserPIDIsRefusedOnceItIsMeaningless is the capability, not a
// formality: getBrowserPid exists so a supervisor can watch a process, and a
// pid handed out after the process died is a number the OS may have given to
// something else.
//
// It asserts the three refusals separately, because a caller told only "no pid"
// cannot tell "nothing started" from "what started is gone" — and those call
// for different reactions.
func TestBrowserPIDIsRefusedOnceItIsMeaningless(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))

	// 1. Never started.
	if _, err := h.BrowserPID(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("before boot: err = %v, want ErrNoSession", err)
	}

	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	pid, err := h.BrowserPID()
	if err != nil {
		t.Fatalf("with a live session: %v", err)
	}
	if pid <= 0 || pid != sess.Browser().PID() {
		t.Fatalf("BrowserPID=%d, browser pid=%d", pid, sess.Browser().PID())
	}
	if !engine.ProcessAlive(pid) {
		t.Fatalf("pid %d is not alive on a freshly booted session", pid)
	}

	// 2. Process killed from outside — the supervisor's own failure case.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("killing the browser: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for engine.ProcessAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	got, err := h.BrowserPID()
	if !errors.Is(err, ErrSessionDied) {
		t.Fatalf("after the process died: pid=%d err=%v, want ErrSessionDied. "+
			"engine.Browser.PID() still returns %d, and a supervisor storing that "+
			"number would later signal whatever inherited it", got, err, pid)
	}
	if got != 0 {
		t.Fatalf("a stale pid (%d) was returned alongside the error; a caller logging "+
			"\"pid=%%d err=%%v\" would put a reusable number into the record", got)
	}

	// 3. Stopped.
	h.Stop(context.Background())
	if _, err := h.BrowserPID(); !errors.Is(err, ErrHolderStopped) {
		t.Fatalf("after Stop: err = %v, want ErrHolderStopped", err)
	}
}

// TestBrowserPIDAfterCleanStopIsRefusedToo covers the ordinary path, which the
// kill test above does not: a session stopped normally leaves engine.Browser
// holding the same pid it always had. Measured — that is what made this
// capability more than a getter.
func TestBrowserPIDAfterCleanStopIsRefusedToo(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	pid := sess.Browser().PID()
	if via := h.Stop(context.Background()); !via.Clean() {
		t.Fatalf("stopped via %s, want clean", via)
	}

	if sess.Browser().PID() != pid {
		t.Fatal("engine.Browser.PID() changed after Stop; this test's premise no longer holds")
	}
	if _, err := h.BrowserPID(); err == nil {
		t.Fatalf("BrowserPID answered after a clean stop; engine still holds pid %d and "+
			"the OS may have reassigned it", pid)
	}
}

// TestConcurrentCapabilityCallsOnOneSession exercises the claim spa.Monitor
// makes in its own doc comment — "safe for concurrent use: a probe timer and a
// command path can both ask" — against a real browser instead of leaving it as
// prose.
//
// The product's shape is exactly this: a liveness timer ticking while a command
// handler drives the same session. Nothing in this module had ever run two
// evaluations against one tab at the same time, so the claim was untested where
// it matters — chromedp serialises on the tab context, and whether that
// serialisation holds under a dozen callers is a fact about the driver, not
// about our types.
//
// IT MEASURES ITS OWN OVERLAP. A concurrency test whose callers never coincide
// proves serialisation, not safety, and -race has nothing to detect. At a 2ms
// round trip that is a real possibility, so the peak number of in-flight
// evaluations is counted and asserted rather than hoped for.
func TestConcurrentCapabilityCallsOnOneSession(t *testing.T) {
	const (
		callers    = 8
		iterations = 6
	)

	runner := engine.NewRunner()
	cfg := holderConfig(t, t.TempDir())
	cfg.Runner = runner
	h := NewHolder(cfg)
	defer h.Stop(context.Background())

	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	var inFlight, maxInFlight int64
	countedEval := func(ctx context.Context, expr string, out *string) error {
		n := atomic.AddInt64(&inFlight, 1)
		for {
			m := atomic.LoadInt64(&maxInFlight)
			if n <= m || atomic.CompareAndSwapInt64(&maxInFlight, m, n) {
				break
			}
		}
		defer atomic.AddInt64(&inFlight, -1)
		return sess.Tab().Evaluate(ctx, expr, out)
	}
	checker := liveness.New(sess.ProcessAlive, runner, countedEval)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []string
		alive int
		start = make(chan struct{})
		worst time.Duration
	)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				// Two different callers of the same session: a liveness probe
				// and a direct evaluation, which is what a capability does.
				got := checker.Check(context.Background(), fmt.Sprintf("conc/live%d-%d", id, j))
				var out string
				evalErr := runner.Do(context.Background(), engine.OpStateProbe,
					fmt.Sprintf("conc/eval%d-%d", id, j),
					func(ctx context.Context) error {
						return countedEval(ctx, "String(2+2)", &out)
					})

				mu.Lock()
				if got.Alive {
					alive++
				} else {
					errs = append(errs, fmt.Sprintf("liveness %d-%d: signal=%s err=%v",
						id, j, got.Signal, got.Err))
				}
				if evalErr != nil {
					errs = append(errs, fmt.Sprintf("eval %d-%d: %v", id, j, evalErr))
				} else if out != "4" {
					// An answer meant for another caller would look exactly
					// like this, which is the failure mode a shared tab has.
					errs = append(errs, fmt.Sprintf("eval %d-%d returned %q, want 4", id, j, out))
				}
				if got.Latency > worst {
					worst = got.Latency
				}
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		shown := errs
		if len(shown) > 8 {
			shown = shown[:8]
		}
		t.Fatalf("%d failure(s) across %d concurrent callers:\n  %s",
			len(errs), callers, strings.Join(shown, "\n  "))
	}
	if want := callers * iterations; alive != want {
		t.Fatalf("alive=%d, want %d", alive, want)
	}

	probes, failures, _, consecutive := checker.Stats()
	if failures != 0 || consecutive != 0 {
		t.Errorf("the monitor recorded failures=%d streak=%d under concurrency alone",
			failures, consecutive)
	}
	if probes != callers*iterations {
		t.Errorf("the monitor counted %d probes, want %d: a lost increment is the counter "+
			"racing", probes, callers*iterations)
	}

	peak := atomic.LoadInt64(&maxInFlight)
	t.Logf("%d callers × %d iterations: all alive, worst latency %s, PEAK OVERLAP %d",
		callers, iterations, worst.Round(time.Millisecond), peak)
	if peak < 2 {
		t.Fatalf("peak overlap was %d: the callers never coincided, so this measured "+
			"SERIALISATION and not concurrency. Nothing here says the shared state is "+
			"safe", peak)
	}
}

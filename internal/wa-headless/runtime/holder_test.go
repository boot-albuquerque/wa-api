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
	"sync"
	"testing"
	"time"

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
	return core.StartConfig{
		BinaryPath:      findChrome(t),
		ProfileDir:      profileDir,
		DebuggingPort:   freePort(t),
		NavigateURL:     pageServer(t, readyPage),
		RequiredModules: []spa.Module{},
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

// TestHolder_SessionSurvivesTheBootContextOfTheCallThatCreatedIt is invariant 7
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
			"command's boot deadline ended the session — invariant 7 says only Stop may "+
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

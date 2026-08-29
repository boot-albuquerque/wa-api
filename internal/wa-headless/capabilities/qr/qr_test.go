package qr

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates the page-global protocol kickScript/parked rely on —
// NOT a JS interpreter. It answers by inspecting the script's SHAPE (which
// of kick/poll/release it is, and whether the kick asked for a refresh),
// the same three operations every capabilities/*.parked helper performs,
// and lets a test script a sequence of "how many kicks until ref shows up".
//
// It never simulates the real JS chain (getRegistrationInfo etc.) — that is
// exactly what TestProbeQRReader_ProductionPackage (probe_qr_test.go, real
// Chrome) exists to measure instead. This double exists to test the GO-SIDE
// retry loop deterministically, in milliseconds, without a browser.
type pageDouble struct {
	mu sync.Mutex

	// refAfterKicks: the kick call whose 1-based count reaches this number
	// is the first one that finds a populated ref. 1 means "ref present
	// from the very first kick". 0 means "never populates".
	refAfterKicks int
	// expiredUntilKick: kicks numbered 1..expiredUntilKick report the
	// "code expired" overlay (why="expired", ref present but stale — see
	// this package's own doc comment on the measured 6-rotation pattern)
	// instead of "no_ref". Zero disables this simulation. Meaningless
	// unless < refAfterKicks.
	expiredUntilKick int
	// staleOnFirstKick: when true, a kick #1 that asked for forceStale (the
	// CALLER's staleHint, not anything the page observed) reports
	// why="stale" instead of following refAfterKicks — simulating "the ref
	// is fine, the caller just decided it has been the same code too long".
	staleOnFirstKick bool
	kicks            int
	refreshKicks     int // how many kicks were asked to refresh (doRefresh=true)
	staleKicks       int // how many kicks asked for forceStale=true

	answers map[string]string
}

var keyPattern = regexp.MustCompile(`window\.(__waHeadlessQR_\d+)`)

func (d *pageDouble) eval(_ context.Context, expr string, out *string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	m := keyPattern.FindStringSubmatch(expr)
	if m == nil {
		return fmt.Errorf("pageDouble: no state key found in script: %s", expr)
	}
	key := m[1]
	if d.answers == nil {
		d.answers = map[string]string{}
	}

	switch {
	case strings.Contains(expr, "async () => {"):
		d.kicks++
		doRefresh := strings.Contains(expr, "const doRefresh = true;")
		forceStale := strings.Contains(expr, "const forceStale = true;")
		if doRefresh {
			d.refreshKicks++
		}
		if forceStale {
			d.staleKicks++
		}
		switch {
		case d.staleOnFirstKick && forceStale && d.kicks == 1:
			// The CALLER forced staleness on the first attempt — same shape
			// as "expired": ref present, page fine, caller says too old.
			d.answers[key] = fmt.Sprintf(`{"ok":false,"why":"stale","qr":"","refreshed":%v}`, doRefresh)
		case d.refAfterKicks > 0 && d.kicks >= d.refAfterKicks:
			// Matches the real script: refreshQR is only ever fired inside
			// the `if (!ref)` branch, so a kick that finds ref ALREADY
			// present never sets refreshed, regardless of doRefresh.
			d.answers[key] = fmt.Sprintf(`{"ok":true,"why":"","qr":"ref%d,static,identity,adv,platform","refreshed":false}`, d.kicks)
		case d.expiredUntilKick > 0 && d.kicks <= d.expiredUntilKick:
			d.answers[key] = fmt.Sprintf(`{"ok":false,"why":"expired","qr":"","refreshed":%v}`, doRefresh)
		default:
			d.answers[key] = fmt.Sprintf(`{"ok":false,"why":"no_ref","qr":"","refreshed":%v}`, doRefresh)
		}
		*out = "kicked"
	case strings.Contains(expr, "delete window."):
		delete(d.answers, key)
		*out = "ok"
	default: // poll: window.KEY || ""
		*out = d.answers[key]
	}
	return nil
}

func compressBudgets(t *testing.T) {
	t.Helper()
	oldBudget, oldTick := Budget, Tick
	oldRetries, oldRetryTick := refreshRetries, refreshRetryTick
	Budget, Tick = 2*time.Second, 5*time.Millisecond
	refreshRetries, refreshRetryTick = 5, 5*time.Millisecond
	t.Cleanup(func() {
		Budget, Tick = oldBudget, oldTick
		refreshRetries, refreshRetryTick = oldRetries, oldRetryTick
	})
}

// TestRead_RefAlreadyPresent_NoRefreshNoRetry: the common case — the page
// already has a code. One kick, no refresh fired, no retry loop entered.
func TestRead_RefAlreadyPresent_NoRefreshNoRetry(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{refAfterKicks: 1}
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code == "" {
		t.Fatal("code is empty, want a code — ref was present from the first kick")
	}
	if refreshed {
		t.Fatal("refreshed=true, want false — nothing should have needed a nudge")
	}
	if d.kicks != 1 {
		t.Fatalf("kicks=%d, want 1", d.kicks)
	}
}

// TestRead_RefMissingThenAppears_RefreshesOnceAndRetries: the page has no
// code yet. Read must nudge refreshQR on attempt 0 ONLY, then retry
// (Go-side, no page clock — invariant 6) until the code shows up, without
// nudging again on every retry.
func TestRead_RefMissingThenAppears_RefreshesOnceAndRetries(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{refAfterKicks: 3} // shows up on the 3rd kick
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code == "" {
		t.Fatal("code is empty, want a code — it should have appeared by the 3rd kick")
	}
	if !refreshed {
		t.Fatal("refreshed=false, want true — ref was empty on the first kick")
	}
	if d.kicks != 3 {
		t.Fatalf("kicks=%d, want 3 (1 miss + 1 refresh-carrying miss... reaching kick 3)", d.kicks)
	}
	if d.refreshKicks != 1 {
		t.Fatalf("refreshKicks=%d, want EXACTLY 1 — retries must not re-fire refreshQR every tick", d.refreshKicks)
	}
}

// TestRead_NeverAppears_GivesUpWithinRefreshRetries: ref never populates.
// Read must give up after refreshRetries extra attempts, not hang or loop
// forever, and must still report refreshed=true (the nudge DID fire).
func TestRead_NeverAppears_GivesUpWithinRefreshRetries(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{refAfterKicks: 0}
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code != "" {
		t.Fatalf("code = %q, want empty — ref never populated", code)
	}
	if !refreshed {
		t.Fatal("refreshed=false, want true")
	}
	wantKicks := refreshRetries + 1
	if d.kicks != wantKicks {
		t.Fatalf("kicks=%d, want %d (1 initial + %d retries)", d.kicks, wantKicks, refreshRetries)
	}
	if d.refreshKicks != 1 {
		t.Fatalf("refreshKicks=%d, want EXACTLY 1", d.refreshKicks)
	}
}

// TestRead_RespectsCallerContext: a caller that cancels mid-retry gets
// ctx.Err() back, not a hang — the whole point of moving the wait onto the
// Go side (invariant 6) is that ctx governs it.
func TestRead_RespectsCallerContext(t *testing.T) {
	compressBudgets(t)
	refreshRetries = 1000 // would hang without cancellation
	d := &pageDouble{refAfterKicks: 0}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := New(engine.NewRunner(), d.eval).Read(ctx, "test", false)
	if err == nil {
		t.Fatal("Read returned nil error, want ctx.Err() — the caller's deadline must be honoured")
	}
}

// TestRead_ExpiredOverlay_NudgesAndRecovers: MEASURED (H145,
// TestProbeQRRetryPattern/TestProbeQRExpiredRecovery) — after 6 automatic
// rotations the SPA shows its own "code expired" overlay with Conn.ref
// still POPULATED but stale, and firing refreshQR() while it shows
// recovers in ~1s. Read must treat why="expired" exactly like "no_ref":
// nudge once, retry Go-side, and return the eventual fresh code — never
// silently hand back the stale one, and never treat "expired" as a hard
// error.
func TestRead_ExpiredOverlay_NudgesAndRecovers(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{expiredUntilKick: 2, refAfterKicks: 3}
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code == "" {
		t.Fatal("code is empty, want a code — expired should have recovered by the 3rd kick")
	}
	if !refreshed {
		t.Fatal("refreshed=false, want true — the overlay was showing on the first kick")
	}
	if d.refreshKicks != 1 {
		t.Fatalf("refreshKicks=%d, want EXACTLY 1 — expired must not be nudged again on every retry", d.refreshKicks)
	}
}

// TestRead_StaleHint_ForcesNudgeEvenWithHealthyRef: the case StaleRefreshAfter
// exists for — MEASURED (2026-08-29, TestProbeQRRetryPattern, 3 minutes)
// that the SPA's own rotation has a real gap (one 60s interval among four
// ~20s ones), during which the page sees nothing wrong: ref present, no
// expired overlay. Without staleHint, Read would just keep handing back the
// same code (see TestRead_RefAlreadyPresent_NoRefreshNoRetry — that IS the
// correct behaviour when nobody asked for staleness). With staleHint=true,
// attempt 0 must nudge anyway and wait for a genuinely new code, even
// though the page itself never asked for a refresh.
func TestRead_StaleHint_ForcesNudgeEvenWithHealthyRef(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{refAfterKicks: 1, staleOnFirstKick: true}
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", true)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code == "" {
		t.Fatal("code is empty, want a code — the second kick should have found a healthy ref")
	}
	if !refreshed {
		t.Fatal("refreshed=false, want true — staleHint must fire the nudge on attempt 0")
	}
	if d.kicks != 2 {
		t.Fatalf("kicks=%d, want 2 (1 forced-stale + 1 that finally returns the code)", d.kicks)
	}
	if d.staleKicks != 1 {
		t.Fatalf("staleKicks=%d, want EXACTLY 1 — staleHint only applies to attempt 0, "+
			"never on retries (same rule as doRefresh)", d.staleKicks)
	}
	if d.refreshKicks != 1 {
		t.Fatalf("refreshKicks=%d, want EXACTLY 1", d.refreshKicks)
	}
}

// TestRead_NoStaleHint_NeverForcesNudge is the NEGATIVE CONTROL for
// TestRead_StaleHint_ForcesNudgeEvenWithHealthyRef: with staleHint=false (the
// default a caller with no tracked history passes), a healthy ref on the
// first kick must be returned immediately, exactly like
// TestRead_RefAlreadyPresent_NoRefreshNoRetry — proving the new parameter
// does not change behaviour when the caller does not ask for it.
func TestRead_NoStaleHint_NeverForcesNudge(t *testing.T) {
	compressBudgets(t)
	d := &pageDouble{refAfterKicks: 1, staleOnFirstKick: true}
	code, refreshed, err := New(engine.NewRunner(), d.eval).Read(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if code == "" {
		t.Fatal("code is empty, want a code — ref was present from the first kick")
	}
	if refreshed {
		t.Fatal("refreshed=true, want false — staleHint was false, nothing should have nudged")
	}
	if d.kicks != 1 {
		t.Fatalf("kicks=%d, want 1 — without staleHint, a healthy ref returns immediately", d.kicks)
	}
	if d.staleKicks != 0 {
		t.Fatalf("staleKicks=%d, want 0 — forceStale must never be requested when staleHint is false", d.staleKicks)
	}
}

package spa

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// settleTestRunner keeps StateProbe short so these tests do not spend real
// wall-clock time waiting on a per-call deadline that is not the thing under
// test — only settlePollInterval and the budget passed to WaitForReady are.
func settleTestRunner() *engine.Runner {
	p := engine.DefaultDeadlines
	p.StateProbe = 200 * time.Millisecond
	return &engine.Runner{Policy: p}
}

// countingPage wraps fakePage's eval with a call counter, so a test can
// assert how many probes actually ran — the difference between "returned
// fast because it stopped polling" and "returned fast by coincidence".
type countingPage struct {
	mu    sync.Mutex
	calls int32
	eval  func(ctx context.Context, expression string, out *string) error
}

func (c *countingPage) wrap(f func(ctx context.Context, expression string, out *string) error) {
	c.eval = f
}

func (c *countingPage) call(ctx context.Context, expression string, out *string) error {
	atomic.AddInt32(&c.calls, 1)
	return c.eval(ctx, expression, out)
}

func (c *countingPage) count() int32 { return atomic.LoadInt32(&c.calls) }

// Required test: immediate-ready. A page already mounted must not pay the
// full waiting budget — WaitForReady has to return on its very first probe.
func TestWaitForReady_ImmediateReady_DoesNotPayFullBudget(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/", HasPane: true}}
	counter := &countingPage{}
	counter.wrap(page.eval)

	const budget = 5 * time.Second
	start := time.Now()
	snap, cls := WaitForReady(context.Background(), settleTestRunner(), counter.call, budget, "test")
	elapsed := time.Since(start)

	if cls != ClassAppReady {
		t.Fatalf("class = %q, want %q", cls, ClassAppReady)
	}
	if !snap.HasPane {
		t.Fatal("snapshot lost HasPane on an immediate-ready page")
	}
	if elapsed >= budget {
		t.Fatalf("elapsed=%s >= budget=%s: an already-mounted page paid the full "+
			"waiting budget instead of returning on its first probe", elapsed, budget)
	}
	if counter.count() != 1 {
		t.Fatalf("probe count = %d, want 1: an already-ready page should settle on "+
			"the first look", counter.count())
	}
}

// Required test: never-ready. A page that never mounts must fail cleanly once
// the budget is exhausted, with no orphan behaviour — WaitForReady must
// return (not hang) once its own bounded context is done.
func TestWaitForReady_NeverReady_FailsCleanlyOnBudgetExhaustion(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/"}} // no pane, no QR, ever
	counter := &countingPage{}
	counter.wrap(page.eval)

	const budget = 900 * time.Millisecond
	start := time.Now()
	done := make(chan struct{})
	var snap PageSnapshot
	var cls PageClass
	go func() {
		snap, cls = WaitForReady(context.Background(), settleTestRunner(), counter.call, budget, "test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForReady did not return within 5x its own budget — looks orphaned")
	}
	elapsed := time.Since(start)

	if cls == ClassAppReady {
		t.Fatalf("class = %q, want anything but %q on a page that never mounts", cls, ClassAppReady)
	}
	if snap.URL != "https://web.whatsapp.com/" {
		t.Errorf("final snapshot lost URL=%q", snap.URL)
	}
	// Budget exhaustion, not an instant bail: it must have actually polled
	// more than once, and it must not run drastically past its own budget.
	if elapsed < budget {
		t.Errorf("elapsed=%s < budget=%s: returned before the budget it was given", elapsed, budget)
	}
	if elapsed > budget+2*time.Second {
		t.Errorf("elapsed=%s well past budget=%s: looks like it kept polling after "+
			"its own context should have stopped it", elapsed, budget)
	}
	if counter.count() < 2 {
		t.Errorf("probe count = %d, want >= 2: a never-ready page over a budget "+
			"comfortably longer than settlePollInterval should have been probed more than once",
			counter.count())
	}
}

// Required test: terminal class. A page showing the QR (LOGIN_REQUIRED) must
// terminate FAST with that specific class, not wait out the budget — this
// slice has no pairing and does not wait for a human.
func TestWaitForReady_TerminalClass_StopsFastWithSpecificCause(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/", HasQR: true}}
	counter := &countingPage{}
	counter.wrap(page.eval)

	const budget = 5 * time.Second
	start := time.Now()
	_, cls := WaitForReady(context.Background(), settleTestRunner(), counter.call, budget, "test")
	elapsed := time.Since(start)

	if cls != ClassLoginRequired {
		t.Fatalf("class = %q, want %q — the specific cause must survive, not collapse "+
			"into a generic not-ready", cls, ClassLoginRequired)
	}
	if elapsed >= budget {
		t.Fatalf("elapsed=%s >= budget=%s: a terminal class must not wait out the budget", elapsed, budget)
	}
	if counter.count() != 1 {
		t.Fatalf("probe count = %d, want 1: a terminal class must stop on the first look", counter.count())
	}
}

// Required test: probe error path, transient. A probe that errors on its
// first N calls and then answers is a stalled renderer recovering (phase 6),
// not a terminal condition — WaitForReady must keep polling through it and
// still reach READY inside the budget.
func TestWaitForReady_ProbeErrorIsTransient_RecoversWithinBudget(t *testing.T) {
	var calls int32
	failUntil := int32(3)
	eval := func(ctx context.Context, expression string, out *string) error {
		n := atomic.AddInt32(&calls, 1)
		if n <= failUntil {
			return errors.New("simulated CDP hiccup")
		}
		page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/", HasPane: true}}
		return page.eval(ctx, expression, out)
	}

	const budget = 5 * time.Second
	snap, cls := WaitForReady(context.Background(), settleTestRunner(), eval, budget, "test")

	if cls != ClassAppReady {
		t.Fatalf("class = %q, want %q after the simulated hiccup cleared", cls, ClassAppReady)
	}
	if !snap.HasPane {
		t.Fatal("final snapshot lost HasPane once the page recovered")
	}
	if atomic.LoadInt32(&calls) <= failUntil {
		t.Fatalf("only %d calls were made; the loop did not actually retry past the failures",
			atomic.LoadInt32(&calls))
	}
}

// Required test: probe error path, terminal-by-exhaustion. A probe that
// NEVER stops erroring is a renderer that never recovers — ClassifyProbe
// folds every such failure into ClassUnresponsive (probe.go), and
// WaitForReady must still return cleanly once its budget runs out rather
// than hang waiting for a target that stopped answering at all.
func TestWaitForReady_ProbeErrorNeverRecovers_FailsCleanlyAsUnresponsive(t *testing.T) {
	eval := func(ctx context.Context, expression string, out *string) error {
		return errors.New("simulated permanent CDP failure")
	}

	const budget = 900 * time.Millisecond
	start := time.Now()
	done := make(chan struct{})
	var cls PageClass
	go func() {
		_, cls = WaitForReady(context.Background(), settleTestRunner(), eval, budget, "test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForReady did not return within 5x its own budget — looks orphaned")
	}
	elapsed := time.Since(start)

	if cls != ClassUnresponsive {
		t.Fatalf("class = %q, want %q — every probe on this page errored", cls, ClassUnresponsive)
	}
	if elapsed < budget {
		t.Errorf("elapsed=%s < budget=%s: returned before the budget it was given", elapsed, budget)
	}
}

// The caller's external context stays sovereign: it must win even when it is
// SHORTER than the budget passed to WaitForReady.
func TestWaitForReady_ExternalContextIsSovereignOverTheBudget(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/"}} // never ready
	counter := &countingPage{}
	counter.wrap(page.eval)

	const shortCtx = 400 * time.Millisecond
	const longBudget = 30 * time.Second // deliberately far longer than shortCtx
	ctx, cancel := context.WithTimeout(context.Background(), shortCtx)
	defer cancel()

	start := time.Now()
	_, cls := WaitForReady(ctx, settleTestRunner(), counter.call, longBudget, "test")
	elapsed := time.Since(start)

	if cls == ClassAppReady {
		t.Fatalf("class = %q, unexpected on a page that never mounts", cls)
	}
	if elapsed > shortCtx+2*time.Second {
		t.Fatalf("elapsed=%s, want close to the external ctx=%s, not the much longer "+
			"budget=%s — the caller's context must stay sovereign", elapsed, shortCtx, longBudget)
	}
}

package engine

// Driving a page: attaching a tab to a launched browser, navigating it, and
// evaluating in it.
//
// This is the last place chromedp appears. Everything above talks to Evaluate
// and Navigate, so ADR-0006 D1 stays a decision recorded in one package rather
// than a dependency spread across the tree — enforced by ../gate_test.go.

import (
	"context"
	"errors"
	"time"

	"github.com/chromedp/chromedp"
)

// Tab is one page attached to a browser.
//
// Its context is the tab's LIFETIME, and it is deliberately not the context of
// any single operation: chromedp creates the target lazily on the first Run and
// binds the goroutines that manage it to whatever context that Run was given.
// Handing it a per-operation context would kill the target when that operation
// finished — the defect PrimeTab exists to prevent.
type Tab struct {
	ctx         context.Context
	cancelTab   context.CancelFunc
	cancelAlloc context.CancelFunc
}

// OpenTab attaches to an already-running browser and materialises one tab.
//
// The browser must be running: this connects to its CDP endpoint, it does not
// start anything. Closing the tab does NOT stop the browser — that is CleanStop's
// job, and conflating the two is how a shutdown ends up bypassing the protocol.
func OpenTab(parent context.Context, b *Browser) (*Tab, error) {
	return OpenTabWithin(parent, b, DefaultDeadlines.For(OpBoot))
}

// OpenTabWithin is OpenTab with an explicit bound on the PRIMING.
//
// THE BOUND IS ON THE WAIT, NOT ON THE CONTEXT, and that distinction is the
// whole design. The comment on Tab explains why PrimeTab must receive the tab's
// own context: chromedp binds the target's goroutines to whatever context the
// first Run is given, so a per-operation context would kill the target the
// moment that operation finished. Wrapping tabCtx in a WithTimeout would
// therefore trade an unbounded hang for a tab that dies on a timer, which is
// worse — it would break invariant 15 (a session's lifetime ends only at Stop).
//
// So priming runs on tabCtx, unbounded as it must be, and the CALLER's wait for
// it is what carries the deadline. On expiry the tab is closed, which cancels
// tabCtx and unblocks the priming goroutine — it does not leak.
//
// WHY THIS EXISTS (HOUSEKEEP H36). OpenTab is one of the few browser entry
// points that does not go through Runner, so no deadline policy applied to it.
// On 2026-08-20, under host contention, a priming that never answered hung
// TestBrowserChainVerifiesTheModuleInventory for 18m28s until the package's own
// 20-minute timeout killed the suite. Two costs, and the second is worse than
// the first: the failure read as "20 minutes" instead of "the tab never
// primed", and the panic skipped every defer, so the browser was left running
// — measured alive and idle 19 minutes later, on a host that was already short
// of resources.
func OpenTabWithin(parent context.Context, b *Browser, primeBudget time.Duration) (*Tab, error) {
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(parent, b.WebSocketURL())
	tabCtx, cancelTab := chromedp.NewContext(alloc)

	t := &Tab{ctx: tabCtx, cancelTab: cancelTab, cancelAlloc: cancelAlloc}

	// Buffered, so the goroutine can always finish its send and exit even when
	// nobody is left waiting for it.
	done := make(chan error, 1)
	go func() { done <- PrimeTab(tabCtx) }()

	timer := time.NewTimer(primeBudget)
	defer timer.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Close()
			return nil, err
		}
		return t, nil
	case <-timer.C:
		// Close FIRST, then report. Closing is what unblocks the goroutine, and
		// reporting before releasing would be the "returned an error and left
		// the resource" shape this module has met before.
		t.Close()
		return nil, &TimeoutError{Op: OpBoot, Label: "open_tab/prime", Deadline: primeBudget}
	case <-parent.Done():
		t.Close()
		return nil, parent.Err()
	}
}

// Context is the tab's lifetime context.
func (t *Tab) Context() context.Context { return t.ctx }

// Close releases the tab. It is safe to call more than once.
func (t *Tab) Close() {
	if t.cancelTab != nil {
		t.cancelTab()
	}
	if t.cancelAlloc != nil {
		t.cancelAlloc()
	}
}

// Screenshot captures the tab's viewport as PNG bytes.
//
// It lives here, and not in the caller that wanted it, because ADR-0006 D1 puts
// the driver behind this package: engine/ is the only directory allowed to
// import chromedp, and the module's gate test enforces it. A screenshot helper
// written next to its user would have spread the dependency across the tree —
// which is exactly what the gate caught when the QR capture tools were first
// written in realspa_test.go.
//
// THE IMAGE IS PAGE CONTENT. Unlike everything else this package returns —
// classes, states, durations, pids — a screenshot carries whatever the page was
// showing: messages, names, and on a pairing screen the QR itself, which is a
// credential. Callers own where it goes; nothing in this module writes it to a
// log or to the repository.
func (t *Tab) Screenshot(r *Runner, label string) ([]byte, error) {
	var png []byte
	err := r.Do(t.ctx, OpStateProbe, label, func(ctx context.Context) error {
		runCtx, cancel := t.derive(ctx)
		defer cancel()
		return chromedp.Run(runCtx, chromedp.CaptureScreenshot(&png))
	})
	if err != nil {
		return nil, err
	}
	if len(png) == 0 {
		// An empty capture is not a blank page: chromedp returns no error when
		// the target is gone mid-capture, so the emptiness IS the signal.
		return nil, errors.New("engine: screenshot came back empty")
	}
	return png, nil
}

// Navigate points the tab at a URL under the Navigate budget.
func (t *Tab) Navigate(r *Runner, url, label string) error {
	return r.Do(t.ctx, OpNavigate, label, func(ctx context.Context) error {
		runCtx, cancel := t.derive(ctx)
		defer cancel()
		return chromedp.Run(runCtx, chromedp.Navigate(url))
	})
}

// Evaluate runs an expression and writes its result into out.
//
// The context handling here is the whole subtlety of this file, and getting it
// wrong produced a chain where every evaluation failed and every page
// classified as UNRESPONSIVE — a browser reported dead by a bug in the code
// asking it questions.
//
// chromedp.Run demands a context carrying ITS values, so it must be a
// descendant of the tab's. The budget, however, belongs to the caller. Neither
// context can simply replace the other:
//
//   - passing the caller's context alone: chromedp rejects it outright;
//   - passing the tab's alone: every deadline above this line silently vanishes,
//     and a page that stops executing JavaScript hangs instead of being
//     classified — which is the phase 6 failure, reintroduced.
//
// So the run context is a CHILD of the tab that inherits the caller's deadline
// and cancellation. Cancelling that child does not destroy the target: the
// target's lifetime is bound to the context of the FIRST Run, which PrimeTab
// already made on the tab's own context.
//
// It matches the spa.Evaluator signature so that package can stay free of
// chromedp; the method value is passed across as a plain function.
func (t *Tab) Evaluate(ctx context.Context, expression string, out *string) error {
	runCtx, cancel := t.derive(ctx)
	defer cancel()
	return chromedp.Run(runCtx, chromedp.Evaluate(expression, out))
}

// derive builds a chromedp-capable context bounded by the caller's.
func (t *Tab) derive(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(t.ctx, deadline)
	}
	runCtx, cancel := context.WithCancel(t.ctx)
	// No deadline, but cancellation still has to travel: a caller that gives up
	// must not leave an evaluation running against a page nobody is waiting for.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-stop:
		}
	}()
	return runCtx, func() { close(stop); cancel() }
}

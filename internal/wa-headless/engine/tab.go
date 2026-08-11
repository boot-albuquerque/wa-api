package engine

// Driving a page: attaching a tab to a launched browser, navigating it, and
// evaluating in it.
//
// This is the last place chromedp appears. Everything above talks to Evaluate
// and Navigate, so ADR-0006 D1 stays a decision recorded in one package rather
// than a dependency spread across the tree — enforced by ../gate_test.go.

import (
	"context"

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
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(parent, b.WebSocketURL())
	tabCtx, cancelTab := chromedp.NewContext(alloc)

	t := &Tab{ctx: tabCtx, cancelTab: cancelTab, cancelAlloc: cancelAlloc}
	if err := PrimeTab(tabCtx); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
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

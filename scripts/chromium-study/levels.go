package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// The three abstraction levels of Part H.
//
// All three run on the SAME chromedp context and the SAME connection. Only the
// implementation of each operation differs, so the measured difference is the
// abstraction level and nothing else. Comparing a hand-rolled CDP client
// against chromedp would have confounded transport with abstraction.
const (
	L0 = 0 // raw Runtime.evaluate — fewest guarantees
	L1 = 1 // cdproto replicating chromedp's guarantees call for call
	L2 = 2 // chromedp high-level
)

// visibleFn replicates chromedp's js/visible.js: a node counts as visible when
// it has a non-zero box or any client rect.
const visibleFn = `function(){return this.offsetWidth>0||this.offsetHeight>0||this.getClientRects().length>0}`

// chromedpRetryInterval is chromedp's Selector default (query.go:136). L1 uses
// the same value; a different poll interval would show up as a latency
// difference that has nothing to do with abstraction.
const chromedpRetryInterval = 5 * time.Millisecond

// truth mirrors window.__truth from the hostile page: what the page believes
// actually happened, independent of whether the controller reported success.
type truth struct {
	RealKeyEvents      int    `json:"realKeyEvents"`
	SubmitViaMouse     int    `json:"submitViaMouse"`
	SubmitViaSynthetic int    `json:"submitViaSynthetic"`
	OverlayWasUp       bool   `json:"overlayWasUp"`
	CommittedEmail     string `json:"committedEmail"`
}

// JobOutcome separates "the controller returned nil" from "the job was done".
type JobOutcome struct {
	Err            error
	CommittedEmail string
	RealKeys       int
	MouseClicks    int
	SyntheticClick int
	// FalseSuccess is the dangerous case: no error, but the page state proves
	// the work did not happen the way a user would have done it.
	FalseSuccess bool
}

const wantEmail = "a@b.c"

// ---------------------------------------------------------------------------
// L1 primitives: cdproto, with chromedp's guarantees reproduced explicitly.
// ---------------------------------------------------------------------------

// l1Query polls for a node exactly as chromedp's Selector does: re-resolve the
// document and re-run the query every retryInterval, so a node replaced between
// attempts is picked up fresh instead of returning a stale id.
func l1Query(ctx context.Context, sel string) (cdp.NodeID, error) {
	deadline := time.Now().Add(JobTimeout)
	for time.Now().Before(deadline) {
		doc, err := dom.GetDocument().Do(ctx)
		if err == nil {
			id, err := dom.QuerySelector(doc.NodeID, sel).Do(ctx)
			if err == nil && id != 0 {
				return id, nil
			}
		}
		time.Sleep(chromedpRetryInterval)
	}
	return 0, fmt.Errorf("l1: %q not found", sel)
}

// l1WaitVisible reproduces chromedp's NodeVisible: box model AND the visible.js
// predicate. Either check alone accepts nodes chromedp would reject.
func l1WaitVisible(ctx context.Context, sel string) (cdp.NodeID, error) {
	deadline := time.Now().Add(JobTimeout)
	for time.Now().Before(deadline) {
		id, err := l1Query(ctx, sel)
		if err != nil {
			return 0, err
		}
		if _, err := dom.GetBoxModel().WithNodeID(id).Do(ctx); err == nil {
			obj, err := dom.ResolveNode().WithNodeID(id).Do(ctx)
			if err == nil && obj.ObjectID != "" {
				var vis bool
				res, exc, err := runtime.CallFunctionOn(visibleFn).
					WithObjectID(obj.ObjectID).WithReturnByValue(true).Do(ctx)
				_ = runtime.ReleaseObject(obj.ObjectID).Do(ctx)
				if err == nil && exc == nil && res != nil {
					if json.Unmarshal([]byte(res.Value), &vis) == nil && vis {
						return id, nil
					}
				}
			}
		}
		time.Sleep(chromedpRetryInterval)
	}
	return 0, fmt.Errorf("l1: %q never visible", sel)
}

// l1Click reproduces chromedp's MouseClickNode (input.go:57): scroll into view,
// content quads, centroid, then REAL mouse events at those coordinates. Real
// events are hit-tested by the browser, so an overlay intercepts them — which
// is the entire behavioural difference against element.click().
func l1Click(ctx context.Context, sel string) error {
	id, err := l1WaitVisible(ctx, sel)
	if err != nil {
		return err
	}
	if err := dom.ScrollIntoViewIfNeeded().WithNodeID(id).Do(ctx); err != nil {
		return err
	}
	quads, err := dom.GetContentQuads().WithNodeID(id).Do(ctx)
	if err != nil {
		return err
	}
	if len(quads) == 0 || len(quads[0]) < 2 || len(quads[0])%2 != 0 {
		return fmt.Errorf("l1: no content quads for %q", sel)
	}
	var x, y float64
	q := quads[0]
	for i := 0; i < len(q); i += 2 {
		x += q[i]
		y += q[i+1]
	}
	n := float64(len(q) / 2)
	x, y = x/n, y/n

	if err := input.DispatchMouseEvent(input.MousePressed, x, y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return err
	}
	return input.DispatchMouseEvent(input.MouseReleased, x, y).
		WithButton(input.Left).WithClickCount(1).Do(ctx)
}

// l1Fill reproduces chromedp's KeyEventNode: focus the node, then dispatch real
// key events per rune via the same keyboard encoding chromedp uses. Assigning
// .value instead would skip the framework's change path entirely.
func l1Fill(ctx context.Context, sel, text string) error {
	id, err := l1WaitVisible(ctx, sel)
	if err != nil {
		return err
	}
	if err := dom.Focus().WithNodeID(id).Do(ctx); err != nil {
		return err
	}
	for _, r := range text {
		for _, k := range kb.Encode(r) {
			if err := k.Do(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// l1Text resolves the node and reads through it, rather than re-querying by
// selector inside the page.
func l1Text(ctx context.Context, sel string) (string, error) {
	id, err := l1Query(ctx, sel)
	if err != nil {
		return "", err
	}
	obj, err := dom.ResolveNode().WithNodeID(id).Do(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = runtime.ReleaseObject(obj.ObjectID).Do(ctx) }()
	res, exc, err := runtime.CallFunctionOn(`function(){return this.textContent}`).
		WithObjectID(obj.ObjectID).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return "", err
	}
	if exc != nil {
		return "", fmt.Errorf("l1: %s", exc.Text)
	}
	var s string
	_ = json.Unmarshal([]byte(res.Value), &s)
	return s, nil
}

// ---------------------------------------------------------------------------
// L0 primitives: one Runtime.evaluate each, no node resolution, no hit test.
// ---------------------------------------------------------------------------

func l0Eval(ctx context.Context, expr string, out any) error {
	res, exc, err := runtime.Evaluate(expr).WithReturnByValue(true).WithAwaitPromise(true).Do(ctx)
	if err != nil {
		return err
	}
	if exc != nil {
		return fmt.Errorf("l0: %s", exc.Text)
	}
	if out != nil && res != nil {
		return json.Unmarshal([]byte(res.Value), out)
	}
	return nil
}

func l0WaitFor(ctx context.Context, expr string) error {
	deadline := time.Now().Add(JobTimeout)
	for time.Now().Before(deadline) {
		var ok bool
		if err := l0Eval(ctx, expr, &ok); err == nil && ok {
			return nil
		}
		time.Sleep(chromedpRetryInterval)
	}
	return fmt.Errorf("l0: timeout on %s", expr)
}

// ---------------------------------------------------------------------------
// The job, at each level, against the hostile page.
// ---------------------------------------------------------------------------

func runHostileJob(ctx context.Context, level int, pageURL string, rec *Rec) (JobOutcome, error) {
	var out JobOutcome

	if err := rec.timed("navigate", func() error {
		return chromedp.Run(ctx, chromedp.Navigate(pageURL))
	}); err != nil {
		out.Err = err
		return out, err
	}

	act := func(name string, f func(context.Context) error) error {
		return rec.timed(name, func() error {
			return chromedp.Run(ctx, chromedp.ActionFunc(f))
		})
	}

	switch level {
	case L2:
		if err := rec.timed("wait_ready", func() error {
			return chromedp.Run(ctx, chromedp.WaitReady("#app-ready", chromedp.ByQuery))
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := rec.timed("fill", func() error {
			return chromedp.Run(ctx, chromedp.SendKeys("#email", wantEmail, chromedp.ByQuery))
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := rec.timed("click", func() error {
			return chromedp.Run(ctx, chromedp.Click("#submit", chromedp.ByQuery))
		}); err != nil {
			out.Err = err
			return out, err
		}
	case L1:
		if err := act("wait_ready", func(ctx context.Context) error {
			_, err := l1Query(ctx, "#app-ready")
			return err
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := act("fill", func(ctx context.Context) error {
			return l1Fill(ctx, "#email", wantEmail)
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := act("click", func(ctx context.Context) error {
			return l1Click(ctx, "#submit")
		}); err != nil {
			out.Err = err
			return out, err
		}
	default: // L0
		if err := act("wait_ready", func(ctx context.Context) error {
			return l0WaitFor(ctx, `!!document.getElementById('app-ready')`)
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := act("fill", func(ctx context.Context) error {
			return l0Eval(ctx, fmt.Sprintf(
				`(()=>{const e=document.getElementById('email');e.value=%q;return true})()`, wantEmail), nil)
		}); err != nil {
			out.Err = err
			return out, err
		}
		if err := act("click", func(ctx context.Context) error {
			return l0Eval(ctx, `(()=>{document.getElementById('submit').click();return true})()`, nil)
		}); err != nil {
			out.Err = err
			return out, err
		}
	}

	if err := act("wait_result", func(ctx context.Context) error {
		return l0WaitFor(ctx, `!!document.querySelector('#result[data-done="1"]')`)
	}); err != nil {
		out.Err = err
		return out, err
	}

	// Ground truth, read the same way for every level so the verification
	// itself cannot favour one of them.
	if err := act("verify", func(ctx context.Context) error {
		var t truth
		if err := l0Eval(ctx, `JSON.stringify(window.__truth)`, new(string)); err != nil {
			return err
		}
		var raw string
		if err := l0Eval(ctx, `JSON.stringify(window.__truth)`, &raw); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return err
		}
		out.CommittedEmail = t.CommittedEmail
		out.RealKeys = t.RealKeyEvents
		out.MouseClicks = t.SubmitViaMouse
		out.SyntheticClick = t.SubmitViaSynthetic
		return nil
	}); err != nil {
		out.Err = err
		return out, err
	}

	// A job "succeeded" only if the framework committed the value a user would
	// have typed AND the submit came from a hit-tested mouse event.
	out.FalseSuccess = out.CommittedEmail != wantEmail || out.MouseClicks == 0
	return out, nil
}

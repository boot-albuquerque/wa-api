package main

// Track A ablation, second attempt.
//
// The first hypothesis — that one shared CDP connection serialized rod — was
// REFUTED by its own control arm: chromedp forced onto a single connection kept
// its throughput (~20 jobs/s at conc 4), and rod given one connection per
// worker did not improve. Both directions failed, so connection count is not
// the mechanism and the arm below replaces it.
//
// The second hypothesis comes from the block profile rather than from reading
// source and guessing. Of rod's 960 s of accumulated goroutine blocking over 80
// jobs, 136.5 s sits under Element.Focus, and 99.91% of that is
// Element.ScrollIntoView. rod's ScrollIntoView calls WaitStableRAF
// (element.go:72), which loops on Page.WaitRepaint until the element's content
// quads stop moving, and WaitRepaint is literally:
//
//	() => new Promise(r => requestAnimationFrame(r))     (page.go:851)
//
// Phase 1 of this study already measured that requestAnimationFrame does not
// fire on a schedule in a backgrounded CDP target — that is why the readiness
// marker had to be rewritten synchronously. So every rod Input() and Click()
// waits, inside the browser, for at least two animation frames that the
// compositor is under no obligation to produce promptly.
//
// That predicts the exact latency shape observed: a healthy p50 (~350 ms, when
// frames happen to arrive) and a p99 of ~20 s (when they do not). It also
// explains why the two earlier fixes failed — more connections cannot speed up
// a wait that happens in the renderer, and a faster Sleeper cannot either,
// because this is a promise, not a poll loop.
//
// This arm keeps rod, keeps its connection, keeps its element resolution, and
// removes ONLY the repaint-stability wait, replacing it with what chromedp does
// at the same point: DOM.scrollIntoViewIfNeeded, then real input events at the
// element's measured centroid. Semantics are preserved — the events are still
// hit-tested by the browser, so an overlay still intercepts them — which is
// what makes the comparison legitimate rather than a repeat of the L0 mistake.
//
// Prediction, stated before the run: throughput rises toward chromedp's and p99
// collapses. If p99 stays at ~20 s, WaitStableRAF is not the cause and this
// hypothesis dies too.

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

type rodNoRAF struct {
	rodCtl
}

func (r *rodNoRAF) Name() string { return "rod-noraf" }

// centroid averages the first content quad, the same way chromedp's
// MouseClickNode does, so the click lands on the same pixel in both libraries.
func centroid(q *proto.DOMGetContentQuadsResult) (proto.Point, error) {
	if q == nil || len(q.Quads) == 0 || len(q.Quads[0]) < 8 {
		return proto.Point{}, fmt.Errorf("no content quads")
	}
	raw := q.Quads[0]
	var x, y float64
	n := 0
	for i := 0; i+1 < len(raw); i += 2 {
		x += raw[i]
		y += raw[i+1]
		n++
	}
	return proto.Point{X: x / float64(n), Y: y / float64(n)}, nil
}

// scrollIntoView issues the CDP command WITHOUT rod's stability loop.
func scrollIntoView(el *rod.Element) error {
	return proto.DOMScrollIntoViewIfNeeded{ObjectID: el.Object.ObjectID}.Call(el)
}

func (r *rodNoRAF) RunJob(ctx context.Context, pageURL string, rec *Rec) error {
	br := r.pick()
	var p *rod.Page
	var err error
	if err = rec.timed("page_create", func() error {
		p, err = br.Page(proto.TargetCreateTarget{URL: "about:blank"})
		return err
	}); err != nil {
		return err
	}
	defer func() { _ = p.Close() }()
	p = p.Timeout(JobTimeout)
	// The sleeper arm is stacked on top of the rAF arm deliberately: Phase 2
	// tested the sleeper ALONE and it did not move concurrency, but that does not
	// rule out an interaction where the rAF stall was simply the larger of two
	// serial costs and hid the other.
	if r.fastPoll {
		p = p.Sleeper(fastSleeper)
	}

	if err := rec.timed("navigate", func() error { return p.Navigate(pageURL) }); err != nil {
		return err
	}
	if err := rec.timed("wait_ready", func() error {
		_, err := p.Element("#app-ready")
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("dom_read", func() error {
		el, err := p.Element("#title")
		if err != nil {
			return err
		}
		_, err = el.Text()
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("fill", func() error {
		el, err := p.Element("#email")
		if err != nil {
			return err
		}
		if err := scrollIntoView(el); err != nil {
			return err
		}
		// DOM.focus instead of rod's Element.Focus, which routes through
		// ScrollIntoView and therefore through WaitStableRAF.
		return (proto.DOMFocus{ObjectID: el.Object.ObjectID}).Call(el)
	}); err != nil {
		return err
	}
	// Typing is a separate step so its cost is attributed to the keystrokes and
	// not to the focus above. Real key events, same as chromedp's SendKeys: a
	// controlled React-style input only commits on isTrusted events, and the
	// ground-truth check would catch a shortcut here as a false success.
	if err := rec.timed("fill_keys", func() error {
		for _, r := range "a@b.c" {
			if err := p.Keyboard.Type(input.Key(r)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := rec.timed("click", func() error {
		el, err := p.Element("#submit")
		if err != nil {
			return err
		}
		if err := scrollIntoView(el); err != nil {
			return err
		}
		shape, err := el.Shape()
		if err != nil {
			return err
		}
		pt, err := centroid(shape)
		if err != nil {
			return err
		}
		if err := p.Mouse.MoveTo(pt); err != nil {
			return err
		}
		if err := p.Mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
			return err
		}
		return p.Mouse.Up(proto.InputMouseButtonLeft, 1)
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_result", func() error {
		_, err := p.Element(`#result[data-done="1"]`)
		return err
	}); err != nil {
		return err
	}
	return rec.timed("evaluate", func() error {
		_, err := p.Eval(`() => document.querySelectorAll('.item').length`)
		return err
	})
}

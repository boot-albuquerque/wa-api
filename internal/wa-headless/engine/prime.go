package engine

// Target priming: materialise the browser target BEFORE any deadline-bounded
// operation touches it.
//
// This exists because of an interaction between chromedp and the deadline
// policy that cost a whole run of the study, and it is not discoverable by
// reading either piece alone.
//
// In chromedp a target is created lazily, on the first Run. The goroutines that
// manage it derive from the context passed to THAT first Run. Runner.Do always
// cancels its bounded child context when it returns — that is what makes the
// deadline real — so letting the target be created inside a Do kills the target
// the moment the first operation finishes. Every subsequent operation on the
// same tab then fails instantly with "context canceled", never with a deadline,
// which reads like a dead browser and is in fact a dead context.
//
// The study's own self-test did not expose it: each case used a fresh tab and a
// single operation, and the interaction only appears from the SECOND operation
// on the same tab onward.
//
// So the first Run happens on the tab's own lifetime context, with no deadline
// of its own. That is defensible precisely because it is not a remote wait: it
// is local target creation. The session-level budget remains the upper bound.
//
// Study origin: scripts/chromium-study/p4c_target.go.

import (
	"context"

	"github.com/chromedp/chromedp"
)

// PrimeTab materialises the target of a chromedp context.
//
// It must be called ONCE per tab, on the tab's own context, and never from
// inside Runner.Do — passing it a bounded child would recreate exactly the
// defect it exists to prevent.
func PrimeTab(tab context.Context) error {
	return chromedp.Run(tab)
}

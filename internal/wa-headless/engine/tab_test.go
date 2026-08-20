package engine

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// TestOpenTabStopsWaitingWhenPrimingNeverAnswers is HOUSEKEEP H36's regression.
//
// It uses a listener that ACCEPTS and then says nothing — a refused connection
// fails fast, and only the accepted-but-silent case leaves chromedp waiting. No
// sleep stands in for it, because a sleep would prove only that the timer works.
//
// WHAT THIS DOES AND DOES NOT REPRODUCE, because the negative control said so
// rather than the comment guessing. With the bound removed, this path does not
// hang forever: it fails after 10s with `could not dial ... context deadline
// exceeded`, which is chromedp's OWN dial timeout. So the blocking used here
// sits at the DIAL, while the field hang — 18m28s, with the stack in
// RemoteAllocator.Allocate on a channel receive — was past it.
//
// The test is therefore honest about its reach: it proves the budget is applied
// and that the failure is classified as this operation's own timeout rather
// than left to a driver's wording. It does NOT reproduce the exact field
// condition, which would need a CDP endpoint that completes the handshake and
// then goes silent. The bound covers both by construction, since it wraps the
// whole wait rather than any one stage.
func TestOpenTabStopsWaitingWhenPrimingNeverAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan struct{}, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold it open and answer nothing. Closing here would let chromedp
			// fail fast and the test would pass for the wrong reason.
			select {
			case accepted <- struct{}{}:
			default:
			}
			defer c.Close()
		}
	}()

	b := &Browser{wsURL: "ws://" + ln.Addr().String() + "/devtools/browser/never"}

	start := time.Now()
	tab, err := OpenTabWithin(context.Background(), b, 400*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		tab.Close()
		t.Fatal("OpenTabWithin succeeded against an endpoint that never answers")
	}
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("got %T (%v), want *TimeoutError naming the priming", err, err)
	}
	if te.Op != OpBoot {
		t.Fatalf("the timeout is classified as %s, want %s", te.Op, OpBoot)
	}
	// The bound is the point. Unbounded, the negative control measured 10s here
	// (chromedp's dial timeout) and 18m28s in the field; 400ms must mean 400ms.
	if elapsed > 5*time.Second {
		t.Fatalf("waited %s for a 400ms budget: the deadline is not being applied", elapsed)
	}
}

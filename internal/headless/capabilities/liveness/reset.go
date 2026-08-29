package liveness

// Reconnecting the socket, and proving it came back.
//
// This is Client.resetState. The reference calls Socket.reconnect() and returns
// nothing at all — no error, no state, no confirmation — which makes it the
// purest silent success in the whole surface: a caller cannot distinguish "the
// socket was reset" from "the module name changed and nothing happened".
//
// THE TRANSITION IS REAL BUT SHORT, and measuring it took two attempts (H116).
// Sampling __x_state every 500ms after a reconnect showed CONNECTED in 24 of 24
// samples, which reads exactly like "reconnect does nothing". Sampling every
// 50ms showed OPENING in 9 of 100 — a window of roughly 450ms that the slower
// sampler stepped over. The zero was the instrument, not the world.
//
// So the postcondition here is the SETTLE, not the transition: seeing OPENING is
// reported when it happens and never required, because requiring a 450ms window
// to land inside a sampling schedule is how a gate starts failing under load
// (F100).

import (
	"context"
	"fmt"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

var (
	// ErrResetRefused is the page failing to start the reconnect.
	ErrResetRefused = fmt.Errorf("liveness: the page refused to reconnect the socket")
	// ErrNeverSettled is the postcondition: the socket did not come back.
	//
	// IT IS ITS OWN ERROR because the repair differs from a refusal. A refusal
	// means the module is gone; a socket that never settles means the session is
	// now WORSE than before the reset, which a caller must be able to act on.
	ErrNeverSettled = fmt.Errorf("liveness: the socket did not return to CONNECTED after the reset")
)

// Reset budgets. Var so a test can compress them.
//
// ResetTick is deliberately far below the ~450ms transition measured in H116, so
// that SawOpening is usually true — but nothing depends on it being true.
var (
	ResetBudget = 30 * time.Second
	ResetTick   = 50 * time.Millisecond
)

// minSettleReads is how many state samples must be taken before a CONNECTED can
// count as "came back" rather than "never left". Four, because the measured
// OPENING window is ~450ms and ResetTick is 50ms: four samples cover 200ms of
// it, which is enough to have LEFT the starting state without demanding that the
// window be caught.
const minSettleReads = 4

// ResetOutcome is what a reset did.
type ResetOutcome struct {
	// Settled is the postcondition: the socket read CONNECTED again. A false
	// here always comes with an error.
	Settled bool
	// SawOpening reports whether the sampler caught the socket mid-reconnect.
	//
	// IT IS INFORMATION AND, FROM GO, USUALLY FALSE. Measured live: 0 of 3 resets
	// caught it from here, while the SAME reconnect sampled INSIDE the page at
	// 50ms showed OPENING in 9 of 100 samples. Each Go-side read is a chromedp
	// round trip, so the effective sampling rate is far coarser than the ~450ms
	// window (H116).
	//
	// The consequence is stated rather than hidden: this postcondition proves the
	// socket is HEALTHY after the call, not that the call did anything. Proving
	// the transition would need the sampling to happen in the page and be parked
	// for Go to collect — which is the store-and-poll shape invariant 6 already
	// prescribes, but with a page-side interval that would need its own entry in
	// the invariant-6 allow-list. That is a deliberate decision, not an
	// oversight, and it is why the ledger row is PARTIAL.
	SawOpening bool
	// Took is how long the settle took, measured from the reconnect call.
	Took time.Duration
	// Before is the state the socket was in when asked to reset.
	Before spa.SocketState
}

func (o ResetOutcome) String() string {
	return fmt.Sprintf("liveness.ResetOutcome(settled=%t sawOpening=%t took=%s before=%s)",
		o.Settled, o.SawOpening, o.Took.Round(time.Millisecond), o.Before)
}

// Reset reconnects the socket and PROVES it came back.
func Reset(ctx context.Context, runner *engine.Runner, eval spa.Evaluator, label string) (ResetOutcome, error) {
	before, err := readSocket(ctx, runner, eval, label+"/before")
	if err != nil {
		return ResetOutcome{}, err
	}
	out := ResetOutcome{Before: before}

	var ack string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/reset", func(c context.Context) error {
		return eval(c, resetScript, &ack)
	}); err != nil {
		return out, fmt.Errorf("%w: %v", ErrResetRefused, err)
	}
	if ack != resetAck {
		return out, fmt.Errorf("%w: the page answered %q", ErrResetRefused, ack)
	}

	started := time.Now()
	deadline := started.Add(ResetBudget)
	reads := 0
	for {
		state, err := readSocket(ctx, runner, eval, label+"/settle")
		if err != nil {
			return out, err
		}
		if state == spa.SocketStateOpening {
			out.SawOpening = true
		}
		// THE SETTLE IS ONLY MEANINGFUL AFTER THE CALL. Reading CONNECTED on the
		// first tick could be the state that was never left, so a reset that has
		// not yet begun would verify itself. The loop therefore keeps going until
		// it has either seen OPENING or taken minSettleReads samples.
		//
		// THE MINIMUM IS COUNTED IN READS, NOT IN TIME. The first version spent a
		// minimum WALL-CLOCK window, which is the load-sensitive shape F100 was
		// about: on a busy machine the same code would sample fewer times inside
		// the same window and the guard would weaken exactly when the system is
		// least healthy. A count does not move.
		reads++
		if state == spa.SocketStateConnected && (out.SawOpening || reads >= minSettleReads) {
			out.Settled = true
			out.Took = time.Since(started)
			return out, nil
		}
		if !time.Now().Before(deadline) {
			out.Took = time.Since(started)
			return out, fmt.Errorf("%w after %s (last state %q)", ErrNeverSettled,
				out.Took.Round(time.Millisecond), state)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(ResetTick):
		}
	}
}

func readSocket(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	label string) (spa.SocketState, error) {
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, label, func(c context.Context) error {
		return eval(c, spa.SocketStateReadExpr, &raw)
	}); err != nil {
		return spa.SocketStateUnread, fmt.Errorf("liveness: reading the socket state: %w", err)
	}
	return spa.SocketState(raw), nil
}

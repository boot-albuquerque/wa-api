package runtime

import (
	"context"
	"errors"
	"sync"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
)

// ErrHolderStopped is returned by Session after Stop. A stopped Holder is
// finished, not idle: it does not boot again.
//
// The alternative — letting Stop return the Holder to a bootable state — was
// rejected because it makes "is this profile owned?" depend on timing rather
// than on a decision. A caller that wants another session after stopping this
// one constructs another Holder, which is an explicit act and which the
// ownership check in core can see.
var ErrHolderStopped = errors.New("runtime: holder already stopped")

// ErrSessionDied is returned by Session when the held session's browser process
// is gone — it crashed, was OOM-killed, or was killed from outside.
//
// The Holder refuses rather than re-booting, and that is a decision, not an
// omission. Re-booting silently would (a) hide a browser that keeps dying,
// turning a loud failure into a slow leak, which is the opposite of the rule
// ADR-0005 D7 sets for this process, and (b) make "how many browsers has this
// profile had" depend on luck. Whether a dead session should be replaced
// automatically is a PRODUCT decision about degradation policy, and it is not
// one this package gets to make on its own.
//
// A caller that wants a fresh session after this constructs a new Holder, which
// is explicit and which core's ownership check can see.
var ErrSessionDied = errors.New("runtime: the held session's browser process is gone")

// ErrNoSession is returned when the Holder has not booted yet. It is separate
// from ErrSessionDied because "nothing has started" and "what started is gone"
// call for different reactions, and a caller told only "no pid" cannot tell
// which it is looking at.
var ErrNoSession = errors.New("runtime: no session has been started yet")

// Holder owns exactly one headless session and keeps it alive across commands.
//
// It is the module's first real consumer of core.StartSession, and that is the
// point of it rather than a side effect. Every defect this module found on
// 2026-08-18 was invisible until a caller existed that HELD a session instead
// of booting one, using it immediately and dropping it: the boot classified the
// SPA once because no fixture mounted slowly, and the session died with its
// boot context because every test passed context.Background(). See
// ARMADILHAS.md.
//
// Deliberately NOT here, this cycle: a capacity ceiling, a recycling policy, a
// pool of any kind. Holder holds ONE session. That boundary is not shyness —
// converting an unlimited resource into a limited one is the class of change
// that made a scenario strictly worse in this repository before (CLAUDE.md,
// regra 1: a limited resource obliges an inventory of everything that can hold
// it, and a session waiting ~16s for the SPA to mount is an obvious slot
// holder). When a ceiling is in scope, it gets its own measurement of the
// scenario where it CHARGES the price.
type Holder struct {
	cfg core.StartConfig

	mu      sync.Mutex
	session *core.Session
	stopped bool
}

// NewHolder prepares a Holder for cfg. It does not boot; the first Session
// call does.
//
// Booting lazily is what lets Session's context bound the BOOT while the
// Holder keeps the session — see the contract on Session.
func NewHolder(cfg core.StartConfig) *Holder { return &Holder{cfg: cfg} }

// Session returns the held session, booting it on first use.
//
// ctx bounds the BOOT ONLY. The returned session outlives it, and outlives the
// call: that is invariant 15 (HANDOFF §6), and a Holder is the first thing in
// this module able to exercise it, because it is the first thing that keeps a
// session past the call that created it.
//
// Concurrent first calls boot exactly one session. That is invariant 13 (one
// WhatsApp profile, one active session owner) enforced HERE rather than left to
// core.StartSession's ownership check: the check would refuse the second boot
// correctly, but the caller would see a spurious error for what is really one
// logical "give me the session" from two goroutines.
func (h *Holder) Session(ctx context.Context) (*core.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stopped {
		return nil, ErrHolderStopped
	}
	if h.session != nil {
		// Ask the cheapest, least ambiguous question before handing the
		// session out: is the process still there? A holder that skips this
		// returns a dead handle, and the caller discovers it as a CDP error in
		// the middle of a business operation rather than as an invalid
		// session (H21).
		//
		// This is NOT a health check, and must not grow into one here. Process
		// liveness is one of the seven distinct signals item 12 of the briefing
		// separates; a live process still says nothing about the socket, the
		// SPA or the identity. What it gives is a sound negative: process gone
		// means everything above it is gone too.
		if !h.session.ProcessAlive() {
			return nil, ErrSessionDied
		}
		return h.session, nil
	}

	// The lock is held across the boot on purpose. A boot takes seconds — it
	// waits for the SPA to mount — so this serialises concurrent first calls
	// for that whole time, which is the cost of the guarantee above. It is
	// acceptable precisely because Holder holds ONE session and has no
	// ceiling: nothing else is queued behind this lock competing for a slot,
	// so a slow boot delays only callers who are asking for THIS session and
	// have nothing to do until it exists.
	sess, err := core.StartSession(ctx, h.cfg)
	if err != nil {
		return nil, err
	}
	h.session = sess
	return sess, nil
}

// BrowserPID answers the product's getBrowserPid: the process id a supervisor
// should register and watch.
//
// IT REFUSES TO ANSWER WHEN THE NUMBER IS NO LONGER MEANINGFUL, and that
// refusal is the whole capability rather than an extra. Measured on 2026-08-19:
// engine.Browser.PID() keeps returning the SAME number after a clean stop, with
// the process already dead. The number is not wrong — it is what the browser
// used to be — but handing it to a supervisor is: operating systems reuse pids,
// so a stored pid signalled later can reach whatever process inherited the
// number. That is the same failure this module keeps meeting in other clothes —
// a value that cannot be told apart from a valid one.
//
// So the pid comes with the liveness check attached, and the three refusals are
// distinct: never started, already stopped, process gone.
func (h *Holder) BrowserPID() (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stopped {
		return 0, ErrHolderStopped
	}
	if h.session == nil {
		return 0, ErrNoSession
	}
	if !h.session.ProcessAlive() {
		// Deliberately NOT returning the stale pid alongside the error. A
		// caller that logs "pid=%d, err=%v" would put a reusable number into
		// the record next to a message nobody reads twice.
		return 0, ErrSessionDied
	}
	return h.session.Browser().PID(), nil
}

// Stop tears the held session down and releases the profile. It is safe to
// call more than once and from any goroutine; only the first call stops.
//
// A Holder that never booted returns engine.StopViaNoop and is still marked
// stopped, so a Stop that races the first Session call cannot leave a session
// running with nobody holding it.
func (h *Holder) Stop(ctx context.Context) engine.StopVia {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.stopped = true
	if h.session == nil {
		return engine.StopViaNoop
	}
	via := h.session.Stop(ctx)
	h.session = nil
	return via
}

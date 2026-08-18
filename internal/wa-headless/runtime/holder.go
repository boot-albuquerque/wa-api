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
// call: that is invariant 7 (HANDOFF §6), and a Holder is the first thing in
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

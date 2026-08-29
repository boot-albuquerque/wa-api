package runtime

import (
	"context"
	"sync"
	"time"

	"wa-api/internal/headless/capabilities/liveness"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/events"
)

// The composition point: the ONLY file that knows both how a session is born
// and who wants to hear about it.
//
// core declares a callback and knows nothing about the bus; events declares a
// second door and knows nothing about sessions. Both of those are deliberate
// (core/lifecycle.go says why), and the cost of the deliberation is that
// somebody has to hold the two ends. This is that somebody, and keeping it to
// one small file is what stops the lifecycle from spreading.

// StateWatchInterval is how often a StateWatcher asks. It is var so a test can
// compress the clock.
var StateWatchInterval = 2 * time.Second

// AttachHub makes this Holder publish its session's lifecycle onto hub.
//
// IT MUST BE CALLED BEFORE THE FIRST Session, and it says so by refusing
// afterwards: the observer is part of the StartConfig the boot reads, so a hub
// attached to an already-booted Holder would miss the one fact it most wants —
// that the session became ready — and would look attached while being deaf.
// Returning false is louder than a silent no-op, and quieter than a panic in a
// wiring path.
func (h *Holder) AttachHub(hub *events.Hub) bool {
	if hub == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || h.session != nil {
		return false
	}
	h.cfg.OnLifecycle = func(f core.LifecycleFact) {
		switch f.Phase {
		case core.PhaseReady:
			hub.PublishSessionState(events.SessionReady, string(f.Phase), readyReason(f.WasSuspect))
		case core.PhaseBootFailed:
			hub.PublishSessionStateWithClass(events.SessionBootFailed, string(f.Phase), f.Reason, f.PageClass)
		case core.PhaseStopped:
			hub.PublishSessionState(events.SessionStopped, string(f.Phase), f.Reason)
		}
	}
	return true
}

// readyReason distinguishes an ordinary ready from a RECOVERY.
//
// Both are "the session is usable", and a subscriber that treats them the same
// is right to. But a profile that carried a suspect marker and then verified
// clean is the discharge of invariant 2, and that is the kind of thing an
// operator wants counted rather than inferred from the absence of a complaint.
func readyReason(wasSuspect bool) string {
	if wasSuspect {
		return reasonRecovered
	}
	return reasonFresh
}

const (
	reasonFresh     = "fresh"
	reasonRecovered = "recovered_suspect"
)

// StateWatcher turns liveness VERDICTS into events, and only when they move.
//
// EVERY PROBE PRODUCES A VERDICT; ONLY A TRANSITION IS AN EVENT. A bus that
// repeats "still alive" every two seconds is a heartbeat wearing an event's
// clothes: it costs every subscriber a wake-up to learn nothing, and it buries
// the one message that mattered under a thousand that did not.
//
// IT OWNS ITS OWN GOROUTINE AND NOTHING ELSE'S. The repository's standing
// invariant is that nothing which waits on a clock may occupy a limited slot
// (CLAUDE.md, regra 3), and this waits on a clock by definition. So it holds no
// pool, no semaphore and no shared budget — one goroutine, ended by its own
// context.
type StateWatcher struct {
	checker  *liveness.Checker
	hub      *events.Hub
	interval time.Duration
	label    string

	mu   sync.Mutex
	last liveness.Signal
}

// NewStateWatcher builds one. It does not probe until Run.
func NewStateWatcher(checker *liveness.Checker, hub *events.Hub, label string) *StateWatcher {
	return &StateWatcher{checker: checker, hub: hub, interval: StateWatchInterval, label: label}
}

// Run watches until ctx ends.
//
// THE FIRST VERDICT IS AN EVENT. There is no previous state to differ from, and
// suppressing it would mean a subscriber that attaches to a session already in
// trouble hears nothing until it changes again — which, for a terminally dead
// process, is never.
func (w *StateWatcher) Run(ctx context.Context) error {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		r := w.checker.Check(ctx, w.label)
		w.mu.Lock()
		moved := r.Signal != w.last
		if moved {
			w.last = r.Signal
		}
		w.mu.Unlock()
		if moved {
			// OUTSIDE THE LOCK, for the same reason the Hub delivers outside
			// its own: a subscriber that asks this watcher what it last decided
			// is the first handler anybody writes.
			w.hub.PublishSessionState(events.SessionStateChanged, string(r.Signal), string(r.Class))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Last is the signal this watcher most recently published, for a test that
// needs to know what it decided without racing its subscriber.
func (w *StateWatcher) Last() liveness.Signal {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

package core

import "wa-api/internal/wa-headless/engine"

// The lifecycle port: how a session's own life leaves this package.
//
// WHY A PORT AND NOT A BUS. Everything else this module observes happens in
// the page, and the events package owns that. A session's birth and death do
// not happen in the page — the page cannot know that a boot verified a module
// inventory, nor that Go has decided to stop it. Those facts exist only here.
//
// The obvious move would be for core to publish them onto the bus directly.
// That would make the layer that OWNS the browser depend on the layer that
// merely reports it, and every future consumer of the bus would drag the
// session lifecycle in behind it. So core declares the narrowest possible
// callback, knows nothing about who listens, and the composition layer
// (runtime) is the single place that knows both sides.
//
// WHAT IT DELIBERATELY DOES NOT CARRY. No error, no profile path, no URL. A
// formatted error is where a path or a jid eventually leaks into the one place
// every capability reads, so the reason is a CLOSED vocabulary of this
// package's own constants: a BootStage or a StopVia, both already enumerated.
// A caller that needs the error has it — StartSession returned it.

// LifecyclePhase is a fact about a session's life that this package can
// actually distinguish.
//
// THE LIST IS SHORT ON PURPOSE. The upstream has nine session events and it
// would be easy to declare nine names here. AUTHENTICATED, CODE_RECEIVED and
// REMOTE_SESSION_SAVED have no observable in this build — pairing is a separate
// slice that does not exist, and there is no remote store at all — so naming
// them would ship three fields nothing can ever set.
type LifecyclePhase string

const (
	// PhaseReady is a boot that reached a VERIFIED ready: the page classified
	// APP_READY and the module inventory passed on that same boot.
	PhaseReady LifecyclePhase = "ready"
	// PhaseBootFailed is a boot that did not. Reason is the BootStage, which
	// is what says whether waiting, relaunching or a human is the repair.
	PhaseBootFailed LifecyclePhase = "boot_failed"
	// PhaseStopped is a session torn down through engine.CleanStop. Reason is
	// the StopVia, so a graceful close and a kill are not the same event.
	PhaseStopped LifecyclePhase = "stopped"
)

// LifecycleFact is one of those facts.
type LifecycleFact struct {
	Phase LifecyclePhase
	// Reason is a BootStage for PhaseBootFailed, a StopVia for PhaseStopped,
	// and empty for PhaseReady — there is only one way to be ready.
	Reason string
	// WasSuspect records that this profile carried a suspect marker when the
	// boot started. It is the one piece of history the phase alone cannot
	// convey, and it changes what a ready MEANS: a verified recovery rather
	// than an ordinary start.
	WasSuspect bool
}

// LifecycleObserver receives lifecycle facts.
//
// IT RUNS ON THE CALLER'S GOROUTINE, inside StartSession or Stop, which means
// an observer that blocks blocks the boot or the teardown. That is stated
// rather than defended against: the composition layer hands this to a Hub whose
// delivery contract already says the same thing, and adding a second queue here
// would put a session's death behind a buffer that the death itself drains.
type LifecycleObserver func(LifecycleFact)

// emit is the one place a fact leaves. A nil observer is the ordinary case.
func emit(obs LifecycleObserver, f LifecycleFact) {
	if obs == nil {
		return
	}
	obs(f)
}

// stopFact builds the fact for a completed stop.
func stopFact(via engine.StopVia) LifecycleFact {
	return LifecycleFact{Phase: PhaseStopped, Reason: string(via)}
}

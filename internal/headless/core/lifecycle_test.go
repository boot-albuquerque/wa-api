package core

import (
	"context"
	"sync"
	"testing"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// recorder collects lifecycle facts from whatever goroutine emits them.
type recorder struct {
	mu    sync.Mutex
	facts []LifecycleFact
}

func (r *recorder) observe(f LifecycleFact) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.facts = append(r.facts, f)
}

func (r *recorder) all() []LifecycleFact {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]LifecycleFact{}, r.facts...)
}

func (r *recorder) phases() []LifecyclePhase {
	out := []LifecyclePhase{}
	for _, f := range r.all() {
		out = append(out, f.Phase)
	}
	return out
}

// A boot that reaches ready and is then stopped emits exactly those two facts,
// in that order. The ORDER is the assertion that matters: a stop reported
// before the ready it ends would let a subscriber conclude the session is up
// after it has gone.
func TestLifecycle_ReadyThenStopped(t *testing.T) {
	rec := &recorder{}
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))
	cfg.OnLifecycle = rec.observe

	sess, err := StartSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if got := rec.phases(); len(got) != 1 || got[0] != PhaseReady {
		t.Fatalf("after a successful boot the facts are %v, want [%s]", got, PhaseReady)
	}

	via := sess.Stop(context.Background())
	got := rec.all()
	if len(got) != 2 {
		t.Fatalf("after Stop the facts are %v, want two", rec.phases())
	}
	if got[1].Phase != PhaseStopped {
		t.Fatalf("second fact is %s, want %s", got[1].Phase, PhaseStopped)
	}
	// The reason is the StopVia, not a formatted sentence: a graceful close and
	// a kill have to be distinguishable by a subscriber that never sees an error.
	if got[1].Reason != string(via) {
		t.Fatalf("stop reason %q, want the StopVia %q", got[1].Reason, string(via))
	}
	if got[0].WasSuspect {
		t.Error("a fresh profile was reported as suspect")
	}

	// Idempotent Stop does not emit a second death. A session dies once.
	sess.Stop(context.Background())
	if n := len(rec.all()); n != 2 {
		t.Fatalf("a second Stop produced %d facts, want the same 2: a session dies once", n)
	}
}

// A boot that fails reports the STAGE, which is what says whether the repair is
// waiting, relaunching, or a human with a phone. A subscriber told only "it
// failed" cannot tell a missing module from a QR screen.
func TestLifecycle_BootFailureCarriesTheStage(t *testing.T) {
	rec := &recorder{}
	blank := `<html><body>nothing here</body></html>`
	cfg := baseConfig(t, pageServer(t, "/blank", blank))
	cfg.SettleBudget = negativePathSettleBudget
	cfg.OnLifecycle = rec.observe

	if _, err := StartSession(context.Background(), cfg); err == nil {
		t.Fatal("a blank page must not reach ready")
	}
	got := rec.all()
	if len(got) != 1 || got[0].Phase != PhaseBootFailed {
		t.Fatalf("facts are %v, want one %s", rec.phases(), PhaseBootFailed)
	}
	if got[0].Reason != string(StageNotReady) {
		t.Fatalf("reason %q, want the stage %q", got[0].Reason, StageNotReady)
	}
}

// EVERY failure path reports, including the ones that fail before a browser
// exists. This is the property the wrapper in StartSession buys: a config
// error returns from a different line than a settle timeout, and a fact emitted
// at each return would be a fact somebody forgets to emit at one of them.
func TestLifecycle_EvenTheEarliestFailureReports(t *testing.T) {
	rec := &recorder{}
	// no-runner: this config never reaches a browser — it returns at the very
	// first validation, which is the whole property under test.
	cfg := StartConfig{OnLifecycle: rec.observe} // no BinaryPath: the first return.
	if _, err := StartSession(context.Background(), cfg); err == nil {
		t.Fatal("an empty config must not boot")
	}
	got := rec.all()
	if len(got) != 1 || got[0].Phase != PhaseBootFailed || got[0].Reason != string(StageConfig) {
		t.Fatalf("facts are %+v, want one %s/%s", got, PhaseBootFailed, StageConfig)
	}
}

// A ready that RECOVERED a suspect profile is not an ordinary ready, and the
// fact carries the difference. Without it, the discharge of invariant 2 — the
// thing that made this boot more expensive and more meaningful — is invisible
// to anybody watching the bus.
func TestLifecycle_ReadyRemembersTheProfileWasSuspect(t *testing.T) {
	rec := &recorder{}
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))
	cfg.OnLifecycle = rec.observe
	if err := engine.MarkSessionSuspect(cfg.ProfileDir, engine.StopViaDirtySignalExitTimeout); err != nil {
		t.Fatalf("priming the marker: %v", err)
	}
	sess, err := StartSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer sess.Stop(context.Background())

	got := rec.all()
	if len(got) != 1 || got[0].Phase != PhaseReady {
		t.Fatalf("facts are %v, want one %s", rec.phases(), PhaseReady)
	}
	if !got[0].WasSuspect {
		t.Fatal("the ready fact does not say the profile was suspect; a verified recovery " +
			"is indistinguishable from an ordinary start")
	}
}

// The observer runs OUTSIDE the session mutex. An observer that reacts to a
// death by tearing more down calls back into the session, and holding the lock
// across the callback would deadlock on the first handler anybody writes.
//
// The test IS the deadlock: without the manual unlock in Stop it never returns,
// and Go reports it as a timeout rather than a failed assertion. That is the
// loudest available signal for this particular defect.
func TestLifecycle_ObserverMayCallBackIntoTheSession(t *testing.T) {
	cfg := baseConfig(t, requirePage(t, spa.RequiredAtStartup))
	var sess *Session
	reentered := make(chan engine.StopVia, 1)
	cfg.OnLifecycle = func(f LifecycleFact) {
		if f.Phase != PhaseStopped {
			return
		}
		reentered <- sess.Stop(context.Background())
	}
	var err error
	sess, err = StartSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	sess.Stop(context.Background())
	if got := <-reentered; got != engine.StopViaNoop {
		t.Fatalf("the re-entrant Stop returned %s, want %s", got, engine.StopViaNoop)
	}
}

// NOTE: a TestLifecycle_NilObserverIsTheOrdinaryCase was removed here on
// 2026-08-21, and the removal is the point rather than a tidy-up.
//
// It booted a real browser to assert that a nil observer costs nothing — which
// EVERY OTHER TEST IN THIS PACKAGE already asserts, because every one of them
// leaves OnLifecycle nil and expects a clean boot. It bought no coverage and
// cost one more browser in a package that boots dozens under -race, and this
// package's gate was failing on exactly that contention.
//
// A test whose property is already carried by the rest of the suite is not free:
// it is paid for in wall clock on every run, and the bill arrives as a flake
// that looks like a defect.

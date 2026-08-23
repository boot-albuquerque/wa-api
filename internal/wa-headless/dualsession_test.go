package waheadless

import (
	"context"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// The dual-session harness: two accounts paired and awake AT THE SAME TIME.
//
// WHY IT HAD TO EXIST. Every earlier two-account test in this package runs the
// accounts in SEQUENCE — one Holder, one session, closed before the next opens
// (groupreqreal_test.go). That is enough when the question is "did the server
// end up in this state", and useless when the question is "what did the OTHER
// session see while this one acted".
//
// H86 measured that a participant change produces ZERO events in the session
// that MADE it, and H119 narrowed that: the gp2 carrier does reach this bus, so
// the zero is about the acting session, not about the bus. Deciding between
// "this build never emits participant events" and "it emits them to everyone
// except the actor" needs both sessions awake simultaneously. Nothing else can
// tell those apart.
//
// COST, STATED: two Chrome instances at once. On a busy machine that is the
// difference between a green gate and a load casualty, which is why every user
// of this harness carries its own budget and reports what it measured rather
// than asserting a wall clock.

// dualSession is two live sessions, each with its own browser and profile.
type dualSession struct {
	// A is the account that OBSERVES. It is named for the role, not the
	// account: which profile plays which side is the caller's choice, and
	// getting it backwards is the mistake this naming exists to prevent.
	A, B    *core.Session
	RunnerA *engine.Runner
	RunnerB *engine.Runner
	stopA   func()
	stopB   func()
}

// Close stops both, and stops the SECOND one even if the first panics.
func (d *dualSession) Close() {
	if d.stopB != nil {
		d.stopB()
	}
	if d.stopA != nil {
		d.stopA()
	}
}

// openDual boots both accounts and returns them awake.
//
// THE PORTS ARE DIFFERENT AND SO ARE THE PROFILES, which is the whole reason
// two browsers can coexist: a shared profile directory would have the second
// Chrome refuse to start, and a shared port would have it attach to the first.
// Both are taken from the caller rather than derived here, so a caller that
// passes the same twice fails loudly instead of silently driving one account.
func openDual(t *testing.T, profileA, profileB string, budget time.Duration) (*dualSession, context.Context, func()) {
	t.Helper()
	if profileA == "" || profileB == "" {
		t.Fatal("both profiles are required")
	}
	if profileA == profileB {
		t.Fatal("the two profiles are the same directory; that is one account, not two")
	}

	d := &dualSession{}
	ctx, cancel := context.WithTimeout(context.Background(), budget)

	boot := func(profile, side string) (*core.Session, *engine.Runner, func()) {
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		sess, err := h.Session(ctx)
		if err != nil {
			// STOP WHAT IS ALREADY UP before failing. A half-open dual session
			// leaks a browser that outlives the test binary, and this package has
			// paid for orphaned Chromes all session.
			d.Close()
			cancel()
			t.Fatalf("boot %s: %v", side, err)
		}
		return sess, runner, func() { _ = h.Stop(context.Background()) }
	}

	d.A, d.RunnerA, d.stopA = boot(profileA, "A/observer")
	d.B, d.RunnerB, d.stopB = boot(profileB, "B/actor")
	return d, ctx, func() { d.Close(); cancel() }
}

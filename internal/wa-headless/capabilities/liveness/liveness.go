// Package liveness answers the product's livenessCheck: is this session still
// usable?
//
// It is the first capability package with a production implementation, and it
// exists because spa.Monitor had no consumer. That is the same shape that hid
// two production defects on 2026-08-18 — a mechanism with no real caller is a
// mechanism nobody has been able to be wrong about yet (ARMADILHAS.md).
//
// WHAT IT ADDS OVER spa.Monitor, which already probes the page: it combines the
// PROCESS signal with the PAGE signal WITHOUT FUSING THEM. Item 12 of this
// initiative's briefing is explicit that process, target, service worker,
// socket, SPA, session and identity are different signals, and the phase 6
// finding this capability was named for is precisely the case they come apart
// in: a renderer that stops answering while everything outside it looks
// healthy. A checker that returned one boolean for both would be unable to
// express that case at all.
package liveness

import (
	"context"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Signal names WHY a check answered the way it did.
//
// Separate values rather than a bare bool because the caller's correct reaction
// differs per signal: a gone process is unrecoverable and the session must be
// replaced, an unresponsive page may be a loaded host worth one more probe, and
// an absent application is a session that is running but logged out or showing
// a QR — three different repairs behind one word if they were merged.
type Signal string

const (
	// SignalAlive: the process is running and the application is mounted.
	SignalAlive Signal = "ALIVE"
	// SignalProcessGone: the browser process is not there. Nothing above it
	// can be true, and no page probe was attempted.
	SignalProcessGone Signal = "PROCESS_GONE"
	// SignalPageUnresponsive: the process is running but the page has failed
	// to answer for the monitor's consecutive-failure threshold.
	SignalPageUnresponsive Signal = "PAGE_UNRESPONSIVE"
	// SignalPageSlow: the page failed this probe but is still under the
	// threshold. Not alive, not yet a verdict.
	SignalPageSlow Signal = "PAGE_SLOW"
	// SignalAppAbsent: the page answered, so it is executing, but the
	// application is not mounted — a QR screen, a redirect, a crashed app.
	SignalAppAbsent Signal = "APP_ABSENT"
)

// Report is one check.
type Report struct {
	// Alive is the answer the product's livenessCheck returns.
	Alive bool
	// Signal is why. Never absent: every path sets it.
	Signal Signal
	// Class is the page classification, when a page probe ran. It is the zero
	// value when the process was already gone — and that is not a claim that
	// the page was fine, it is the absence of a measurement.
	Class spa.PageClass
	// PageProbed records whether the page was asked at all, so a zero Class and
	// a zero Latency cannot be read as "the page answered instantly".
	PageProbed bool
	// Latency is the page round trip. Zero when PageProbed is false.
	Latency time.Duration
	// ConsecutiveFailures is the monitor's streak, including this probe.
	ConsecutiveFailures int
	// Err is the probe error, if any.
	Err error
}

// Checker answers livenessCheck for one session.
//
// It takes what it needs as functions rather than a *core.Session so that it
// can be exercised with a double, which the capability template in
// capabilities/send/doc.go requires. It also keeps the dependency arrow
// pointing away from the session lifecycle: a capability should not need to
// know how a session is born to ask whether it is alive.
type Checker struct {
	processAlive func() bool
	monitor      *spa.Monitor
}

// New builds a Checker. processAlive is the cheap, page-independent signal —
// core.Session.ProcessAlive satisfies it.
func New(processAlive func() bool, runner *engine.Runner, eval spa.Evaluator) *Checker {
	return &Checker{
		processAlive: processAlive,
		monitor:      &spa.Monitor{Runner: runner, Eval: eval},
	}
}

// NOTE: an earlier NewWithMonitor was removed here on 2026-08-20. It existed
// "so a caller that already keeps failure history does not start a second
// streak", and no such caller was ever written. An exported constructor with no
// user is a promise nobody asked for, and the capability template in
// capabilities/send/doc.go says not to invent abstraction ceremony — this was
// mine. It comes back when a caller needs it, with a test.

// Check runs one liveness check.
//
// THE PROCESS IS ASKED FIRST, and the order is load-bearing rather than
// stylistic. The process question needs no cooperation from the page and costs
// nothing; the page question costs a full StateProbe budget. On a dead process
// the page probe cannot succeed, so asking it would spend the budget to learn
// something already known — and would then report PAGE_UNRESPONSIVE, naming the
// consequence instead of the cause. This is the same reason the module records
// "read failed" separately from "the page said X".
func (c *Checker) Check(ctx context.Context, label string) Report {
	if c.processAlive != nil && !c.processAlive() {
		return Report{Alive: false, Signal: SignalProcessGone}
	}

	res := c.monitor.Check(ctx, label)
	out := Report{
		Alive:               res.Alive,
		Class:               res.Class,
		PageProbed:          true,
		Latency:             res.Latency,
		ConsecutiveFailures: res.ConsecutiveFailures,
		Err:                 res.Err,
	}
	switch {
	case res.Alive:
		out.Signal = SignalAlive
	case res.Unresponsive():
		out.Signal = SignalPageUnresponsive
	case res.Err != nil:
		out.Signal = SignalPageSlow
	default:
		// The page answered and said the application is not mounted.
		out.Signal = SignalAppAbsent
	}
	return out
}

// Stats forwards the monitor's counters. Counts and durations only.
func (c *Checker) Stats() (probes, failures int, lastLatency time.Duration, consecutive int) {
	return c.monitor.Stats()
}

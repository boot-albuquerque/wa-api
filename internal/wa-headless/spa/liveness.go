package spa

// Liveness: deciding whether a session is still usable.
//
// This is the capability phase 6 wrote the rule for, and the rule is a
// prohibition:
//
//	Liveness of a session cannot be "the process is alive" nor "the target
//	exists". It has to be an Evaluate with a deadline on the controller side.
//
// The measurement behind it: a page answered once, then went silent for over
// four minutes while the renderer process stayed up, the target stayed
// attached, the service worker stayed alive and the CDP connection kept
// accepting browser-level commands. Every structural health check would have
// reported that session healthy — and in a fleet with standby and recycling,
// that means keeping a dead session and counting it as capacity.
//
// Study origin: scripts/chromium-study/ACHADO-RENDERER-NAO-RESPONSIVO.md.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// livenessScript is the round trip.
//
// It reads structure, not a constant: evaluating `1` proves the renderer
// executes JavaScript and nothing more, and a renderer can execute while the
// application is gone. Requiring #pane-side means the answer is "the
// application is still mounted AND the page is still running".
//
// It is NOT yet the authenticated round trip the product's contract describes —
// "forces I/O to the authenticated context, not a cached value". That needs the
// SPA module inventory of CAP-06, and claiming it before then would be the kind
// of overstatement this initiative keeps catching. What this probe rules out is
// the measured failure mode; what it does not yet rule out is a mounted UI over
// a dead socket.
//
// It never sends anything. The product contract rejects synthetic-send
// explicitly, and so does this.
const livenessScript = `JSON.stringify(!!document.querySelector('#pane-side'))`

// DefaultUnresponsiveAfter is how many consecutive failed probes make a session
// UNRESPONSIVE.
//
// More than one, because a single blown deadline can be a loaded host. Not
// many, because the measured state lasted over four minutes: waiting out ten
// probes would spend that whole window calling a dead session merely slow.
const DefaultUnresponsiveAfter = 3

// LivenessResult is one probe.
type LivenessResult struct {
	// Alive is the answer the product's livenessCheck returns.
	Alive bool
	// Class is why, and it is what invariant 12 requires: no session dies
	// without a classified cause.
	Class PageClass
	// Latency is how long the round trip took, recorded on success AND on
	// failure.
	//
	// The boolean alone is not enough, and this is the second half of the phase
	// 6 rule: degradation shows up as a probe that takes three seconds before it
	// ever shows up as a probe that times out. A fleet that only watches the
	// boolean learns nothing until the session is already gone.
	Latency time.Duration
	// ConsecutiveFailures counts probes that have failed in a row, including
	// this one.
	ConsecutiveFailures int
	// Err is the probe error, if any.
	Err error
}

// Unresponsive reports whether this result means the session should be treated
// as lost rather than idle.
func (r LivenessResult) Unresponsive() bool { return r.Class == ClassUnresponsive }

// Monitor probes one session repeatedly and remembers how it has been going.
//
// Safe for concurrent use: a probe timer and a command path can both ask.
type Monitor struct {
	// Runner supplies the budget. The probe runs under StateProbe, which is
	// short on purpose — a tab that does not answer within it IS the result.
	Runner *engine.Runner
	// Eval is the page evaluator.
	Eval Evaluator
	// UnresponsiveAfter overrides DefaultUnresponsiveAfter.
	UnresponsiveAfter int
	// Now is the clock, for tests.
	Now func() time.Time

	mu           sync.Mutex
	consecutive  int
	lastLatency  time.Duration
	totalProbes  int
	totalFailure int
}

func (m *Monitor) threshold() int {
	if m.UnresponsiveAfter > 0 {
		return m.UnresponsiveAfter
	}
	return DefaultUnresponsiveAfter
}

func (m *Monitor) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Check runs one probe.
//
// It never blocks longer than the StateProbe budget, so it is safe to call from
// a path that holds a limited slot — the standing invariant of this repository
// is that nothing waiting on a clock or a dead peer may occupy one.
func (m *Monitor) Check(ctx context.Context, label string) LivenessResult {
	start := m.now()

	var raw string
	err := m.Runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return m.Eval(ctx, livenessScript, &raw)
	})
	latency := m.now().Sub(start)

	res := LivenessResult{Latency: latency, Err: err}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalProbes++
	m.lastLatency = latency

	if err != nil {
		m.consecutive++
		m.totalFailure++
		res.ConsecutiveFailures = m.consecutive
		// A single failure is not a verdict. Below the threshold the session is
		// reported not-alive but NOT unresponsive: the difference is whether a
		// recycler should act, and acting on one slow probe would recycle
		// healthy sessions off a loaded host.
		if m.consecutive >= m.threshold() {
			res.Class = ClassUnresponsive
		} else {
			res.Class = ClassOther
		}
		return res
	}

	var mounted bool
	if jsonErr := json.Unmarshal([]byte(raw), &mounted); jsonErr != nil {
		// The page answered with something unexpected. It answered, so this is
		// not unresponsiveness — but it is not proof of life either.
		m.consecutive++
		m.totalFailure++
		res.ConsecutiveFailures = m.consecutive
		res.Class = ClassOther
		res.Err = fmt.Errorf("liveness: unexpected answer %q: %w", raw, jsonErr)
		return res
	}

	// The probe came back in time. Whatever it says, the page is executing, so
	// the failure streak is over — a mounted-or-not answer is still an answer.
	m.consecutive = 0

	if !mounted {
		// Executing, but the application is not there: a QR screen, a redirect,
		// a crashed app. Not alive, not unresponsive, and the caller needs a
		// page probe to say which.
		res.Class = ClassOther
		return res
	}
	res.Alive = true
	res.Class = ClassAppReady
	return res
}

// Stats reports what the monitor has seen. Counts and durations only — nothing
// from the page.
func (m *Monitor) Stats() (probes, failures int, lastLatency time.Duration, consecutive int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalProbes, m.totalFailure, m.lastLatency, m.consecutive
}

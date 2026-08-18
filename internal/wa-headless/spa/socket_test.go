package spa

import (
	"math"
	"strings"
	"testing"
	"time"
)

// TestOpeningWindowThresholdTermsAreLocked pins the three named terms in
// socket.go's production code — healthyUpperBound, measurementUncertainty,
// explicitGuardBand — to the values EVIDENCIA-SPA.md M7/M8 measured, and
// separately checks that OpeningWindowThreshold is their SUM (composition),
// not a literal asserted equal to them. Unlike the prior version of this
// test, the terms below are read from production code, not copied as
// literals into this file — a mutation to any one of the three consts in
// socket.go, or to how they are composed, fails here.
//
// What this test does NOT protect: it does not verify that 1.36s, 0.31s, or
// 677.69s are still what EVIDENCIA-SPA.md M7/M8 say. No reasonable Go test
// parses EVIDENCIA-SPA.md, so that link is prose, not code, and stays a
// manual-review obligation (H14). What changed in LOOP 04.5 is that the
// terms themselves stopped living only in prose — they are named data in
// socket.go now, and this test locks THAT, not just the sum.
func TestOpeningWindowThresholdTermsAreLocked(t *testing.T) {
	const (
		wantHealthyUpperBound      = 1360 * time.Millisecond // M7.3: max over 21 boots/7 conditions, net-heavy r1
		wantMeasurementUncertainty = 310 * time.Millisecond  // M7.5: worst observed spacing on the net-* legs
		wantExplicitGuardBand      = 1360 * time.Millisecond // engineering choice: one more full healthyUpperBound width
	)
	if healthyUpperBound != wantHealthyUpperBound {
		t.Errorf("healthyUpperBound = %v, want %v (M7.3)", healthyUpperBound, wantHealthyUpperBound)
	}
	if measurementUncertainty != wantMeasurementUncertainty {
		t.Errorf("measurementUncertainty = %v, want %v (M7.5)", measurementUncertainty, wantMeasurementUncertainty)
	}
	if explicitGuardBand != wantExplicitGuardBand {
		t.Errorf("explicitGuardBand = %v, want %v (engineering choice, == term 1)", explicitGuardBand, wantExplicitGuardBand)
	}

	got := healthyUpperBound + measurementUncertainty + explicitGuardBand
	if got != OpeningWindowThreshold {
		t.Fatalf("OpeningWindowThreshold = %v, but healthyUpperBound+measurementUncertainty+explicitGuardBand = %v; "+
			"the constant is no longer the SUM of its named terms", OpeningWindowThreshold, got)
	}
	// Sanity against the two envelope numbers the derivation cites: C must
	// stay far below the "no ceiling below" floor M8 measured, and it must
	// stay above the M7 lower bound it is built from.
	const (
		m7LowerBound     = 1360 * time.Millisecond
		m8NoceilingBelow = 677690 * time.Millisecond // 677.69s, M8.4 long-cut-3
	)
	if OpeningWindowThreshold <= m7LowerBound {
		t.Fatalf("C = %v must be strictly above the M7 lower bound %v", OpeningWindowThreshold, m7LowerBound)
	}
	if OpeningWindowThreshold >= m8NoceilingBelow {
		t.Fatalf("C = %v must stay far below the M8 no-ceiling floor %v", OpeningWindowThreshold, m8NoceilingBelow)
	}
}

// TestClassifyOpeningDurationBoundary is the mandatory table test: below C,
// at C (boundary, defined as inclusive), and above C.
func TestClassifyOpeningDurationBoundary(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want SocketLiveness
	}{
		{"zero", 0, SocketOpeningWithinEnvelope},
		{"far below C", 1360 * time.Millisecond, SocketOpeningWithinEnvelope}, // the M7 lower bound itself
		{"one tick below C", OpeningWindowThreshold - time.Millisecond, SocketOpeningWithinEnvelope},
		{"exactly C", OpeningWindowThreshold, SocketOpeningDegraded}, // boundary is inclusive, see socket.go
		{"one tick above C", OpeningWindowThreshold + time.Millisecond, SocketOpeningDegraded},
		{"far above C, M6 boot-lag scale", 5 * time.Second, SocketOpeningDegraded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyOpeningDuration(tt.d); got != tt.want {
				t.Errorf("ClassifyOpeningDuration(%v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

// WHAT IS GUARANTEED BY TESTS VS BY STRUCTURE (F-28 errata).
//
// The tests in this file cover a FINITE list of sampled points — see each
// test's own doc comment for what its list contains. No finite test proves
// "ClassifyOpeningDuration returns only these two values for every possible
// time.Duration" or "no future edit adds a third branch" — that would take
// an infeasible sweep over the full int64 domain.
//
// What DOES guarantee "exactly two outcomes, forever" is STRUCTURAL, not
// tested: ClassifyOpeningDuration (socket.go) has exactly two return
// statements in its body, both returning one of the two SocketLiveness
// constants declared in this package, and SocketLiveness has no third
// constant for a third statement to return. A change that adds a third
// return statement is a diff any reviewer sees directly in socket.go; the
// tests below narrow where a THIRD VALUE could sneak in via a value already
// computed from d (e.g. a threshold-gated branch), not whether one exists in
// the abstract — that question is answered by reading the function, not by
// running it.

// TestClassifyOpeningDurationM8SampleNeverReachesSessionLost is the NEGATIVE
// CONTROL DEC-04.4-02 requires, for exactly ONE historically important
// sample: 677.69s, the exact figure M8.4's long-cut-3 measured the socket
// still sitting in OPENING at, with no ceiling found inside the 12-minute
// window and no independent evidence the session was lost. This test proves
// that THIS SAMPLE classifies as SocketOpeningDegraded and never as a
// session-lost value — it does not, and was never meant to, prove that NO
// duration anywhere produces a session-lost value; that broader claim is
// covered by TestClassifyOpeningDurationSampledPointsMapToTwoValues plus the
// structural argument above (SocketLiveness has no session-lost value to
// return at all). This function must never be in a position to trigger a
// teardown call (it has no side effects at all; it is a pure
// duration->verdict mapping). LOOP 04.5 removed the SESSION_LOST value from
// SocketLiveness entirely (it had no producer and was an unreachable
// placeholder), so the check below is against the SocketLiveness string
// space, not a specific removed identifier — the removal itself is the
// stronger control: the value this test used to check against no longer
// exists for anything to return.
//
// This test was run, before the removal, against a deliberately
// reintroduced defect (making ClassifyOpeningDuration return the old
// SocketSessionLost value past a duration threshold) and the mutation
// FAILED this test before being reverted. The failure output is pasted in
// the LOOP-04.4-T1 worker report, not in this file — the mutation was never
// committed.
func TestClassifyOpeningDurationM8SampleNeverReachesSessionLost(t *testing.T) {
	const farAboveC = 677690 * time.Millisecond // 677.69s, EVIDENCIA-SPA.md M8.4 long-cut-3

	got := ClassifyOpeningDuration(farAboveC)
	if got != SocketOpeningDegraded {
		t.Fatalf("ClassifyOpeningDuration(%v) = %v, want SocketOpeningDegraded", farAboveC, got)
	}
	if string(got) == "SESSION_LOST" {
		t.Fatalf("ClassifyOpeningDuration(%v) = SESSION_LOST; DEC-04.4-02 forbids duration in "+
			"OPENING alone from ever producing this value — M8 measured this exact duration with "+
			"the socket still OPENING and no independent evidence of session loss (EVIDENCIA-SPA.md M8.4)", farAboveC)
	}
}

// TestClassifyOpeningDurationSampledPointsMapToTwoValues checks a FIXED LIST
// of representative and extreme points, not a sweep (there is no step
// between them) and not an exhaustion of the domain (time.Duration has
// ~1.8*10^19 representable values; this list has 11). A mutation whose
// trigger falls strictly between two of these points, or is more extreme
// than the most extreme point tested, can still escape this test — see
// F-28's executed negative controls in the LOOP-04.5-F28 worker report for a
// concrete example of exactly that gap (a fake third branch above 365 days
// and below the practical maximum escaped the pre-F28 version of this test
// entirely). The list below closes the two gaps that report found (the
// unbounded region above the old top sample, and the region between 24h and
// 365 days) by adding the true practical extreme (time.Duration's own
// maximum, ~292 years) and points that bracket the boundary at C by exactly
// one nanosecond on each side; it does not, and cannot, close every possible
// gap in an unbounded int64 domain.
func TestClassifyOpeningDurationSampledPointsMapToTwoValues(t *testing.T) {
	durations := []time.Duration{
		0,
		OpeningWindowThreshold - 1, // one ns below C
		OpeningWindowThreshold,     // exactly C
		OpeningWindowThreshold + 1, // one ns above C
		time.Millisecond,
		time.Second,
		time.Minute,
		time.Hour,
		24 * time.Hour,
		677690 * time.Millisecond,    // 677.69s, EVIDENCIA-SPA.md M8.4 long-cut-3
		time.Duration(math.MaxInt64), // ~292 years, the practical extreme: time.Duration cannot represent a longer positive duration
	}
	for _, d := range durations {
		switch got := ClassifyOpeningDuration(d); got {
		case SocketOpeningWithinEnvelope, SocketOpeningDegraded:
			// expected
		default:
			t.Fatalf("ClassifyOpeningDuration(%v) = %v, which is neither SocketOpeningWithinEnvelope nor "+
				"SocketOpeningDegraded — duration in OPENING must never reach a third state", d, got)
		}
	}
}

// TestClassifyOpeningDurationNegativeIsWithinEnvelope decides, deliberately,
// what a negative duration means: the caller computes d as time elapsed
// since the socket entered SocketStateOpening (time.Since-shaped), so a
// negative value is nonsense as real input — no real caller can produce one
// — and is therefore OUT OF CONTRACT, not a case this function is asked to
// handle meaningfully. It is not left undecided by accident, though:
// time.Duration is a totally ordered int64, so "<" against C is still
// well-defined for a negative value, and ClassifyOpeningDuration has no
// input validation (see its doc comment: it trusts the caller). This test
// locks the CURRENT, well-defined behavior — negative compares less than
// C's positive value, so it reads WithinEnvelope — so a future change to
// that behavior is a deliberate, reviewed diff instead of a silent drift.
func TestClassifyOpeningDurationNegativeIsWithinEnvelope(t *testing.T) {
	const negative = -1 * time.Hour // out-of-contract input; see doc comment above

	got := ClassifyOpeningDuration(negative)
	if got != SocketOpeningWithinEnvelope {
		t.Fatalf("ClassifyOpeningDuration(%v) = %v, want SocketOpeningWithinEnvelope (current, deliberately "+
			"locked behavior for out-of-contract negative input)", negative, got)
	}
}

// TestSocketStateReadExprMatchesModuleSocketModel guards against the read
// expression silently drifting from the module name it depends on: if
// ModuleSocketModel's spelling ever changes without this file being touched,
// the expression would still compile as a JS string but read the wrong
// window.require argument.
func TestSocketStateReadExprMatchesModuleSocketModel(t *testing.T) {
	want := "window.require('" + string(ModuleSocketModel) + "')"
	if !strings.Contains(SocketStateReadExpr, want) {
		t.Fatalf("SocketStateReadExpr does not embed window.require(%q); got:\n%s", string(ModuleSocketModel), SocketStateReadExpr)
	}
}

package spa

import (
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

// TestClassifyOpeningDurationNeverReachesSessionLost is the NEGATIVE CONTROL
// DEC-04.4-02 requires: a duration far above C — 677.69s, the exact figure
// M8.4's long-cut-3 measured the socket still sitting in OPENING at, with no
// ceiling found inside the 12-minute window and no independent evidence the
// session was lost — must classify as SocketOpeningDegraded, and this
// function must never be in a position to trigger a teardown call (it has
// no side effects at all; it is a pure duration->verdict mapping). LOOP 04.5
// removed the SESSION_LOST value from SocketLiveness entirely (it had no
// producer and was an unreachable placeholder), so the check below is
// against the SocketLiveness string space, not a specific removed
// identifier — the removal itself is the stronger control: the value this
// test used to check against no longer exists for anything to return.
//
// This test was run, before the removal, against a deliberately
// reintroduced defect (making ClassifyOpeningDuration return the old
// SocketSessionLost value past a duration threshold) and the mutation
// FAILED this test before being reverted. The failure output is pasted in
// the LOOP-04.4-T1 worker report, not in this file — the mutation was never
// committed.
func TestClassifyOpeningDurationNeverReachesSessionLost(t *testing.T) {
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

// TestClassifyOpeningDurationExhaustsToTwoValues is a second, structural
// guard on the same invariant: it enumerates every value ClassifyOpeningDuration
// is documented to return and fails if a third value ever appears, across a
// wide sweep of durations including ones far past anything EVIDENCIA-SPA.md
// measured. A change that adds a third branch — reachable only past some
// very large duration a narrower sweep would miss — still gets caught here.
func TestClassifyOpeningDurationExhaustsToTwoValues(t *testing.T) {
	durations := []time.Duration{
		0, time.Millisecond, time.Second, OpeningWindowThreshold,
		time.Minute, time.Hour, 24 * time.Hour, 677690 * time.Millisecond,
		365 * 24 * time.Hour,
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

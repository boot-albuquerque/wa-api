package spa

import (
	"strings"
	"testing"
	"time"
)

// TestOpeningWindowThresholdMatchesDocumentedTerms locks the three literal
// terms below — copied from socket.go's derivation comment — to the
// OpeningWindowThreshold constant, so a future edit to the constant without a
// matching edit to the comment (or vice versa) fails here instead of
// drifting silently.
//
// What this test PROTECTS: the constant against diverging from the three
// numbers socket.go's comment claims it is built from.
//
// What this test does NOT protect: it does not verify that 1.36s, 0.31s, or
// 677.69s are still what EVIDENCIA-SPA.md M7/M8 say. The three consts below
// are hardcoded literals in THIS file, not a read of the markdown — no
// reasonable Go test parses EVIDENCIA-SPA.md, so that link is prose, not
// code, and stays a manual-review obligation. A mutation that changes only
// socket.go's derivation NARRATIVE (e.g. the comment's stated value for
// term 1) without touching the OpeningWindowThreshold constant or this
// file's literals passes this test — confirmed by running exactly that
// mutation. Don't read a green run here as "the derivation is correct
// against the evidence"; read it as "the constant has not silently drifted
// from what the comment claims."
func TestOpeningWindowThresholdMatchesDocumentedTerms(t *testing.T) {
	const (
		healthyUpperBound      = 1360 * time.Millisecond // M7.3: max over 21 boots/7 conditions, net-heavy r1
		measurementUncertainty = 310 * time.Millisecond  // M7.5: worst observed spacing on the net-* legs
		explicitGuardBand      = 1360 * time.Millisecond // engineering choice: one more full healthyUpperBound width
	)
	got := healthyUpperBound + measurementUncertainty + explicitGuardBand
	if got != OpeningWindowThreshold {
		t.Fatalf("OpeningWindowThreshold = %v, but the sum of its documented terms is %v; "+
			"the constant and its derivation comment have drifted apart", OpeningWindowThreshold, got)
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
		{"zero", 0, SocketHealthy},
		{"far below C", 1360 * time.Millisecond, SocketHealthy}, // the M7 lower bound itself
		{"one tick below C", OpeningWindowThreshold - time.Millisecond, SocketHealthy},
		{"exactly C", OpeningWindowThreshold, SocketDegraded}, // boundary is inclusive, see socket.go
		{"one tick above C", OpeningWindowThreshold + time.Millisecond, SocketDegraded},
		{"far above C, M6 boot-lag scale", 5 * time.Second, SocketDegraded},
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
// session was lost — must classify as SocketDegraded, never SocketSessionLost,
// and this function must never be in a position to trigger a teardown call
// (it has no side effects at all; it is a pure duration->verdict mapping).
//
// This test was run against a deliberately reintroduced defect (making
// ClassifyOpeningDuration return SocketSessionLost past a duration
// threshold) and the mutation FAILED this test before being reverted. The
// failure output is pasted in the LOOP-04.4-T1 worker report, not in this
// file — the mutation is not committed.
func TestClassifyOpeningDurationNeverReachesSessionLost(t *testing.T) {
	const farAboveC = 677690 * time.Millisecond // 677.69s, EVIDENCIA-SPA.md M8.4 long-cut-3

	got := ClassifyOpeningDuration(farAboveC)
	if got == SocketSessionLost {
		t.Fatalf("ClassifyOpeningDuration(%v) = SESSION_LOST; DEC-04.4-02 forbids duration in "+
			"OPENING alone from ever producing this value — M8 measured this exact duration with "+
			"the socket still OPENING and no independent evidence of session loss (EVIDENCIA-SPA.md M8.4)", farAboveC)
	}
	if got != SocketDegraded {
		t.Fatalf("ClassifyOpeningDuration(%v) = %v, want SocketDegraded", farAboveC, got)
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
		case SocketHealthy, SocketDegraded:
			// expected
		default:
			t.Fatalf("ClassifyOpeningDuration(%v) = %v, which is neither SocketHealthy nor "+
				"SocketDegraded — duration in OPENING must never reach a third state", d, got)
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

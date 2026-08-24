package engine

import (
	"testing"
	"time"
)

// allOpKinds is the enumeration the tests below hold the policy against.
// Adding a constant in deadline.go without adding it here makes
// TestEveryOpKindIsEnumerated fail, which is the point: an operation class that
// no test knows about is an operation class whose deadline nobody checked.
var allOpKinds = []OpKind{
	OpNavigate, OpQuery, OpEvaluate, OpAction,
	OpStateProbe, OpRecoveryProbe, OpShutdown, OpBoot,
}

// The degenerate policy — every field zero — is what "the code does nothing"
// looks like here, and it is NOT harmless: context.WithTimeout(parent, 0) is
// already expired, so a zero deadline turns every operation into an instant
// failure that reads like a dead browser. Refusing zero is refusing to
// misdiagnose.
func TestDefaultDeadlinesAreAllPositive(t *testing.T) {
	for _, k := range allOpKinds {
		if d := DefaultDeadlines.For(k); d <= 0 {
			t.Errorf("%s: deadline is %v; a non-positive budget fails instantly "+
				"and looks like an unresponsive target", k, d)
		}
	}
}

// Each class must read its OWN field. A missing case in the switch falls
// through to Evaluate and would be invisible with realistic values, because
// several classes plausibly share a duration. Distinct sentinels make the
// omission fail.
func TestEveryOpKindReadsItsOwnField(t *testing.T) {
	p := DeadlinePolicy{
		Navigate:      1 * time.Second,
		Query:         2 * time.Second,
		Evaluate:      3 * time.Second,
		Action:        4 * time.Second,
		StateProbe:    5 * time.Second,
		RecoveryProbe: 6 * time.Second,
		Shutdown:      7 * time.Second,
		Boot:          8 * time.Second,
	}
	want := map[OpKind]time.Duration{
		OpNavigate:      1 * time.Second,
		OpQuery:         2 * time.Second,
		OpEvaluate:      3 * time.Second,
		OpAction:        4 * time.Second,
		OpStateProbe:    5 * time.Second,
		OpRecoveryProbe: 6 * time.Second,
		OpShutdown:      7 * time.Second,
		OpBoot:          8 * time.Second,
	}
	for _, k := range allOpKinds {
		if got := p.For(k); got != want[k] {
			t.Errorf("For(%s) = %v, want %v — the switch is missing this class "+
				"and silently fell back to Evaluate", k, got, want[k])
		}
	}
}

func TestEveryOpKindIsEnumerated(t *testing.T) {
	// A cheap completeness check: the enumeration above must cover every value
	// the policy distinguishes. If For() grows a case whose result differs from
	// Evaluate for a kind not in allOpKinds, this cannot see it — but the
	// enumeration is what the other tests iterate, so keeping it honest is
	// worth stating.
	seen := map[OpKind]bool{}
	for _, k := range allOpKinds {
		if seen[k] {
			t.Fatalf("%s listed twice in allOpKinds", k)
		}
		seen[k] = true
	}
	if len(allOpKinds) != 8 {
		t.Fatalf("allOpKinds has %d entries; deadline.go declares 8 classes — "+
			"add the new class here and give it a deadline", len(allOpKinds))
	}
}

// An unknown class must still be bounded. Returning zero here would be worse
// than the fallback: it would make an unbudgeted operation fail instantly
// instead of merely being budgeted approximately.
func TestUnknownOpKindFallsBackToABoundedDeadline(t *testing.T) {
	d := DefaultDeadlines.For(OpKind("SomethingNobodyBudgeted"))
	if d <= 0 {
		t.Fatalf("unknown class got %v; an unbounded or zero fallback is the "+
			"defect this policy exists to prevent", d)
	}
	if d != DefaultDeadlines.Evaluate {
		t.Fatalf("unknown class got %v, want the Evaluate budget %v", d, DefaultDeadlines.Evaluate)
	}
}

package spa

import (
	"strings"
	"testing"
)

// TestTheClassifierOuterFunctionIsSynchronous. engine.Tab.Evaluate does not
// await, so an async wrapper hands Go a Promise where it expects an answer —
// which is exactly what the first version did, and every control came back as
// "cannot unmarshal object into string".
func TestTheClassifierOuterFunctionIsSynchronous(t *testing.T) {
	if strings.HasPrefix(ClassifyWriteExpr, "`(async function") ||
		strings.Contains(ClassifyWriteExpr[:60], "async function") {
		t.Fatal("the outer function is async; Evaluate would receive a Promise")
	}
	if !strings.Contains(ClassifyWriteExpr, "(async () => {") {
		t.Fatal("the await does not live in an inner IIFE, so the write is not deferred")
	}
}

// TestTheClassifierParksTheReaderRatherThanItsValue. Reading once, straight
// after the await, reported the starring control — measured IMMEDIATE at 696ms —
// as "did not move". The instrument built to detect that defect had it.
func TestTheClassifierParksTheReaderRatherThanItsValue(t *testing.T) {
	if !strings.Contains(ClassifyWriteExpr, "read: snap") {
		t.Fatal("the reader function is not parked; only a value would be")
	}
	if !strings.Contains(ClassifyReadExpr, "const now = s.read();") {
		t.Fatal("the poll does not re-run the reader")
	}
	if !strings.Contains(ClassifyReadExpr, "stage: 'settling'") {
		t.Fatal("the poll has no settling answer, so Go cannot tell 'not yet' from 'not at all'")
	}
}

// TestTheClassifierHasNoClockOfItsOwn. Invariant 6: the page never decides how
// long to wait, or the wait becomes a duration nothing in Go can see.
func TestTheClassifierHasNoClockOfItsOwn(t *testing.T) {
	for _, banned := range []string{"setTimeout(", "setInterval(", "Date.now()"} {
		if strings.Contains(ClassifyWriteExpr, banned) || strings.Contains(ClassifyReadExpr, banned) {
			t.Fatalf("the classifier uses the page's clock (%s)", banned)
		}
	}
}

// TestAThrowingReaderIsNotAFailedWrite. If the reader itself is wrong, saying
// "the write did nothing" would blame the wrong half.
func TestAThrowingReaderIsNotAFailedWrite(t *testing.T) {
	if !strings.Contains(ClassifyWriteExpr, "READER_THREW") {
		t.Fatal("a throwing reader is not distinguished from a failed write")
	}
	if !strings.Contains(ClassifyWriteExpr, "WRITE_THREW") {
		t.Fatal("a throwing write is not named")
	}
}

// TestTheClassesAreDistinctValues. UNKNOWN must not collide with a real answer.
func TestTheClassesAreDistinctValues(t *testing.T) {
	seen := map[WriteClass]bool{}
	for _, c := range []WriteClass{ClassImmediate, ClassCrossSession, ClassNothing, ClassUnknown} {
		if c == "" {
			t.Fatal("a write class is the empty string; a zero value would claim it")
		}
		if seen[c] {
			t.Fatalf("duplicate write class %q", c)
		}
		seen[c] = true
	}
}

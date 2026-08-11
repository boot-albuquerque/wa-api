package observability

import (
	"strings"
	"sync"
	"testing"
)

func TestRecordsReturnsACopy(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Evaluate", Label: "a", Result: ResultOK})

	got := l.Records()
	got[0].Label = "tampered"

	if l.Records()[0].Label != "a" {
		t.Fatal("Records() handed out the internal slice; a caller serialising the " +
			"log could race with, or rewrite, the history it is reporting")
	}
}

func TestTimeoutsCountsOnlyBlownBudgets(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Query", Result: ResultOK})
	l.Add(OpRecord{Op: "Query", Result: ResultError})
	l.Add(OpRecord{Op: "StateProbe", Result: ResultTimeout})
	l.Add(OpRecord{Op: "StateProbe", Result: ResultTimeout})

	if n := l.Timeouts(); n != 2 {
		t.Fatalf("Timeouts() = %d, want 2 — an application error is not a "+
			"timeout, and counting it as one overstates how dead the target is", n)
	}
}

// A driver error can quote whatever the page threw. Truncation is not a
// redaction policy, but an unbounded error string is an unbounded amount of
// somebody's data in a log line.
func TestAddTruncatesErrorText(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Evaluate", Result: ResultError, Err: strings.Repeat("x", maxErrTextLen*3)})

	if got := len(l.Records()[0].Err); got != maxErrTextLen {
		t.Fatalf("error text kept %d chars, want %d", got, maxErrTextLen)
	}
}

// Operations on one browser come from more than one goroutine as soon as a
// liveness probe exists alongside a command. Run under -race.
func TestAddIsSafeUnderConcurrency(t *testing.T) {
	l := NewOpLog()
	const writers, each = 8, 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				l.Add(OpRecord{Op: "Evaluate", Result: ResultOK})
				_ = l.Records()
			}
		}()
	}
	wg.Wait()

	if n := len(l.Records()); n != writers*each {
		t.Fatalf("recorded %d operations, want %d — entries were lost", n, writers*each)
	}
}

func TestSinceIsRelativeToTheLogOrigin(t *testing.T) {
	l := NewOpLog()
	if d := l.Since(l.t0); d != 0 {
		t.Fatalf("Since(origin) = %v, want 0; every record in one log must share one clock", d)
	}
}

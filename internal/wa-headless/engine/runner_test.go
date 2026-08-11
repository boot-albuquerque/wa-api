package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// testPolicy gives every class a distinct, short budget so that "Do used the
// deadline of the WRONG class" is a visible failure rather than a coincidence.
var testPolicy = DeadlinePolicy{
	Navigate:      300 * time.Millisecond,
	Query:         310 * time.Millisecond,
	Evaluate:      320 * time.Millisecond,
	Action:        330 * time.Millisecond,
	StateProbe:    40 * time.Millisecond,
	RecoveryProbe: 50 * time.Millisecond,
	Shutdown:      340 * time.Millisecond,
}

// The degenerate implementation of Do is `return f(parent)` — it passes any
// test that only checks the return value of a fast operation. This one fails
// against it, because an unbounded parent has no deadline to report.
func TestDoBoundsTheContextByClass(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	for _, k := range allOpKinds {
		var deadline time.Time
		var ok bool
		err := r.Do(context.Background(), k, "budget", func(ctx context.Context) error {
			deadline, ok = ctx.Deadline()
			return nil
		})
		if err != nil {
			t.Fatalf("%s: unexpected error %v", k, err)
		}
		if !ok {
			t.Fatalf("%s: f received a context with NO deadline — the policy was not applied", k)
		}
		want := testPolicy.For(k)
		got := time.Until(deadline)
		// Generous window: the assertion is "the budget of THIS class", and the
		// classes are 10ms apart, so anything looser would stop discriminating.
		if got > want || got < want-20*time.Millisecond {
			t.Errorf("%s: remaining budget %v, want ~%v — Do applied the wrong class", k, got, want)
		}
	}
}

func TestDoReportsTimeoutAsTimeout(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	err := r.Do(context.Background(), OpStateProbe, "slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("got %T (%v), want *TimeoutError", err, err)
	}
	if te.Op != OpStateProbe || te.Label != "slow" || te.Deadline != testPolicy.StateProbe {
		t.Errorf("TimeoutError lost its identity: %+v", te)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("TimeoutError must unwrap to context.DeadlineExceeded")
	}
}

// The one that matters most: a driver that swallows the cancellation and
// returns nil must NOT be believed. "The operation succeeded" and "the deadline
// blew and nobody noticed" are the two readings, and reporting the first is how
// a dead session gets counted as capacity.
func TestDoDoesNotBelieveASuccessThatOutlivedItsDeadline(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	err := r.Do(context.Background(), OpStateProbe, "liar", func(ctx context.Context) error {
		<-ctx.Done()
		return nil // the driver reports success after the budget is gone
	})
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("got %v, want *TimeoutError: the deadline blew and f lied about it", err)
	}
}

// The mirror of the case above, and just as important: an application error
// that arrived IN TIME must survive untouched. Converting it to a timeout would
// say the target did not answer when it answered with a refusal — the same
// class of lie, pointing the other way.
func TestDoPreservesTheOperationError(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	sentinel := errors.New("the page refused")
	err := r.Do(context.Background(), OpEvaluate, "refused", func(ctx context.Context) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the operation's own error", err)
	}
	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatal("an in-time application error was reported as a timeout")
	}
}

// The property that forced PrimeTab to exist: the bounded context dies with Do.
// Anything created inside f that captures it dies too.
func TestDoCancelsTheChildContextOnReturn(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	var captured context.Context
	_ = r.Do(context.Background(), OpEvaluate, "capture", func(ctx context.Context) error {
		captured = ctx
		return nil
	})
	select {
	case <-captured.Done():
	case <-time.After(time.Second):
		t.Fatal("the bounded context outlived Do; the deadline would not be enforceable")
	}
	if errors.Is(captured.Err(), context.DeadlineExceeded) {
		t.Fatal("the fast path must end in cancellation, not in an expired deadline")
	}
}

// Parent cancellation is not a timeout. Shutting the stack down must not be
// recorded as "the target stopped answering", or every clean stop would look
// like an incident.
func TestDoDoesNotReportParentCancellationAsTimeout(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := r.Do(parent, OpNavigate, "cancelled", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatal("parent cancellation was reported as a deadline of the operation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestNewRunnerUsesTheMeasuredDefaults(t *testing.T) {
	if NewRunner().Policy != DefaultDeadlines {
		t.Fatal("NewRunner must carry DefaultDeadlines; a Runner with a zero policy " +
			"fails every operation instantly")
	}
}

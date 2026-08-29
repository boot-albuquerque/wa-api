package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/observability"
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
	Boot:          350 * time.Millisecond,
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

// The four cases HOUSEKEEP H42 demands, kept together because the defect was
// precisely that two of them were indistinguishable from a third.
//
// A derived context reports DeadlineExceeded whether the operation's own clock
// ran out or the caller's already had, so classifying on ctx.Err() blamed the
// target for a page it never consulted. The fix attaches a private cause to the
// operation's own timeout and reads context.Cause, which stays correct even
// when the two deadlines coincide.

// 1. The caller's deadline is SHORTER than the operation's. This is the case
// that produced two false conclusions in one session: the error said "deadline
// of 5s exceeded" while what had run out were the caller's 60 seconds.
func TestDoBlamesTheCallerWhenTheParentDeadlineExpiresFirst(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}
	// Navigate's budget is 300ms; the caller allows 20ms.
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := r.Do(parent, OpNavigate, "parent-expires", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatalf("the caller's expiry was reported as the operation's own deadline: %v", err)
	}
	var ge *CallerGaveUpError
	if !errors.As(err, &ge) {
		t.Fatalf("got %T (%v), want *CallerGaveUpError", err, err)
	}
	if !strings.Contains(err.Error(), "CALLER") {
		t.Fatalf("the message must name the caller, not the target: %v", err)
	}
	if n := r.Log.Timeouts(); n != 0 {
		t.Fatalf("Timeouts()=%d, want 0 — a caller that ran out of budget is not a "+
			"target that stopped answering", n)
	}
	if n := r.Log.CallerGaveUp(); n != 1 {
		t.Fatalf("CallerGaveUp()=%d, want 1", n)
	}
}

// 2. The caller CANCELS. This one already passed before the fix, and that is
// the trap it documents: Canceled is distinguishable from DeadlineExceeded, so
// the protected half made both halves look protected.
func TestDoBlamesTheCallerOnCancellationAndRecordsItAsSuch(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}
	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := r.Do(parent, OpNavigate, "parent-cancelled", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatalf("cancellation was reported as a deadline: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want it to unwrap to context.Canceled", err)
	}
	if n := r.Log.Timeouts(); n != 0 {
		t.Fatalf("Timeouts()=%d, want 0 — a clean stop must not read as an outage", n)
	}
	if n := r.Log.CallerGaveUp(); n != 1 {
		t.Fatalf("CallerGaveUp()=%d, want 1", n)
	}
}

// 3. The operation's OWN deadline. The fix must not make real timeouts vanish,
// which is the failure a caller would never notice until a dead session was
// reported as healthy.
func TestDoStillReportsTheOperationsOwnDeadline(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}
	// StateProbe's budget is 40ms; the caller allows far more.
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := r.Do(parent, OpStateProbe, "own-deadline", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("got %T (%v), want *TimeoutError", err, err)
	}
	if te.Op != OpStateProbe {
		t.Fatalf("the timeout named %s, want %s", te.Op, OpStateProbe)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("a real timeout must still unwrap to context.DeadlineExceeded")
	}
	if n := r.Log.Timeouts(); n != 1 {
		t.Fatalf("Timeouts()=%d, want 1", n)
	}
	if n := r.Log.CallerGaveUp(); n != 0 {
		t.Fatalf("CallerGaveUp()=%d, want 0 — this was the operation's own clock", n)
	}
}

// 4. Success, with a caller whose deadline is nearby. The classification reads
// the context AFTER the call, so an operation that finished in time must not be
// reclassified by a parent that expires a moment later.
func TestDoReportsSuccessWithoutBlamingAnyone(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}
	parent, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := r.Do(parent, OpNavigate, "fine", func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if n := r.Log.Timeouts(); n != 0 {
		t.Fatalf("Timeouts()=%d, want 0", n)
	}
	if n := r.Log.CallerGaveUp(); n != 0 {
		t.Fatalf("CallerGaveUp()=%d, want 0", n)
	}
	recs := r.Log.Records()
	if len(recs) != 1 || recs[0].Result != observability.ResultOK {
		t.Fatalf("records=%v, want a single ok", recs)
	}
}

// THE TARGET GOING AWAY IS ITS OWN VERDICT (decisão 69).
//
// Measured in H182: a call in flight when the browser is SIGKILLed returns
// `context canceled` — the vocabulary of a cancellation the CALLER asked for —
// on a context nobody cancelled. A caller cannot act on that, and the module
// already carries TimeoutError precisely so "no answer" is not confused with
// "an answer that is an error".
func TestATargetThatWentAwayIsNamed(t *testing.T) {
	r := NewRunner()
	r.TargetAlive = func() bool { return false }
	err := r.Do(context.Background(), OpStateProbe, "t", func(context.Context) error {
		return context.Canceled
	})
	var gone *TargetGoneError
	if !errors.As(err, &gone) {
		t.Fatalf("err = %v, want TargetGoneError", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("the original cause was dropped; a caller that wants it cannot reach it")
	}
}

// A LIVE TARGET IS NOT ACCUSED. The probe only reclassifies when the process is
// actually absent — otherwise every application error would become "the browser
// is gone".
func TestALiveTargetKeepsTheDriversError(t *testing.T) {
	r := NewRunner()
	r.TargetAlive = func() bool { return true }
	// O ERRO E' JUSTAMENTE O QUE A MENSAGEM ACUSARIA. Um primeiro controle
	// negativo trocou a sonda de processo por `errors.Is(err, context.Canceled)`
	// e este teste PASSOU, porque usava um erro qualquer — media a sonda com uma
	// entrada que a redacao nunca reclamaria. Com context.Canceled, classificar
	// pela mensagem falha aqui, que e' o ponto da decisao 69.
	want := fmt.Errorf("driver said: %w", context.Canceled)
	err := r.Do(context.Background(), OpStateProbe, "t", func(context.Context) error {
		return want
	})
	var gone *TargetGoneError
	if errors.As(err, &gone) {
		t.Fatal("a live browser was reported as gone")
	}
	if !errors.Is(err, want) {
		t.Fatalf("the driver's error was replaced: %v", err)
	}
}

// A TIMEOUT STAYS A TIMEOUT even if the browser died right after. The order of
// the checks is the rule: our own deadline explains the failure first, and a
// process that went away afterwards does not rewrite what happened.
func TestATimeoutIsNotRelabelledWhenTheTargetAlsoDied(t *testing.T) {
	r := NewRunner()
	r.Policy.StateProbe = 20 * time.Millisecond
	r.TargetAlive = func() bool { return false }
	err := r.Do(context.Background(), OpStateProbe, "t", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	var to *TimeoutError
	if !errors.As(err, &to) {
		t.Fatalf("err = %v, want TimeoutError", err)
	}
}

// WITHOUT A PROBE, NOTHING IS CLAIMED. A Runner with no way to check the process
// passes the driver's error through rather than guessing.
func TestWithoutAProbeTheErrorIsUntouched(t *testing.T) {
	r := NewRunner()
	want := errors.New("boom")
	err := r.Do(context.Background(), OpStateProbe, "t", func(context.Context) error {
		return want
	})
	// O errors.Is SOZINHO NAO BASTA: TargetGoneError desembrulha para a causa,
	// entao um Runner que acusasse sem sonda ainda passaria por ele. A asserção
	// tem de ser sobre o TIPO — foi assim que um controle negativo que nao mordeu
	// revelou este teste fraco.
	var gone *TargetGoneError
	if errors.As(err, &gone) {
		t.Fatal("a Runner with no way to check the process claimed the browser was gone")
	}
	if !errors.Is(err, want) {
		t.Fatalf("the error changed with no probe to justify it: %v", err)
	}
}

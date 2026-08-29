package engine

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/headless/observability"
)

func TestNewRunnerTracesByDefault(t *testing.T) {
	// Tracing that has to be switched on is tracing that will be off during the
	// one run that needed it.
	if NewRunner().Log == nil {
		t.Fatal("NewRunner produced a Runner with no OpLog; a stop with no trace " +
			"is indistinguishable from work still in progress")
	}
}

func TestDoRecordsTheOperation(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}
	_ = r.Do(context.Background(), OpNavigate, "boot/navigate", func(ctx context.Context) error {
		return nil
	})

	recs := r.Log.Records()
	if len(recs) != 1 {
		t.Fatalf("recorded %d operations, want 1", len(recs))
	}
	got := recs[0]
	if got.Op != string(OpNavigate) || got.Label != "boot/navigate" {
		t.Errorf("record lost its identity: %+v", got)
	}
	if got.Result != observability.ResultOK {
		t.Errorf("Result = %q, want %q", got.Result, observability.ResultOK)
	}
	// The promised budget must be recorded next to the actual duration.
	// Without it the log says how long something took but not whether that was
	// too long, which is the only question the record has to answer.
	if got.DeadlineMS != testPolicy.Navigate.Milliseconds() {
		t.Errorf("DeadlineMS = %d, want %d", got.DeadlineMS, testPolicy.Navigate.Milliseconds())
	}
}

// The three outcomes must stay distinct in the record, for the same reason they
// stay distinct in the return value: a run that collapses them cannot say
// whether the target refused or went silent.
func TestDoRecordsTimeoutAndErrorDistinctly(t *testing.T) {
	r := &Runner{Policy: testPolicy, Log: observability.NewOpLog()}

	_ = r.Do(context.Background(), OpStateProbe, "silent", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	_ = r.Do(context.Background(), OpEvaluate, "refused", func(ctx context.Context) error {
		return errors.New("the page refused")
	})

	recs := r.Log.Records()
	if len(recs) != 2 {
		t.Fatalf("recorded %d operations, want 2", len(recs))
	}
	if recs[0].Result != observability.ResultTimeout {
		t.Errorf("the silent target was recorded as %q, want %q",
			recs[0].Result, observability.ResultTimeout)
	}
	if recs[1].Result != observability.ResultError {
		t.Errorf("the refusal was recorded as %q, want %q",
			recs[1].Result, observability.ResultError)
	}
	if r.Log.Timeouts() != 1 {
		t.Errorf("Timeouts() = %d, want 1", r.Log.Timeouts())
	}
}

// A Runner built as a zero value — which tests and future call sites will do —
// must still enforce deadlines. Tracing is what degrades, never the policy.
func TestDoWorksWithoutALog(t *testing.T) {
	r := &Runner{Policy: testPolicy}
	err := r.Do(context.Background(), OpStateProbe, "nolog", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("got %v, want *TimeoutError even with tracing off", err)
	}
}

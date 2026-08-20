package engine

// Runner is the only sanctioned way to perform a remote operation.
//
// It exists so that "apply the deadline" is not something a call site can
// forget. A path that talks to the browser outside Runner.Do is outside the
// policy, and the static gate in this package treats it as a defect rather than
// as a style preference.
//
// Study origin: scripts/chromium-study/p4c_deadline.go (phase 4C, sections 3-6).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wa-api/internal/wa-headless/observability"
)

// TimeoutError is "the target did not answer" — a claim that must never be
// confused with "the target answered with an error".
//
// The distinction is the whole reason this type exists instead of a formatted
// string. Phase 6 measured a page that stopped executing JavaScript for over
// four minutes while its target stayed attached and its process stayed alive:
// the only signal that separated a live session from a dead one was whether an
// evaluation came back within a deadline. A caller that cannot tell a timeout
// from an application error cannot make that call, and every caller writing its
// own strings.Contains against an error message would be one caller away from
// getting it wrong.
//
// It unwraps to context.DeadlineExceeded so errors.Is keeps working for code
// that only cares that time ran out.
type TimeoutError struct {
	Op       OpKind
	Label    string
	Deadline time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s(%s): deadline of %s exceeded", e.Op, e.Label, e.Deadline)
}

func (e *TimeoutError) Unwrap() error { return context.DeadlineExceeded }

// errOperationDeadline is the private cause attached to an operation's OWN
// timeout. It is never returned to a caller; it exists so that
// context.Cause can tell this operation's clock from the caller's.
var errOperationDeadline = errors.New("engine: the operation's own deadline")

// CallerGaveUpError is "the caller stopped waiting" — which is not a claim
// about the target at all.
//
// It exists for the same reason TimeoutError does: so a caller can branch on
// the cause instead of reading a string. Retrying this is work nobody asked
// for, while retrying a TimeoutError may be exactly right.
//
// It unwraps to the parent's cause, so errors.Is keeps working for code that
// only cares whether it was a cancellation or an expiry.
type CallerGaveUpError struct {
	Op    OpKind
	Label string
	Cause error
}

func (e *CallerGaveUpError) Error() string {
	return fmt.Sprintf("%s(%s): the CALLER's context ended first (%v); the target was not asked",
		e.Op, e.Label, e.Cause)
}

func (e *CallerGaveUpError) Unwrap() error { return e.Cause }

// Runner applies a DeadlinePolicy to every operation it executes, and records
// each one.
//
// The recording is not optional tooling. A browser stack fails by stopping, and
// a stop with no trace is indistinguishable from work still in progress — which
// is how a 24-minute hang went unexplained until every operation carried its
// budget beside its duration.
type Runner struct {
	Policy DeadlinePolicy
	Log    *observability.OpLog
}

// NewRunner builds a Runner on the measured defaults, with tracing on.
func NewRunner() *Runner {
	return &Runner{Policy: DefaultDeadlines, Log: observability.NewOpLog()}
}

// Do runs f under the deadline of class k.
//
// f receives a context that is ALREADY bounded; it cannot choose to wait
// longer. The bounded context is cancelled when Do returns, and that is not an
// implementation detail: it is the property that makes the deadline real. It is
// also the property that forced PrimeTab to exist, because a lazily created
// browser target inherits the context of the first call that touched it, and
// creating it inside a Do would kill it as soon as that Do returned.
//
// The returned error is a *TimeoutError when the budget ran out, and f's own
// error otherwise.
func (r *Runner) Do(parent context.Context, k OpKind, label string, f func(context.Context) error) error {
	deadline := r.Policy.For(k)
	// WithTimeoutCause, not WithTimeout, and the cause is what makes the
	// classification below structural instead of inferred.
	//
	// ctx derives from parent, so ctx.Err() reports DeadlineExceeded whether
	// THIS operation ran out of time or the CALLER already had. Those are
	// opposite diagnoses, and the code used to report both as the operation's
	// own 5s budget being blown — blaming the target for a page it never
	// consulted. Attaching a private cause makes the two distinguishable even
	// when the deadlines coincide, which no comparison of parent.Err() can
	// promise. See HOUSEKEEP H42.
	ctx, cancel := context.WithTimeoutCause(parent, deadline, errOperationDeadline)
	defer cancel()

	start := time.Now()
	err := f(ctx)
	elapsed := time.Since(start)

	// The context is consulted, not the error: a driver is free to report a
	// blown deadline as any error it likes, or as none at all, and trusting its
	// wording would put the classification in someone else's hands.
	//
	// The CAUSE separates whose clock ran out. Only our own cause is this
	// operation timing out; anything else that ended the context came from the
	// caller.
	timedOut := errors.Is(context.Cause(ctx), errOperationDeadline)
	callerGaveUp := !timedOut && ctx.Err() != nil

	rec := observability.OpRecord{
		Op:         string(k),
		Label:      label,
		DurationMS: elapsed.Milliseconds(),
		DeadlineMS: deadline.Milliseconds(),
		Result:     observability.ResultOK,
	}
	switch {
	case timedOut:
		rec.Result = observability.ResultTimeout
	case callerGaveUp:
		rec.Result = observability.ResultCallerGaveUp
	case err != nil:
		rec.Result = observability.ResultError
	}
	if err != nil {
		rec.Err = err.Error()
	}
	if r.Log != nil {
		rec.StartMS = r.Log.Since(start).Milliseconds()
		r.Log.Add(rec)
	}

	if timedOut {
		return &TimeoutError{Op: k, Label: label, Deadline: deadline}
	}
	if callerGaveUp {
		// Name the caller, not the target. The message is the whole point of
		// this branch: the previous wording sent two investigations after a
		// page that had never been asked anything.
		return &CallerGaveUpError{Op: k, Label: label, Cause: context.Cause(parent)}
	}
	return err
}

package engine

// Deadlines are a property of the OPERATION CLASS, never of the call site.
//
// This is the single most expensive lesson of the chromium study, and it was
// paid three times. Phase 4B declared its own harness UNRELIABLE after three
// runs hung on the same cause — a CDP call with no deadline — and each spot fix
// let the defect return in the next file. The conclusion was not "a timeout was
// missing there"; it was that a deadline the caller has to remember is a
// deadline that will eventually be forgotten.
//
// So there is no exported function in this package that performs a remote
// operation without deriving its context from a policy. A new code path cannot
// opt out, because opting out would mean not calling the only entry point that
// exists.
//
// Study origin: scripts/chromium-study/p4c_deadline.go (phase 4C, sections 3-6).

import "time"

// OpKind names the class of a remote operation, and with it the applicable
// deadline. It is a closed set on purpose: a caller that cannot find its class
// here is describing an operation this package has not budgeted for, and that
// is a design question, not a parameter.
type OpKind string

const (
	OpNavigate      OpKind = "Navigate"
	OpQuery         OpKind = "Query"
	OpEvaluate      OpKind = "Evaluate"
	OpAction        OpKind = "Action"
	OpStateProbe    OpKind = "StateProbe"
	OpRecoveryProbe OpKind = "RecoveryProbe"
	OpShutdown      OpKind = "Shutdown"
)

// DeadlinePolicy is the deadline of each class of remote operation.
//
// The values are measured, not chosen. Navigate covers the worst case observed
// against the real target with headroom. StateProbe is deliberately short: a
// tab that does not answer within it IS the result, not a wait to be extended —
// phase 6 measured a page that stopped executing JavaScript for over four
// minutes while every structural signal (target attached, process alive,
// service worker present) reported healthy. Extending the probe would only have
// bought a slower way to learn the same thing.
type DeadlinePolicy struct {
	Navigate      time.Duration `json:"navigate"`
	Query         time.Duration `json:"query"`
	Evaluate      time.Duration `json:"evaluate"`
	Action        time.Duration `json:"action"`
	StateProbe    time.Duration `json:"state_probe"`
	RecoveryProbe time.Duration `json:"recovery_probe"`
	Shutdown      time.Duration `json:"shutdown"`
}

// DefaultDeadlines carries the values validated across phases 4C, 5 and 6 of
// the study. Changing one of them changes what a measurement means, so a change
// here belongs with the measurement that motivated it.
var DefaultDeadlines = DeadlinePolicy{
	Navigate:      30 * time.Second,
	Query:         15 * time.Second,
	Evaluate:      10 * time.Second,
	Action:        15 * time.Second,
	StateProbe:    5 * time.Second,
	RecoveryProbe: 2 * time.Second,
	Shutdown:      10 * time.Second,
}

// For returns the deadline of a class.
//
// An unknown class falls back to Evaluate rather than to "no deadline": the
// failure mode of a wrong-but-finite budget is a spurious timeout, which is
// visible. The failure mode of an unbounded wait is the 24-minute hang this
// package exists to prevent.
func (p DeadlinePolicy) For(k OpKind) time.Duration {
	switch k {
	case OpNavigate:
		return p.Navigate
	case OpQuery:
		return p.Query
	case OpEvaluate:
		return p.Evaluate
	case OpAction:
		return p.Action
	case OpStateProbe:
		return p.StateProbe
	case OpRecoveryProbe:
		return p.RecoveryProbe
	case OpShutdown:
		return p.Shutdown
	}
	return p.Evaluate
}

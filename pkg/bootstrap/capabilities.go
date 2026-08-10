package bootstrap

import (
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// Capability report (ADR-0005 D7, and the fix for F104).
//
// # The problem this solves
//
// What this process can and cannot do is spread across three places today: the
// cluster mode comes from an environment variable, the database type is
// RESOLVED (never declared), and "can this run in N pods?" is a conclusion a
// reader has to derive from both. Nobody derives it. The one signal that
// existed — the `falling back to sqlite` warning — only fires when the Postgres
// variables are PARTIALLY set, and it is one line among hundreds at startup.
//
// F104 is the consequence: a deployment scaled to two replicas without
// declaring `WA_API_CLUSTER_MODE=multi` resolves to `single` + SQLite, which is
// a LEGAL combination, and nothing refuses it. The D1 guard only protects
// whoever declared `multi` — precisely the step most likely to be forgotten.
//
// This report does not prevent that. It makes the state DECLARED instead of
// inferred, which is the difference between an operator seeing it and an
// operator deriving it.
//
// # Why the values are strings and not booleans
//
// `multi_pod: false` cannot distinguish "SQLite, this is impossible" from
// "Postgres is there, you just have not turned it on". Those two need opposite
// actions from whoever is reading, and collapsing them into one boolean is how
// a report becomes decoration.

const (
	// multiPodUnsupported: the stack cannot support N pods at all. Each replica
	// would carry its own SQLite file — or worse, share the same file through
	// the same volume, which is corruption rather than divergence.
	multiPodUnsupported = "not_supported"

	// multiPodAvailable: the stack COULD support N pods (Postgres is there),
	// but the mode was not declared, so this process behaves as a single one.
	multiPodAvailable = "available"

	// multiPodActive: declared and in force.
	multiPodActive = "active"

	// ownershipEnforced / ownershipInactive: whether the per-session lease is
	// actually arbitrating. In `single` it is inert BY DESIGN — one process has
	// nothing to coordinate with — and reporting it as "ok" would train people
	// to ignore the field.
	ownershipEnforced = "enforced"
	ownershipInactive = "inactive"

	// durableRetryOutbox states where delivery durability comes from. It is
	// constant on purpose: the outbox works on both dialects (ADR-0005 D3), and
	// this field exists to kill the recurring assumption that durability needs
	// a broker or Postgres.
	durableRetryOutbox = "outbox"

	// capabilityReportMessage is the log message. Grep-able on purpose: it is
	// the first thing to ask for when someone reports "it is behaving as if it
	// were another instance".
	capabilityReportMessage = "capability report"
)

// capabilityReport is what this process declares it can do.
//
// It is also served in the /health/ready body, which is UNAUTHENTICATED. The
// disclosure is deliberate and bounded: these are deployment-shape facts
// (sqlite vs postgres, single vs multi), never host, port, user or credential —
// the same line the readiness check already draws when it puts a reason code in
// the body and the driver error in the log.
type capabilityReport struct {
	ClusterMode      string `json:"cluster_mode"`
	Database         string `json:"database"`
	MultiPod         string `json:"multi_pod"`
	SessionOwnership string `json:"session_ownership"`
	DurableRetry     string `json:"durable_retry"`
}

// currentCapabilities holds the report for the running process.
//
// A pointer rather than a value so the zero state ("nobody set it") is
// distinguishable from a legitimately empty report — see currentCapabilityReport.
var currentCapabilities atomic.Pointer[capabilityReport]

// publishCapabilities derives the report, records it, and logs it once.
//
// Called from the startup path right after the mode is resolved and validated,
// which is the earliest moment both facts are known — and before the database
// is opened, so a process that dies on a bad stack still says what it thought
// it was.
//
// # Why deriving is not a separate function
//
// It was, and the split cost more than it bought. A pure `buildCapabilityReport`
// is an eligible function with nothing to log — it has no error path and touches
// no I/O — so it lowered the log-coverage ratio for a separation with no
// independent purpose: nobody derives this report without publishing it.
//
// The derivation stays fully testable through the return value, which is why
// this function returns the report it just published.
//
// `multi` already implies Postgres — D1 refuses to start otherwise — so the
// order of the two conditions below cannot produce a contradictory report.
func publishCapabilities(mode, databaseType string) capabilityReport {
	report := capabilityReport{
		ClusterMode:      mode,
		Database:         databaseType,
		MultiPod:         multiPodUnsupported,
		SessionOwnership: ownershipInactive,
		DurableRetry:     durableRetryOutbox,
	}

	if databaseType == databaseTypePostgres {
		report.MultiPod = multiPodAvailable
	}

	if mode == clusterModeMulti {
		report.MultiPod = multiPodActive
		report.SessionOwnership = ownershipEnforced
	}

	currentCapabilities.Store(&report)

	log.Info().
		Str("cluster_mode", report.ClusterMode).
		Str("database", report.Database).
		Str("multi_pod", report.MultiPod).
		Str("session_ownership", report.SessionOwnership).
		Str("durable_retry", report.DurableRetry).
		Msg(capabilityReportMessage)

	return report
}

// currentCapabilityReport reads the published report.
//
// Returns the zero value when nothing was published — which happens in tests
// that exercise handlers without the startup path. The zero value has empty
// strings rather than plausible defaults ON PURPOSE: a report that invents
// `cluster_mode: single` when nobody declared anything would be the same class
// of lie this file exists to remove.
func currentCapabilityReport() capabilityReport {
	if report := currentCapabilities.Load(); report != nil {
		return *report
	}
	return capabilityReport{}
}

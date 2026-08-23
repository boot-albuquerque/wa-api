package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Health separation (ADR-0005, D6).
//
// The Exp 1 measurement showed a healthy PROCESS with a dead SESSION while the
// database still said connected=1. One probe cannot answer both questions, and
// answering only the first is what makes a readiness probe lie.
//
//	/health/live   the process is up and the HTTP server answers.
//	/health/ready  this pod can SERVE: the database answers, and — in multi —
//	               the ownership heartbeat is still ticking.
//
// # What readiness deliberately does NOT check
//
// The ADR states readiness as "this pod is serving the sessions it claims to
// own". Taken literally, a single dead session would flip the probe and k8s
// would pull the pod out of rotation ENTIRELY — cutting off the other ninety
// nine sessions it is serving perfectly well. The cure would be worse than the
// disease.
//
// The literal question is per-SESSION, and a readiness probe is per-POD: it has
// no way to express "route user A elsewhere, keep sending me user B". That is
// routing-by-owner (D5), not readiness, and until it exists the honest split is:
//
//   - readiness FAILS on what makes the whole pod unable to serve;
//   - per-session drift is REPORTED, in /health, where an operator and a future
//     router can read it without taking the pod down.
//
// Recording this because the divergence from the ADR text is deliberate. F98
// already removed the drift's main source — a lease held for a session that no
// longer exists is now handed back instead of renewed forever.
const (
	// readinessDBTimeout bounds the database check. A readiness probe that can
	// hang is worse than one that fails: kubelet would wait out its own
	// timeout with no answer, and the pod would sit in an undefined state.
	readinessDBTimeout = 2 * time.Second

	statusReady    = "ready"
	statusNotReady = "not_ready"
	checkOK        = "ok"
)

// ReadinessReport is what /health/ready answers.
//
// Checks carries a per-dependency verdict rather than a single boolean because
// "not ready" without a reason costs an operator a debugging session. The
// values are short reason codes, never error strings from the database: this
// endpoint is unauthenticated (the kubelet cannot carry a token), and a driver
// error can leak host, port and user.
//
// Capabilities carries what this pod IS (sqlite vs postgres, single vs multi),
// which is a different question from whether it is healthy — a SQLite pod in
// `single` is perfectly ready and still cannot serve as one of N replicas.
// Keeping it out of Checks is deliberate: Checks holds verdicts that can fail
// the probe, and none of these can. Mixing facts into verdicts is how a report
// stops being read.
type ReadinessReport struct {
	Status       string            `json:"status"`
	Checks       map[string]string `json:"checks"`
	Capabilities capabilityReport  `json:"capabilities"`
}

// Ready reports whether every check passed.
func (r ReadinessReport) Ready() bool { return r.Status == statusReady }

// pinger is the slice of the database readiness needs. Narrow on purpose, so
// the probe can be tested without a database.
type pinger interface {
	PingContext(ctx context.Context) error
}

// buildReadinessProbe assembles the readiness check for this process.
//
// The lease manager is nil in `single` mode BY DESIGN (there is no ownership to
// coordinate), and the ownership check is then absent from the report rather
// than reported as passing. A check that always says "ok" trains people to
// ignore it.
//
// # Why leases arrives as a getter and not as a value
//
// The router is built by s.routes() (main.go:409), and the lease manager is
// installed by setupSessionOwnership four lines LATER — the order is
// deliberate, because ownership has to exist before connectOnStartup decides
// which sessions this process may take.
//
// Capturing *leaseManager at wiring time therefore captures nil, forever, even
// in `multi`. The check would simply never appear, and the report would look
// exactly like a healthy `single` process. Measured on the bench: with the
// value form, GET /health/ready in multi answered {"database":"ok"} and nothing
// else. Reading through a getter defers the lookup to request time, which is
// when the answer is knowable.
func buildReadinessProbe(db pinger, leases func() *leaseManager) func(context.Context) ReadinessReport {
	return func(ctx context.Context) ReadinessReport {
		// Lido AQUI, e não capturado no fio, pelo mesmo motivo que o lease
		// acima: o relatório é publicado no arranque, e capturar o valor em
		// tempo de montagem congelaria o que estivesse lá naquele instante.
		// Com o lease isso já aconteceu de verdade — o campo simplesmente
		// nunca aparecia. Ler em tempo de requisição é ler quando a resposta é
		// conhecível.
		report := ReadinessReport{
			Status:       statusReady,
			Checks:       map[string]string{},
			Capabilities: currentCapabilityReport(),
		}

		dbCtx, cancel := context.WithTimeout(ctx, readinessDBTimeout)
		defer cancel()

		switch err := db.PingContext(dbCtx); {
		case err == nil:
			report.Checks["database"] = checkOK
		default:
			// The error goes to the log, where it is authenticated; the body
			// gets a code. Sessions cannot be served without the database, so
			// this is the one check that is unambiguously fatal to readiness.
			log.Warn().Err(err).Dur("timeout", readinessDBTimeout).
				Msg("readiness: database did not answer; reporting this pod as not ready")
			report.Checks["database"] = "unreachable"
			report.Status = statusNotReady
		}

		var manager *leaseManager
		if leases != nil {
			manager = leases()
		}

		if manager != nil && manager.HeartbeatStalled() {
			log.Error().
				Msg("readiness: the ownership heartbeat stopped ticking; this pod's leases are expiring and another replica may take sessions it is still serving")
			report.Checks["session_ownership"] = "heartbeat_stalled"
			report.Status = statusNotReady
		} else if manager != nil {
			report.Checks["session_ownership"] = checkOK
		}

		return report
	}
}

// livenessHandler answers /health/live (and the older /livez).
//
// No auth, no database, no ReadMemStats — cheap enough to be hit by every
// HEALTHCHECK tick without becoming a DoS amplifier. It answers exactly one
// question: is this process still able to serve HTTP?
func livenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// readinessHandler answers /health/ready.
//
// 503 on failure, not 500: the pod is temporarily unable to serve, and 5xx-as-
// bug would be read as "this process is broken, restart it" when the usual
// cause is a dependency that will come back.
func readinessHandler(probe func(context.Context) ReadinessReport) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report := probe(r.Context())

		status := http.StatusOK
		if !report.Ready() {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(report); err != nil {
			// The status line is already out; there is nothing left to
			// negotiate. Worth a log because a probe whose body never arrives
			// looks to the kubelet like a timeout, not an encoding fault.
			log.Warn().Err(err).Msg("readiness: failed to write the report body")
		}
	})
}

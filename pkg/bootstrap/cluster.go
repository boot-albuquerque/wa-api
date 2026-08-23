package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// Cluster mode (ADR-0005, D1).
//
// The target is running on N pods, but the core — keeping sessions alive and
// delivering events — never needs more than SQLite and one process. Both ends
// are supported modes, and what separates them is an EXPLICIT decision, never
// an inference.
//
// This exists because of two SILENT failure modes measured in F89:
//
//  1. The production install runs on SQLite without anyone asking for it: the
//     Postgres variables were PARTIALLY set and the process degraded with a
//     warning. Across N pods that would be every replica on its own database,
//     each believing it owns everything.
//  2. Two replicas on the same session do not fight in a loop, as assumed.
//     WhatsApp sends ONE `StreamReplaced`, the loser's session dies and never
//     reconnects — process alive, HTTP answering, liveness green, session dead.
//     And the database still reports `connected=1`.
//
// In both cases the system stayed up while lying about its own state. The
// answer here is to refuse to start rather than degrade.

const (
	envClusterMode = "WA_API_CLUSTER_MODE"

	// clusterModeSingle is the default, and it is the catastrophic-scenario
	// mode: SQLite, one process, no coordination. It stays functional because
	// the core needs nothing more — see the principle in ADR-0005.
	clusterModeSingle = "single"

	// clusterModeMulti allows N processes and therefore REQUIRES a database
	// that can coordinate. It is where the D2 lease applies.
	clusterModeMulti = "multi"

	// databaseTypePostgres is the value GetDatabaseConfig reports for Postgres.
	// Named because three places compare against it — this validation, the
	// store wiring in main.go, and the tests — and a repeated literal is a
	// divergence waiting to happen.
	databaseTypePostgres = "postgres"

	// instanceLockFile lives in the data directory because that is what
	// identifies an installation: two processes with different data dirs are
	// different installations and do not conflict.
	instanceLockFile = ".wa-api-instance.lock"
)

// clusterModeFromEnv reads the mode from the environment.
//
// An absent value means `single` — the safe default, aligned with "SQLite by
// default". An UNKNOWN value is an error, and does not fall back: a typo in
// `WA_API_CLUSTER_MODE=mutli` inside a k8s manifest must not silently become a
// process that believes it owns everything.
func clusterModeFromEnv() (string, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(envClusterMode)))
	switch raw {
	case "":
		return clusterModeSingle, nil
	case clusterModeSingle, clusterModeMulti:
		return raw, nil
	default:
		return "", fmt.Errorf("%s=%q is invalid: use %q or %q", envClusterMode, raw, clusterModeSingle, clusterModeMulti)
	}
}

// validateStackForMode refuses combinations that cannot work.
//
// In `multi`, SQLite is not acceptable degradation: it is corruption waiting to
// happen. Each replica would have its own file, its own set of sessions, and no
// way to know about the others. The error propagates so the caller can kill the
// process.
func validateStackForMode(mode, databaseType string) error {
	if mode == clusterModeMulti && databaseType != databaseTypePostgres {
		return fmt.Errorf(
			"%s=%s requires Postgres, but the resolved database was %q: set DB_USER, DB_PASSWORD, DB_NAME, DB_HOST and DB_PORT (all of them, not a subset)",
			envClusterMode, clusterModeMulti, databaseType)
	}
	return nil
}

// lockSingleInstance guarantees only one process uses this data directory.
//
// `flock` rather than a PID file: the OS releases the lock AUTOMATICALLY when
// the process dies, including on `kill -9`. A PID file would require detecting
// a stale lock, which is exactly where this kind of mechanism tends to fail —
// and it fails by releasing when it should not.
//
// LIMITATION, and it matters: this protects against a second process ON THE
// SAME MACHINE. Two machines pointing at the same Postgres in `single` mode are
// not detected — that is what `multi` mode with a lease is for (ADR-0005 D2).
// The case this lock covers is the one that actually happens: someone starts a
// second process without noticing.
func lockSingleInstance(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory %s: %w", dataDir, err)
	}
	path := filepath.Join(dataDir, instanceLockFile)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening the instance lock %s: %w", path, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf(
			"another process is already using the data directory %s: with %s=%s only one process may run per installation (use %s=%s with Postgres for N replicas)",
			dataDir, envClusterMode, clusterModeSingle, envClusterMode, clusterModeMulti)
	}

	// Recording the PID is diagnostics, not control: the flock is what decides.
	if err := file.Truncate(0); err == nil {
		if _, err := file.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0); err != nil {
			log.Warn().Err(err).Str("file", path).Msg("could not record the PID in the instance lock")
		}
	}

	return func() {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
			log.Warn().Err(err).Str("file", path).Msg("failed to release the instance lock")
		}
		if err := file.Close(); err != nil {
			log.Warn().Err(err).Str("file", path).Msg("failed to close the instance lock")
		}
	}, nil
}

// prepareCluster resolves the mode, validates the stack, and locks the instance
// when the mode is `single`. It returns the release function.
//
// Called BEFORE opening the database: an impossible configuration must die
// without having touched any state.
// It returns the RESOLVED mode, not just the release func: this is the only
// place the mode is decided, and the capability report (D7) needs the same
// value. Resolving it a second time in the caller would let the two drift —
// and a report that disagrees with the running configuration is worse than no
// report, because it is believed.
func prepareCluster(dataDir, databaseType string) (string, func(), error) {
	mode, err := clusterModeFromEnv()
	if err != nil {
		return "", nil, err
	}
	if err := validateStackForMode(mode, databaseType); err != nil {
		return "", nil, err
	}

	if mode == clusterModeMulti {
		// In `multi`, exclusivity comes from the per-session lease (D2), not
		// from an installation-wide lock — locking here would stop the second
		// replica from starting, which is the whole point of the mode.
		return mode, func() {}, nil
	}

	release, err := lockSingleInstance(dataDir)
	if err != nil {
		return "", nil, err
	}
	log.Info().Str("mode", mode).Str("data_dir", dataDir).Msg("single instance locked")
	return mode, release, nil
}

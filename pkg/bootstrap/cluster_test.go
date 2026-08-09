package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-0005 D1. These tests pin the two refusals that prevent the SILENT failure
// modes measured in F89 — degrading to SQLite unasked, and letting a second
// process turn into a zombie.

const databaseTypeSQLite = "sqlite"

func TestClusterMode_DefaultsToSingle(t *testing.T) {
	t.Setenv(envClusterMode, "")
	mode, err := clusterModeFromEnv()
	if err != nil {
		t.Fatalf("unexpected error with the variable unset: %v", err)
	}
	if mode != clusterModeSingle {
		t.Errorf("mode = %q with the variable unset, want %q", mode, clusterModeSingle)
	}
}

// TestClusterMode_UnknownValueIsAnError is the difference between this
// mechanism and the ones that already failed in this repo: an invalid value
// does NOT fall back to the default.
//
// `WA_API_CLUSTER_MODE=mutli` in a k8s manifest would land on `single` if the
// default were forgiving, and the operator would get N replicas each believing
// it owns everything — the silent zombie from F89's experiment 1, multiplied.
func TestClusterMode_UnknownValueIsAnError(t *testing.T) {
	for _, value := range []string{"mutli", "cluster", "1", "true", "yes"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(envClusterMode, value)
			if _, err := clusterModeFromEnv(); err == nil {
				t.Errorf("value %q was accepted; it must be a startup error", value)
			}
		})
	}
}

func TestClusterMode_AcceptsBothValidValues(t *testing.T) {
	for _, value := range []string{"single", "multi", "  MULTI  ", "Single"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(envClusterMode, value)
			mode, err := clusterModeFromEnv()
			if err != nil {
				t.Fatalf("value %q rejected: %v", value, err)
			}
			if mode != clusterModeSingle && mode != clusterModeMulti {
				t.Errorf("mode = %q", mode)
			}
		})
	}
}

// TestValidateStack_MultiRequiresPostgres pins the refusal production would
// need today: it runs on SQLite through the automatic fallback, with the
// Postgres variables PARTIALLY set. In `multi` that cannot be a warning.
func TestValidateStack_MultiRequiresPostgres(t *testing.T) {
	if err := validateStackForMode(clusterModeMulti, databaseTypeSQLite); err == nil {
		t.Fatal("multi with sqlite was accepted; it must be a fatal startup error")
	} else if !strings.Contains(err.Error(), "DB_USER") {
		// The message has to say what to do. An error that only reports failure
		// forces the operator to read the source.
		t.Errorf("message does not name the variables to set: %q", err)
	}
	if err := validateStackForMode(clusterModeMulti, databaseTypePostgres); err != nil {
		t.Errorf("multi with postgres rejected: %v", err)
	}
}

// TestValidateStack_SingleAcceptsBoth: the catastrophic scenario is a supported
// mode. SQLite with one pod has to pass — it is the default, not the exception.
func TestValidateStack_SingleAcceptsBoth(t *testing.T) {
	for _, databaseType := range []string{databaseTypeSQLite, databaseTypePostgres} {
		if err := validateStackForMode(clusterModeSingle, databaseType); err != nil {
			t.Errorf("single with %s rejected: %v", databaseType, err)
		}
	}
}

// TestInstanceLock_SecondAttemptFails reproduces, as a test, the scenario from
// F89's experiment 1: two processes over the same installation.
//
// Measured there: WhatsApp kills one, the loser's session stays dead and never
// reconnects, and the database keeps reporting `connected=1`. Here the second
// process does not even start.
func TestInstanceLock_SecondAttemptFails(t *testing.T) {
	dir := t.TempDir()

	release, err := lockSingleInstance(dir)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer release()

	// A second attempt IN THE SAME PROCESS uses a different file descriptor,
	// which is what flock distinguishes — the same thing a second process sees.
	if _, err := lockSingleInstance(dir); err == nil {
		t.Fatal("a second lock over the same directory was granted; the F89 zombie is still possible")
	} else if !strings.Contains(err.Error(), envClusterMode) {
		t.Errorf("message does not point at the way out (%s=multi): %q", envClusterMode, err)
	}
}

// TestInstanceLock_ReleaseAllowsAnother: the lock must not become a permanent
// padlock. A graceful restart has to start again — and F89 measured session
// re-establishment at 1–2s, so any delay here would be noticeable.
func TestInstanceLock_ReleaseAllowsAnother(t *testing.T) {
	dir := t.TempDir()

	release, err := lockSingleInstance(dir)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	release()

	secondRelease, err := lockSingleInstance(dir)
	if err != nil {
		t.Fatalf("after releasing, the lock was denied: %v", err)
	}
	secondRelease()
}

// TestInstanceLock_DistinctDirectoriesDoNotConflict: two INSTALLATIONS on the
// same machine are legitimate — that is exactly how F89's validation ran, with
// production in one directory and the test environment in another.
func TestInstanceLock_DistinctDirectoriesDoNotConflict(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()

	releaseFirst, err := lockSingleInstance(first)
	if err != nil {
		t.Fatalf("lock on the first directory failed: %v", err)
	}
	defer releaseFirst()

	releaseSecond, err := lockSingleInstance(second)
	if err != nil {
		t.Fatalf("lock on the second directory denied because of the first; distinct installations must not conflict: %v", err)
	}
	defer releaseSecond()
}

func TestInstanceLock_CreatesTheDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")
	release, err := lockSingleInstance(dir)
	if err != nil {
		t.Fatalf("did not create the data directory: %v", err)
	}
	defer release()
	if _, err := os.Stat(filepath.Join(dir, instanceLockFile)); err != nil {
		t.Errorf("lock file does not exist: %v", err)
	}
}

// TestPrepareCluster_MultiDoesNotLock: in `multi` exclusivity is per SESSION
// (the D2 lease), not per installation. Locking the installation there would
// stop the second replica from starting, which is the whole point of the mode.
func TestPrepareCluster_MultiDoesNotLock(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeMulti)
	dir := t.TempDir()

	firstRelease, err := prepareCluster(dir, databaseTypePostgres)
	if err != nil {
		t.Fatalf("multi+postgres rejected: %v", err)
	}
	defer firstRelease()

	secondRelease, err := prepareCluster(dir, databaseTypePostgres)
	if err != nil {
		t.Fatalf("a second replica in multi was blocked by the installation lock: %v", err)
	}
	defer secondRelease()
}

func TestPrepareCluster_SingleLocks(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeSingle)
	dir := t.TempDir()

	release, err := prepareCluster(dir, databaseTypeSQLite)
	if err != nil {
		t.Fatalf("single+sqlite rejected; it is the supported catastrophic scenario: %v", err)
	}
	defer release()

	if _, err := prepareCluster(dir, databaseTypeSQLite); err == nil {
		t.Error("a second process in single mode was accepted")
	}
}

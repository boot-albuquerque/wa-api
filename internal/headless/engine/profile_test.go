package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const testHost = "headless-pod-7"

// writeSingletons builds a profile in the shape Chromium leaves behind.
//
// The lock is a SYMLINK whose target is "<hostname>-<pid>" and which points at
// nothing — that is how chrome/browser/process_singleton_posix.cc writes it.
// The double has to imitate that exact rule: a double that wrote a regular file
// would let a Stat-then-Open implementation pass, and against a real profile
// that implementation reports "no such file" for a lock that is right there.
func writeSingletons(t *testing.T, dir, lockTarget string) {
	t.Helper()
	if lockTarget != "" {
		if err := os.Symlink(lockTarget, filepath.Join(dir, singletonLock)); err != nil {
			t.Fatalf("symlink %s: %v", singletonLock, err)
		}
	}
	for _, name := range []string{singletonCookie, singletonSocket} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func remaining(t *testing.T, dir string) []string {
	t.Helper()
	var left []string
	for _, name := range singletonFiles {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			left = append(left, name)
		}
	}
	return left
}

func deadProcess(int) bool { return false }
func liveProcess(int) bool { return true }

// The happy path of invariant 15: the previous browser is gone, so its files go
// too. Chromium refuses to boot while they exist.
func TestReclaimRemovesStaleSingletonsOfADeadHolder(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, fmt.Sprintf("%s-424242", testHost))

	res, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost, ProcessAlive: deadProcess})
	if err != nil {
		t.Fatalf("ReclaimProfile: %v", err)
	}
	if len(res.Removed) != 3 {
		t.Fatalf("removed %v, want all three", res.Removed)
	}
	if left := remaining(t, dir); left != nil {
		t.Fatalf("still present: %v — the next boot will refuse", left)
	}
	if res.Reason == "" {
		t.Error("a reclaim with no stated reason is a silent delete")
	}
}

// The failure this code exists to prevent. Deleting a live browser's lock puts
// two browsers on one profile, which destroys the session for real — and the
// study's own section 18 says the reclaim is only legitimate under exclusive
// ownership.
func TestReclaimRefusesWhileTheHolderIsAlive(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, fmt.Sprintf("%s-31337", testHost))

	_, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost, ProcessAlive: liveProcess})
	if !errors.Is(err, ErrProfileHeldByLiveBrowser) {
		t.Fatalf("got %v, want ErrProfileHeldByLiveBrowser", err)
	}
	if left := remaining(t, dir); len(left) != 3 {
		t.Fatalf("files were removed anyway: %v remain, want all three intact", left)
	}
}

// No probe means no proof of death. Guessing "probably dead" is the one guess
// this function must never make.
func TestReclaimRefusesWithoutALivenessProbe(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, fmt.Sprintf("%s-999", testHost))

	_, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost})
	if !errors.Is(err, ErrProfileHeldByLiveBrowser) {
		t.Fatalf("got %v, want a refusal: a nil probe cannot prove the holder is gone", err)
	}
	if left := remaining(t, dir); len(left) != 3 {
		t.Fatalf("files were removed without any proof of death: %v remain", left)
	}
}

// A lock from another host cannot be judged from here. Refusing is loud; the
// alternative is silent and permanent.
func TestReclaimRefusesAForeignHostByDefault(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, "some-other-pod-4321")

	_, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost, ProcessAlive: deadProcess})
	if !errors.Is(err, ErrProfileHeldByAnotherHost) {
		t.Fatalf("got %v, want ErrProfileHeldByAnotherHost", err)
	}
	if left := remaining(t, dir); len(left) != 3 {
		t.Fatalf("a foreign host's lock was removed: %v remain", left)
	}
}

func TestReclaimAllowsAForeignHostWhenAuthorised(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, "some-other-pod-4321")

	res, err := ReclaimProfile(dir, ReclaimOptions{
		Hostname: testHost, ProcessAlive: deadProcess, AllowForeignHost: true,
	})
	if err != nil {
		t.Fatalf("ReclaimProfile: %v", err)
	}
	if len(res.Removed) != 3 {
		t.Fatalf("removed %v, want all three", res.Removed)
	}
	// The override must be visible afterwards; "the boot worked" and "the boot
	// worked because we deleted somebody else's lock" cannot read the same.
	if res.Reason == "" || res.LockHolder != "some-other-pod-4321" {
		t.Errorf("the override left no trace: %+v", res)
	}
}

// A hostname may contain hyphens, so the pid is after the LAST one. Splitting
// on the first would read the pod name as a pid and never parse.
func TestLockHolderSplitsOnTheLastHyphen(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, "headless-pod-7-8899")

	got, err := readLockHolder(dir)
	if err != nil {
		t.Fatalf("readLockHolder: %v", err)
	}
	if !got.parsed {
		t.Fatal("a well-formed lock did not parse")
	}
	if got.host != "headless-pod-7" || got.pid != 8899 {
		t.Fatalf("parsed host=%q pid=%d, want host=%q pid=8899", got.host, got.pid, "headless-pod-7")
	}
}

// An unreadable lock proves nothing about a holder, and Chromium will not boot
// while it exists. Removing it is the only way forward — but it must say so.
func TestReclaimClearsAnUnreadableLock(t *testing.T) {
	dir := t.TempDir()
	writeSingletons(t, dir, "")
	if err := os.WriteFile(filepath.Join(dir, singletonLock), []byte("not a symlink"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	res, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost, ProcessAlive: liveProcess})
	if err != nil {
		t.Fatalf("ReclaimProfile: %v", err)
	}
	if len(res.Removed) != 3 {
		t.Fatalf("removed %v, want all three", res.Removed)
	}
	if res.Reason == "" {
		t.Error("an unprovable removal with no stated reason is a silent delete")
	}
}

// A profile that has never booted has nothing to reclaim, and saying "error" to
// a first boot would block every new account.
func TestReclaimOnAFreshProfileIsNotAnError(t *testing.T) {
	dir := t.TempDir()

	res, err := ReclaimProfile(dir, ReclaimOptions{Hostname: testHost, ProcessAlive: deadProcess})
	if err != nil {
		t.Fatalf("ReclaimProfile on a fresh profile: %v", err)
	}
	if len(res.Removed) != 0 {
		t.Fatalf("removed %v from a profile that never booted", res.Removed)
	}
}

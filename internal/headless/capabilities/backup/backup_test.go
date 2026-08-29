package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// profileFixture builds a directory shaped like a profile: nested dirs, regular
// files, and — when live is true — the SingletonLock SYMLINK Chromium writes,
// whose target is "<host>-<pid>".
//
// The lock is a real symlink to a real pid, not a plain file, because that is
// what engine.ProfileInUse reads. A fixture writing a regular file there would
// be more permissive than production: Readlink would fail, the holder would
// parse as absent, and the in-use check would say "free" — the double being
// better behaved than the world (ARMADILHAS §1).
func profileFixture(t *testing.T, live bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Default", "Sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Local State":                                       `{"profile":{}}`,
		filepath.Join("Default", "Cookies"):                 "cookie-bytes",
		filepath.Join("Default", "Sessions", "Session_123"): "session-bytes",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if live {
		// os.Getpid() is alive by definition — this process.
		target := "testhost-" + strconv.Itoa(os.Getpid())
		if err := os.Symlink(target, filepath.Join(dir, "SingletonLock")); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBackupCopiesAStoppedProfile(t *testing.T) {
	src := profileFixture(t, false)
	dst := filepath.Join(t.TempDir(), "copy")

	got, err := Backup(src, dst)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if got.Files != 3 {
		t.Fatalf("Files=%d, want 3", got.Files)
	}
	if got.Bytes == 0 {
		t.Fatal("Bytes=0 on a copy that reported files")
	}
	for _, name := range []string{
		"Local State",
		filepath.Join("Default", "Cookies"),
		filepath.Join("Default", "Sessions", "Session_123"),
	} {
		body, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatalf("%s missing from the copy: %v", name, err)
		}
		if len(body) == 0 {
			t.Fatalf("%s is empty in the copy", name)
		}
	}
}

// TestLiveProfileIsRefused is the capability. Measured: 11 of 1124 files change
// in 10s of an IDLE live session, so a copy taken now is corrupt-looking-fine.
func TestLiveProfileIsRefused(t *testing.T) {
	src := profileFixture(t, true)
	dst := filepath.Join(t.TempDir(), "copy")

	_, err := Backup(src, dst)
	if !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("err=%v, want ErrProfileInUse: a copy of a live profile catches files "+
			"mid-write and the corruption only shows up on restore", err)
	}
	// And nothing may be left behind: a half-written directory is worse than no
	// directory, because someone finds it later and trusts it.
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("the refused backup left %s behind (stat err=%v)", dst, statErr)
	}
}

// TestSingletonLockIsNotCopied: the lock is a symlink naming a pid, and copying
// it into a restored profile carries a claim about a process that does not
// exist there — the stale-lock condition ReclaimProfile exists to clean up.
func TestSingletonLockIsNotCopied(t *testing.T) {
	src := profileFixture(t, false)
	// A dangling lock: present, but naming a pid that is not running. InUse()
	// must not refuse on it, and the copy must not carry it.
	if err := os.Symlink("otherhost-999999", filepath.Join(src, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")

	got, err := Backup(src, dst)
	if err != nil {
		t.Fatalf("Backup refused a profile whose lock names a dead pid on another host: %v", err)
	}
	if got.Skipped == 0 {
		t.Fatal("Skipped=0: the Singleton symlink was not recognised as a non-regular file")
	}
	if _, err := os.Lstat(filepath.Join(dst, "SingletonLock")); !os.IsNotExist(err) {
		t.Fatal("the copy carries a SingletonLock; a restored profile would start with a " +
			"lock naming a process that never existed there")
	}
}

func TestNonEmptyDestinationIsRefused(t *testing.T) {
	src := profileFixture(t, false)
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "leftover"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Backup(src, dst)
	if !errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("err=%v, want ErrDestinationNotEmpty: merging two profiles produces "+
			"neither, and the failure shows up as a session that boots and misbehaves", err)
	}
}

func TestEmptySourceIsAnError(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	_, err := Backup(src, dst)
	if err == nil {
		t.Fatal("copying nothing reported success; a wrong source path would look like a " +
			"working backup")
	}
	if errors.Is(err, ErrProfileInUse) || errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("the empty-source failure was reported as something else: %v", err)
	}
}

func TestSameDirectoryIsRefused(t *testing.T) {
	src := profileFixture(t, false)
	if _, err := Backup(src, src); err == nil {
		t.Fatal("backing a profile up onto itself reported success")
	}
}

// TestVerifyRequiresAnActualBoot pins the contract that a file count is not
// proof. A backup nobody tried to restore is unverified by definition.
func TestVerifyRequiresAnActualBoot(t *testing.T) {
	if err := Verify(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("Verify accepted a nil boot function and reported success")
	}
	bootErr := errors.New("the copy did not reach READY")
	err := Verify(context.Background(), t.TempDir(), func(context.Context, string) error {
		return bootErr
	})
	if !errors.Is(err, bootErr) {
		t.Fatalf("err=%v, want it to wrap the boot failure", err)
	}
	if err := Verify(context.Background(), t.TempDir(), func(context.Context, string) error {
		return nil
	}); err != nil {
		t.Fatalf("a successful boot reported %v", err)
	}
}

// failingCloser writes fine and fails on Close, which is how a buffered write
// really fails: the bytes go into a buffer, the flush happens at Close, and the
// disk being full or the mount going away surfaces THERE and nowhere earlier.
type failingCloser struct {
	w         io.Writer
	closeErr  error
	closed    bool
	writeErr  error
	bytesSeen int
}

func (f *failingCloser) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.bytesSeen += len(p)
	return f.w.Write(p)
}

func (f *failingCloser) Close() error {
	f.closed = true
	return f.closeErr
}

// TestCloseErrorFailsTheBackup closes H27: the guard existed and nothing
// exercised it, so turning it off did not fail the suite. That is the
// definition of an untested guard, and this module treats those as debt.
func TestCloseErrorFailsTheBackup(t *testing.T) {
	src := profileFixture(t, false)
	dst := filepath.Join(t.TempDir(), "copy")
	boom := errors.New("disk full on flush")

	var opened []*failingCloser
	open := func(path string) (io.WriteCloser, error) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, err
		}
		fc := &failingCloser{w: f, closeErr: boom}
		opened = append(opened, fc)
		return fc, nil
	}

	_, err := backupWith(src, dst, open)
	if err == nil {
		t.Fatal("a backup whose files failed to flush reported success; that is how a " +
			"truncated file ends up in a backup nobody doubts until they need it")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the Close failure", err)
	}
	if len(opened) == 0 {
		t.Fatal("the injected opener was never used; this test proves nothing")
	}
}

// TestCloseIsCalledEvenWhenTheCopyFails is the property the error check alone
// does NOT give. Returning early on a copy error would leave the descriptor
// open and skip the flush — and a test that only asserted the Close ERROR is
// surfaced would still pass, because it never reaches that path.
func TestCloseIsCalledEvenWhenTheCopyFails(t *testing.T) {
	src := profileFixture(t, false)
	dst := filepath.Join(t.TempDir(), "copy")
	copyBoom := errors.New("write failed mid-copy")

	var opened []*failingCloser
	open := func(path string) (io.WriteCloser, error) {
		fc := &failingCloser{w: io.Discard, writeErr: copyBoom}
		opened = append(opened, fc)
		return fc, nil
	}

	_, err := backupWith(src, dst, open)
	if err == nil {
		t.Fatal("a backup whose copy failed reported success")
	}
	if !errors.Is(err, copyBoom) {
		t.Fatalf("err = %v, want it to wrap the copy failure: a copy error explains a "+
			"close error, so it is the one to report", err)
	}
	if len(opened) == 0 {
		t.Fatal("the injected opener was never used")
	}
	for i, fc := range opened {
		if !fc.closed {
			t.Fatalf("destination %d was never closed after the copy failed: the descriptor "+
				"leaks and the flush that this guard exists for never happens", i)
		}
	}
}

// TestProductionPathStillUsesTheRealOpener guards the seam itself. An injection
// point that production stopped going through would make every test above
// measure something nothing runs.
func TestProductionPathStillUsesTheRealOpener(t *testing.T) {
	src := profileFixture(t, false)
	dst := filepath.Join(t.TempDir(), "copy")

	got, err := Backup(src, dst)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if got.Files == 0 {
		t.Fatal("Backup copied nothing through the production opener")
	}
	body, err := os.ReadFile(filepath.Join(dst, "Local State"))
	if err != nil || len(body) == 0 {
		t.Fatalf("the production path did not write real bytes: %v", err)
	}
}

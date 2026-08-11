package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// startFake runs a shell script as a stand-in for Chromium.
//
// It is started exactly the way the launcher must start the real thing —
// Setpgid — because SignalStop escalates to the process GROUP, and a test that
// skipped it would pass while the production escalation signalled this test
// binary's own group instead.
func startFake(t *testing.T, script string) *Browser {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake browser: %v", err)
	}
	b := newBrowser(cmd, "ws://127.0.0.1:0/devtools/browser/fake")
	t.Cleanup(func() {
		if b.PID() > 0 {
			_ = syscall.Kill(-b.PID(), syscall.SIGKILL)
		}
	})
	return b
}

func TestWaitExitReturnsWhenTheProcessLeaves(t *testing.T) {
	b := startFake(t, "exit 0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("WaitExit: %v", err)
	}
}

// A browser asked to close reports a non-zero status on the way out. Reading
// that as "it did not exit" would make CleanStop signal a process that already
// left — and label a clean stop dirty.
func TestWaitExitAcceptsANonZeroExitStatus(t *testing.T) {
	b := startFake(t, "exit 3")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("a non-zero exit was reported as a failure to exit: %v", err)
	}
}

// os.Process.Wait may be called only once, but CleanStop is not the only caller
// interested in the exit. The fact must survive being asked repeatedly, and
// concurrently. Run under -race.
func TestWaitExitIsRepeatableAndConcurrent(t *testing.T) {
	b := startFake(t, "exit 0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("first WaitExit: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = b.WaitExit(ctx)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent WaitExit %d: %v", i, err)
		}
	}
}

// The budget has to bind, or CleanStop's bound does not either.
func TestWaitExitRespectsTheBudget(t *testing.T) {
	b := startFake(t, "sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := b.WaitExit(ctx)
	if !errors.Is(err, ErrBrowserStillRunning) {
		t.Fatalf("got %v, want ErrBrowserStillRunning", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("WaitExit took %v on a 150ms budget", elapsed)
	}
}

func TestSignalStopTerminatesAnOrdinaryProcess(t *testing.T) {
	b := startFake(t, "sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.SignalStop(ctx); err != nil {
		t.Fatalf("SignalStop: %v", err)
	}
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("the process survived SIGTERM: %v", err)
	}
}

// The case that makes the fallback terminal.
//
// Chromium spawns renderers. SIGTERM to the parent alone leaves them orphaned,
// and an orphan keeps the profile lock, which turns one dirty stop into a
// session that cannot restart. The escalation must reach the whole GROUP.
func TestSignalStopKillsTheGroupWhenSigtermIsIgnored(t *testing.T) {
	childFile := filepath.Join(t.TempDir(), "child.pid")
	// The parent ignores SIGTERM, exactly like a browser wedged mid-write; the
	// child is the orphan-to-be.
	b := startFake(t, "trap '' TERM; sleep 30 & echo $! > "+childFile+"; wait")

	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(childFile)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
				childPID = pid
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		t.Skip("the fake browser never reported a child pid; nothing to assert about the group")
	}

	// A short budget stands in for signalGrace: SignalStop bounds the wait by
	// the caller's context, so the escalation happens here in ~200ms instead of
	// the production ten seconds.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := b.SignalStop(ctx); err != nil {
		t.Fatalf("SignalStop: %v", err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	if err := b.WaitExit(waitCtx); err != nil {
		t.Fatalf("the parent survived the escalation: %v", err)
	}

	// The child must be gone too. Poll: reaping is not instantaneous.
	gone := false
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		if syscall.Kill(childPID, syscall.Signal(0)) != nil {
			gone = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !gone {
		t.Fatalf("child %d outlived the escalation — an orphaned Chromium holds the "+
			"profile lock and blocks the next boot", childPID)
	}
}

// Signalling a process that already left is not harmless: pids get recycled,
// and the signal would land on whatever inherited the number.
func TestSignalStopOnAnExitedProcessIsANoop(t *testing.T) {
	b := startFake(t, "exit 0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("WaitExit: %v", err)
	}
	if err := b.SignalStop(ctx); err != nil {
		t.Fatalf("SignalStop on an exited process: %v", err)
	}
}

func TestPIDIsReported(t *testing.T) {
	b := startFake(t, "sleep 30")
	if b.PID() <= 0 {
		t.Fatal("no pid: the product's ChromiumSupervisor has nothing to register")
	}
}

// End to end for CAP-02 plus this loop, against a REAL process: the CDP command
// is what makes it leave, and the exit is what proves it.
//
// The fake browser kills its own process on Browser.close, which is what
// Chromium does. Without that the test would only prove the command was sent.
func TestCleanStopEndsARealProcessThroughTheProtocol(t *testing.T) {
	b := startFake(t, "sleep 30")

	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		if cmd.Method == methodBrowserClose {
			_ = syscall.Kill(-b.PID(), syscall.SIGKILL)
		}
		return cdpMessage{ID: cmd.ID, Result: json.RawMessage(`{}`)}, actionReply
	})
	b.wsURL = f.wsURL()

	via := CleanStop(context.Background(), shortShutdownRunner(), b)

	if via != StopViaBrowserClose {
		t.Fatalf("stopped via %q, want %q", via, StopViaBrowserClose)
	}
	if !via.Clean() {
		t.Error("a protocol stop of a real process must classify as clean")
	}
}

func TestProcessAliveSeesARunningProcess(t *testing.T) {
	b := startFake(t, "sleep 30")

	if !ProcessAlive(b.PID()) {
		t.Fatal("a running process was reported as gone; the reclaim would delete " +
			"a live browser's lock and put two browsers on one profile")
	}
}

func TestProcessAliveSeesAnExitedProcess(t *testing.T) {
	b := startFake(t, "exit 0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.WaitExit(ctx); err != nil {
		t.Fatalf("WaitExit: %v", err)
	}
	if ProcessAlive(b.PID()) {
		t.Fatal("a reaped process was reported as alive; every reclaim after a crash " +
			"would refuse and no profile would ever boot again")
	}
}

// The probe must not be a stop. A pid it reports on has to survive being asked.
func TestProcessAliveDoesNotDisturbTheProcess(t *testing.T) {
	b := startFake(t, "sleep 30")

	for i := 0; i < 5; i++ {
		if !ProcessAlive(b.PID()) {
			t.Fatalf("the process died after %d liveness probes — signal 0 must deliver nothing", i)
		}
	}
}

func TestProcessAliveRejectsNonsensePIDs(t *testing.T) {
	for _, pid := range []int{0, -1, -12345} {
		if ProcessAlive(pid) {
			t.Errorf("pid %d reported as alive", pid)
		}
	}
}

// EPERM means "the process exists and is someone else's", and reading it as
// "gone" would let the reclaim delete a live browser's lock.
//
// No process this test starts can produce EPERM — they all belong to it. pid 1
// does: it is init/launchd, owned by root, and signal 0 to it from an
// unprivileged process returns EPERM while the process is very much alive.
func TestProcessAliveTreatsEPERMAsAlive(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: signal 0 to pid 1 succeeds outright, so this run " +
			"cannot exercise the EPERM branch")
	}
	if err := syscall.Kill(1, syscall.Signal(0)); !errors.Is(err, syscall.EPERM) {
		t.Skipf("signal 0 to pid 1 returned %v, not EPERM; this platform cannot "+
			"exercise the branch here", err)
	}
	if !ProcessAlive(1) {
		t.Fatal("pid 1 reported as gone. EPERM means the process exists and belongs " +
			"to someone else; reading it as dead lets the reclaim delete a live " +
			"browser's lock and put two browsers on one profile")
	}
}

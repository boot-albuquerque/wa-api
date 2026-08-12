package engine

// Browser is a running Chromium, as this package needs to hold one: a CDP
// endpoint, a process to wait for, and a dirty way out.
//
// It is the concrete BrowserProcess that CAP-02 declared and left unimplemented
// so the shutdown ORDER could be tested against a double first.
//
// Study origin: scripts/chromium-study/p3_launcher.go, the `launched` type.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// signalGrace is how long SIGTERM gets before the process group is killed.
//
// Chromium spawns children, and SIGTERM to the parent alone leaves them
// orphaned. An orphaned Chromium keeps the profile lock and blocks the next
// boot, which turns one dirty stop into a session that cannot restart — so the
// escalation is not optional cleanup, it is what makes the fallback terminal.
const signalGrace = 10 * time.Second

// ErrBrowserStillRunning is returned by WaitExit when the budget ran out and
// the process is still there.
var ErrBrowserStillRunning = errors.New("browser did not exit within the budget")

// Browser owns one Chromium process.
//
// Safe for concurrent use, and it has to be: CleanStop waits on the exit while
// a liveness probe may be reading the endpoint, and os.Process.Wait may only be
// called once. A single reaper goroutine calls it and everyone else watches the
// channel it closes.
type Browser struct {
	cmd        *exec.Cmd
	wsURL      string
	profileDir string

	exited   chan struct{}
	waitOnce sync.Once
	mu       sync.Mutex
	waitErr  error
}

// newBrowser wraps an already-started command and begins reaping it.
//
// The command MUST have been started with Setpgid, because SignalStop escalates
// to the process group; without it the escalation would signal this process's
// own group. LaunchOptions.command sets it, and TestSignalStopKillsTheGroup
// would fail loudly if it stopped doing so.
func newBrowser(cmd *exec.Cmd, wsURL, profileDir string) *Browser {
	b := &Browser{cmd: cmd, wsURL: wsURL, profileDir: profileDir, exited: make(chan struct{})}
	go b.reap()
	return b
}

func (b *Browser) reap() {
	b.waitOnce.Do(func() {
		err := b.cmd.Wait()
		b.mu.Lock()
		b.waitErr = err
		b.mu.Unlock()
		close(b.exited)
	})
}

// WebSocketURL is the browser-level CDP endpoint.
func (b *Browser) WebSocketURL() string { return b.wsURL }

// ProfileDir is where this browser's session lives, or "" when it holds none.
//
// CleanStop uses it to mark a dirty stop, so a Browser built without it stops
// silently losing that record — which is why the launcher always passes it.
func (b *Browser) ProfileDir() string { return b.profileDir }

// PID is the Chromium process id, or 0 once it is gone.
//
// The product's ChromiumSupervisor consumes this (getBrowserPid in the
// WaClientAdapter surface), so it is not diagnostics — it is a capability.
func (b *Browser) PID() int {
	if b.cmd == nil || b.cmd.Process == nil {
		return 0
	}
	return b.cmd.Process.Pid
}

// WaitExit blocks until the process is gone or ctx expires.
//
// A process that already exited returns immediately, however many times it is
// asked: the exit is a fact, not an event that can be consumed.
func (b *Browser) WaitExit(ctx context.Context) error {
	select {
	case <-b.exited:
		b.mu.Lock()
		defer b.mu.Unlock()
		// A non-zero exit status is how a browser we asked to close reports
		// itself. It is not a failure to exit, and treating it as one would
		// send a signal to a process that already left.
		var exitErr *exec.ExitError
		if errors.As(b.waitErr, &exitErr) {
			return nil
		}
		return b.waitErr
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrBrowserStillRunning, ctx.Err())
	}
}

// SignalStop is the DIRTY fallback: SIGTERM, then SIGKILL to the whole group.
//
// It corrupts session state — phase 4C measured the profile shrinking from 123
// to 118 MB at exactly the first degradation, and remote invalidation does not
// reduce local storage. Nothing should call it except CleanStop, after the
// protocol path has been given its chance and the process has been given time
// to leave on its own.
func (b *Browser) SignalStop(ctx context.Context) error {
	if b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	select {
	case <-b.exited:
		return nil // already gone; signalling a corpse only risks a recycled pid
	default:
	}

	if err := b.cmd.Process.Signal(syscall.SIGTERM); err != nil { //ablation:stop-form
		if !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("SIGTERM: %w", err)
		}
		return nil
	}

	grace, cancel := context.WithTimeout(ctx, signalGrace)
	defer cancel()
	select {
	case <-b.exited:
		return nil
	case <-grace.Done():
	}

	// The escalation targets the GROUP (negative pid). Killing only the parent
	// orphans the renderers, and an orphan holds the profile lock.
	if err := syscall.Kill(-b.cmd.Process.Pid, syscall.SIGKILL); err != nil { //ablation:stop-form
		return fmt.Errorf("SIGKILL of process group: %w", err)
	}
	return nil
}

// ProcessAlive reports whether a pid is running on this host.
//
// It is the liveness probe ReclaimProfile requires and refuses to work without:
// deleting a live browser's SingletonLock puts two browsers on one profile,
// which is how a paired session is destroyed for real (study section 18).
//
// It lives here because this is the layer that knows about processes, and it is
// a plain function so the reclaim never has to construct a Browser to ask a
// question about a pid it read off a lock file.
//
// Signal 0 delivers NOTHING — it asks the kernel whether the pid exists. It is
// not a shutdown path and carries no ablation marker, and the static gate knows
// the difference: see deliversNothing in shutdown_policy_test.go.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists and belongs to someone else. Reading that
	// as "gone" would let the reclaim delete a live browser's lock.
	return errors.Is(err, syscall.EPERM)
}

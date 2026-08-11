package engine

// Shutting a browser down is a credential operation, not a process operation.
//
// Phase 4C measured that stopping Chromium by signal corrupts the WhatsApp
// session state: the profile shrank from 123 to 118 MB at exactly the first
// degradation, and remote invalidation does not reduce local storage. The clean
// path is Browser.close over CDP, which is the shutdown Chromium understands as
// clean and during which it flushes IndexedDB.
//
// The requirement was written in the phase 4C report and then stayed inside the
// experiment that measured it. Five modes of the study kept stopping by signal,
// and the symptom came back a phase later as Singleton files left in the
// profile — logged as F94 in the root HOUSEKEEP.md. The static gate in this
// package exists so that this port does not repeat it.
//
// Study origin: scripts/chromium-study/p4c_lifecycle.go.

import (
	"context"
)

// methodBrowserClose is the only CDP method this package sends outside a
// driver. Named, because a literal repeated in two places is the same bug
// waiting to diverge.
const methodBrowserClose = "Browser.close"

// CloseBrowserViaCDP asks the browser to shut itself down, over a connection
// this package owns.
//
// A returned error means the command's fate is unknown, NOT that the browser is
// still running: Chromium was measured replying in 2 ms and then dropping the
// socket, so a lost reply and a refusal are indistinguishable from here. The
// caller decides the outcome by watching the process exit, never by believing
// this reply — the same rule that separates "answered the protocol" from "the
// application is running" one layer up.
func CloseBrowserViaCDP(ctx context.Context, wsURL string) error {
	conn, err := dialBrowser(ctx, wsURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.call(ctx, methodBrowserClose, nil)
	return err
}

// BrowserProcess is the little this package needs to know about a running
// browser in order to stop it: where to reach it, how to wait for it, and the
// dirty way out.
//
// It is an interface rather than a struct so that the ORDER in CleanStop can be
// tested against a double. That is not convenience: the defect this code exists
// to prevent is a call site reaching for the signal, and a shutdown whose
// sequence is only exercised against a real browser is a sequence nobody checks.
type BrowserProcess interface {
	// WebSocketURL is the browser-level CDP endpoint.
	WebSocketURL() string
	// WaitExit blocks until the process is gone or ctx expires.
	WaitExit(ctx context.Context) error
	// SignalStop is the DIRTY fallback. It is on this interface because it must
	// exist, and named so that no call site reaches for it by accident.
	SignalStop(ctx context.Context) error
}

// StopVia labels how a browser actually went down.
//
// The label is returned rather than assumed because the study lost an entire
// comparison run to a silent fallback: an arm that was supposed to stop by
// protocol fell back to a signal and became a replica of the arm it was being
// compared against. A stop whose form nobody records is that trap waiting again.
type StopVia string

const (
	// StopViaNoop is nothing to stop.
	StopViaNoop StopVia = "noop"
	// StopViaBrowserClose is the clean path: acknowledged, then exited.
	StopViaBrowserClose StopVia = "browser.close"
	// StopViaBrowserCloseUnconfirmed is the clean path with a lost reply. The
	// process left on its own within budget, which is the evidence that counts;
	// the acknowledgement is not.
	StopViaBrowserCloseUnconfirmed StopVia = "browser.close_unconfirmed"
	// StopViaDirtySignalCloseRefused is a signal sent after the protocol
	// refused. Dirty by name: this is the shutdown that corrupts session state.
	StopViaDirtySignalCloseRefused StopVia = "DIRTY_signal_close_refused"
	// StopViaDirtySignalExitTimeout is a signal sent because the process did
	// not leave after being asked.
	StopViaDirtySignalExitTimeout StopVia = "DIRTY_signal_after_close_timeout"
)

// Clean reports whether a stop preserved session state.
func (s StopVia) Clean() bool {
	return s == StopViaBrowserClose || s == StopViaBrowserCloseUnconfirmed || s == StopViaNoop
}

const (
	opLabelClose    = "shutdown/browser.close"
	opLabelWaitExit = "shutdown/wait-exit"
	opLabelSignal   = "shutdown/signal"
)

// CleanStop is the shutdown every path that touches a paired profile must use.
//
// The sequence is the contract:
//
//  1. ask over CDP;
//  2. WAIT for the process to leave — the exit is the verdict, not the reply;
//  3. only then, and only if it did not leave, fall back to the signal.
//
// Step 2 is why this is a separate concern from CloseBrowserViaCDP. Without the
// wait, a signal following immediately behind the command would give every stop
// the dirty shutdown underneath, and the two paths would be indistinguishable
// by construction — which is exactly how the study's browserclose arm turned
// into a replica of its own control.
//
// It diverges deliberately from the study here: p4c_lifecycle.go signalled
// immediately when the CDP command errored. This waits first, because the
// browser was measured replying in 2 ms and then dropping the socket, so a lost
// reply is the ordinary case rather than a refusal, and signalling on it would
// corrupt the very session the clean path just saved.
//
// Bounded cost: at most one Shutdown budget for steps 1-2 and one more for the
// fallback. It blocks the caller for that long, so it must not run inside a
// limited slot — nothing that waits on a clock or a dead peer may occupy one.
func CleanStop(parent context.Context, r *Runner, p BrowserProcess) StopVia {
	if p == nil {
		return StopViaNoop
	}
	if r == nil {
		r = NewRunner()
	}

	closeErr := r.Do(parent, OpShutdown, opLabelClose, func(ctx context.Context) error {
		return CloseBrowserViaCDP(ctx, p.WebSocketURL())
	})

	exitErr := r.Do(parent, OpShutdown, opLabelWaitExit, func(ctx context.Context) error {
		return p.WaitExit(ctx)
	})
	if exitErr == nil {
		if closeErr == nil {
			return StopViaBrowserClose
		}
		return StopViaBrowserCloseUnconfirmed
	}

	_ = r.Do(parent, OpShutdown, opLabelSignal, func(ctx context.Context) error {
		return p.SignalStop(ctx) //ablation:stop-form
	})
	if closeErr != nil {
		return StopViaDirtySignalCloseRefused
	}
	return StopViaDirtySignalExitTimeout
}

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

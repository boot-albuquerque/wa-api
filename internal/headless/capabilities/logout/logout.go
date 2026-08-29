// Package logout deauthenticates a live wa_headless session — the
// counterpart to a phone owner choosing "Log out" under Linked Devices.
//
// # Where the call comes from, and what was measured
//
// whatsapp-web.js (src/Client.js, Client.logout, fetched 2026-08-29 from
// github.com/pedroslopez/whatsapp-web.js@main):
//
//	window.require('WAWebSocketModel').Socket.logout()
//
// Zero arguments — the reference's surrounding steps (closing its own
// puppeteer browser, waiting up to 1s) are specific to ITS process
// lifecycle, not to the page call itself.
//
// H122 (internal/wa-headless/HOUSEKEEP.md, 2026-08-22) had only confirmed
// Socket.logout EXISTS and is a function — refusing to CALL it on policy
// grounds, since it deauthenticates a real account and only a human with
// the phone can restore it. That policy was revisited on explicit request
// (F380/this package), and the call was finally MEASURED — not guessed —
// against a genuinely paired, disposable profile
// (TestProbeSocketLogout, internal/wa-headless/probe_logout_test.go):
//
//	BEFORE: socket=CONNECTED, hasOwner=true
//	Socket.logout() returns without throwing
//	AFTER (~18s): socket=UNPAIRED, a fresh working QR is showing,
//	              the page never crashed or hung
//
// The page does not confirm the logout synchronously — the transition from
// CONNECTED to UNPAIRED takes several seconds and this package does not
// wait for it (invariant 6: no page clock, and the caller's own next
// status poll will observe the new state naturally, the same way it
// already observes any other async change on this page). Do only reports
// whether the CALL itself was accepted by the page.
package logout

import (
	"context"
	"encoding/json"
	"fmt"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// ErrLogout is the page refusing or throwing on the call itself.
var ErrLogout = fmt.Errorf("logout: the page refused Socket.logout()")

const script = `(() => {
	try {
		window.require('WAWebSocketModel').Socket.logout();
		return JSON.stringify({ok: true});
	} catch (e) {
		return JSON.stringify({ok: false, why: String((e && e.message) || e).slice(0, 200)});
	}
})()`

type wireResult struct {
	OK  bool   `json:"ok"`
	Why string `json:"why"`
}

// Do fires Socket.logout() and reports whether the page accepted the call.
func Do(ctx context.Context, runner *engine.Runner, eval spa.Evaluator, label string) error {
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/logout", func(c context.Context) error {
		return eval(c, script, &raw)
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLogout, err)
	}
	var out wireResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return fmt.Errorf("logout: unexpected answer shape: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("%w: %s", ErrLogout, out.Why)
	}
	return nil
}

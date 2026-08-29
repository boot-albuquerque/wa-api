package headless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeSocketLogout measures Socket.logout()'s real effect against a
// PAIRED, disposable profile — the exact gap H122 left open
// (internal/wa-headless/HOUSEKEEP.md): the function was confirmed to EXIST
// and to be a function, but the call itself was never invoked, on POLICY
// grounds (it deauthenticates the account for real, and only a human with
// the phone can restore it). This probe is the one deliberate exception,
// against a profile that exists specifically for this
// (commit 682b5921 — "dois slots de perfil descartável").
//
// Call shape MEASURED from the reference implementation, not guessed
// (whatsapp-web.js, src/Client.js, Client.logout, fetched 2026-08-29 from
// github.com/pedroslopez/whatsapp-web.js@main):
//
//	window.require('WAWebSocketModel').Socket.logout()
//
// Zero arguments, no sequencing beyond the call itself — the reference's
// own surrounding steps (closing the puppeteer browser, waiting up to 1s)
// are specific to ITS process-lifecycle model, not to the page call.
func TestProbeSocketLogout(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LOGOUT") == "" {
		t.Skip("set WA_PROBE_LOGOUT=1 — DESTRUCTIVE: deauthenticates the profile's account for real")
	}
	profile := os.Getenv("WA_HEADLESS_PROFILE_DIR")
	if profile == "" {
		t.Fatal("WA_HEADLESS_PROFILE_DIR is required — point it at a disposable, PAIRED profile you are willing to deauthenticate")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// PairingSession (not Session/core.StartSession): the strict boot path
	// refuses anything not ALREADY APP_READY at the moment it checks, and a
	// restored session with real chat history can take longer to settle
	// than a blank one — measured: both disposable profiles reported
	// PAIRING_LOADING on the strict path. The tolerant boot lets this probe
	// poll for CONNECTED itself, with its own budget, instead of failing on
	// the first snapshot.
	sess, err := h.PairingSession(ctx)
	if err != nil {
		t.Fatalf("boot (pairing-tolerant): %v", err)
	}
	eval := sess.Tab().Evaluate

	// The page needs time to settle — poll for the socket to report
	// CONNECTED (a genuinely paired, restored session) instead of assuming
	// the first read is final.
	settleDeadline := time.Now().Add(45 * time.Second)
	var lastSocket string
	for {
		var state string
		if err := eval(ctx, `(() => {
			try {
				const S = window.require('WAWebSocketModel');
				const sk = S && (S.Socket || S.default || S);
				return (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
			} catch (e) { return ""; }
		})()`, &state); err != nil {
			t.Fatalf("poll socket state while settling: %v", err)
		}
		if state != lastSocket {
			t.Logf("socket state: %q (t+%s)", state, 45*time.Second-time.Until(settleDeadline))
			lastSocket = state
		}
		if state == "CONNECTED" {
			break
		}
		if time.Now().After(settleDeadline) {
			t.Fatalf("socket state never reached CONNECTED within 45s (last seen: %q) — "+
				"this profile does not look paired right now; refusing to proceed with a "+
				"destructive logout against a session that may already be logged out", lastSocket)
		}
		time.Sleep(1 * time.Second)
	}

	// BEFORE: confirm this really is a paired, connected session — the
	// measurement is worthless against a profile that was not actually
	// paired.
	var before string
	if err := eval(ctx, `(() => {
		const out = { socket: "", hasOwner: false };
		try {
			const S = window.require('WAWebSocketModel');
			const sk = S && (S.Socket || S.default || S);
			out.socket = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
		} catch (e) {}
		try {
			out.hasOwner = !!window.require('WAWebSignalStoreApi');
		} catch (e) {}
		return JSON.stringify(out);
	})()`, &before); err != nil {
		t.Fatalf("read before-state: %v", err)
	}
	t.Logf("BEFORE logout: %s", before)

	var beforeParsed struct {
		Socket string `json:"socket"`
	}
	_ = json.Unmarshal([]byte(before), &beforeParsed)
	if beforeParsed.Socket != "CONNECTED" {
		t.Fatalf("socket state = %q before logout, want CONNECTED — this profile does not look paired; "+
			"refusing to proceed (the measurement needs a genuinely paired session)", beforeParsed.Socket)
	}

	// THE CALL.
	var logoutRaw string
	if err := eval(ctx, `(() => {
		try {
			window.require('WAWebSocketModel').Socket.logout();
			return "called";
		} catch (e) {
			return "error: " + String((e && e.message) || e);
		}
	})()`, &logoutRaw); err != nil {
		t.Fatalf("Socket.logout(): %v", err)
	}
	t.Logf("Socket.logout() returned: %q", logoutRaw)

	// AFTER: poll for a few seconds to see what the page settles into.
	deadline := time.Now().Add(20 * time.Second)
	var after string
	for {
		if err := eval(ctx, `(() => {
			const out = { socket: "", hasQR: false, testids: [] };
			try {
				const S = window.require('WAWebSocketModel');
				const sk = S && (S.Socket || S.default || S);
				out.socket = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
			} catch (e) {}
			try {
				const conn = window.require('WAWebConnModel');
				out.hasQR = !!(conn && conn.Conn && conn.Conn.ref);
			} catch (e) {}
			try {
				const nodes = document.querySelectorAll('[data-testid]');
				const seen = new Set();
				nodes.forEach(n => seen.add(n.getAttribute('data-testid')));
				out.testids = Array.from(seen).sort();
			} catch (e) {}
			return JSON.stringify(out);
		})()`, &after); err != nil {
			t.Fatalf("read after-state: %v", err)
		}
		t.Logf("AFTER logout (t+%s): %s", time.Since(deadline.Add(-20*time.Second)).Round(time.Second), after)
		var afterParsed struct {
			Socket string `json:"socket"`
		}
		_ = json.Unmarshal([]byte(after), &afterParsed)
		if afterParsed.Socket != "" && afterParsed.Socket != "CONNECTED" {
			t.Logf("MEASURED: socket state changed to %q after Socket.logout()", afterParsed.Socket)
			break
		}
		if time.Now().After(deadline) {
			t.Logf("MEASURED: socket state still %q after 20s — logout may not have taken effect, or the state model reports differently than expected", afterParsed.Socket)
			break
		}
		time.Sleep(2 * time.Second)
	}
}

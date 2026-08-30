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

// TestProbeRequestPairingCode measures requestPairingCode's real call shape
// against an UNPAIRED session — the exact gap H122 left open
// (internal/headless/HOUSEKEEP.md): the module surface was confirmed
// present, but the call itself was only ever exercised against a PAIRED lab
// session (socket state CONNECTED), where the reference's own state gate
// stops it before anything runs. Never invoked, only probed for presence.
//
// Call shape MEASURED from the reference implementation, not guessed
// (whatsapp-web.js, src/Client.js, requestPairingCode, fetched
// 2026-08-29 from github.com/pedroslopez/whatsapp-web.js@main):
//
//	window.require('WAWebAltDeviceLinkingApi').setPairingType('ALT_DEVICE_LINKING');
//	await window.require('WAWebAltDeviceLinkingApi').initializeAltDeviceLinking();
//	return window.require('WAWebAltDeviceLinkingApi').startAltLinkingFlow(phoneNumber, showNotification);
//
// The reference also arms a re-generation interval (window.codeInterval,
// default 180000ms) that keeps calling the same three-step chain as long as
// Socket.state stays UNPAIRED/UNPAIRED_IDLE — this probe does not exercise
// that part, only the single request/response.
func TestProbeRequestPairingCode(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PAIRPHONE") == "" {
		t.Skip("set WA_PROBE_PAIRPHONE=1 (needs an UNPAIRED profile — a paired one will report the reference's own state-gate refusal, which is a valid, different measurement)")
	}
	profile := os.Getenv("WA_HEADLESS_PROFILE_DIR")
	if profile == "" {
		t.Fatal("HEADLESS_PROFILE_DIR is required — point it at a disposable, UNPAIRED profile")
	}
	phone := os.Getenv("WA_PROBE_PAIRPHONE_NUMBER")
	if phone == "" {
		phone = "15550101234" // placeholder; a code is generated regardless of deliverability
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sess, err := h.PairingSession(ctx)
	if err != nil {
		t.Fatalf("boot (pairing-tolerant): %v", err)
	}
	eval := sess.Tab().Evaluate

	// The page needs its own ~15s to mount and populate Conn.ref/the socket
	// model before anything below is meaningful — the same settle window
	// probe_qr_test.go's own measurement documents. Poll for it instead of a
	// blind sleep, so a slow boot does not produce a false "invariant" noise
	// measurement.
	readyDeadline := time.Now().Add(30 * time.Second)
	for {
		var state string
		if err := eval(ctx, `(() => {
			try {
				const S = window.require('WAWebSocketModel');
				const sk = S && (S.Socket || S.default || S);
				return (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
			} catch (e) { return ""; }
		})()`, &state); err != nil {
			t.Fatalf("poll socket state: %v", err)
		}
		if state != "" {
			t.Logf("socket state settled: %q", state)
			break
		}
		if time.Now().After(readyDeadline) {
			t.Fatal("socket state never populated within 30s")
		}
		time.Sleep(1 * time.Second)
	}

	script := `(() => {
	window.__pp = null;
	(async () => {
		const out = { ok: false, code: "", why: "", socketBefore: "" };
		try {
			const S = window.require('WAWebSocketModel');
			const sk = S && (S.Socket || S.default || S);
			out.socketBefore = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
			window.require('WAWebAltDeviceLinkingApi').setPairingType('ALT_DEVICE_LINKING');
			await window.require('WAWebAltDeviceLinkingApi').initializeAltDeviceLinking();
			const code = await window.require('WAWebAltDeviceLinkingApi').startAltLinkingFlow(` + `"` + phone + `"` + `, true);
			out.ok = true;
			out.code = String(code || "");
		} catch (e) {
			out.why = String((e && e.message) || e).slice(0, 300);
			try {
				out.errName = e && e.name;
				out.errCode = e && (e.code != null ? e.code : (e.type && e.type.code));
				out.errTypeName = e && e.type && e.type.name;
				out.errKeys = e ? Object.keys(e) : [];
				out.errJSON = JSON.stringify(e, Object.getOwnPropertyNames(e || {})).slice(0, 500);
			} catch (e2) {}
		}
		window.__pp = JSON.stringify(out);
	})();
	return 'kicked';
})()`

	var kicked string
	if err := eval(ctx, script, &kicked); err != nil {
		t.Fatalf("kick: %v", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__pp || \"\"", &raw); err != nil {
			t.Fatalf("poll: %v", err)
		}
		if raw != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered within 60s")
		}
		time.Sleep(500 * time.Millisecond)
	}

	var out struct {
		OK           bool     `json:"ok"`
		Code         string   `json:"code"`
		Why          string   `json:"why"`
		SocketBefore string   `json:"socketBefore"`
		ErrName      string   `json:"errName"`
		ErrCode      any      `json:"errCode"`
		ErrTypeName  string   `json:"errTypeName"`
		ErrKeys      []string `json:"errKeys"`
		ErrJSON      string   `json:"errJSON"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("payload: %v (raw=%s)", err, raw)
	}
	t.Logf("MEASURED: socketBefore=%q ok=%v code=%q(len=%d) why=%q errName=%q errCode=%v errTypeName=%q errKeys=%v errJSON=%s",
		out.SocketBefore, out.OK, out.Code, len(out.Code), out.Why,
		out.ErrName, out.ErrCode, out.ErrTypeName, out.ErrKeys, out.ErrJSON)
}

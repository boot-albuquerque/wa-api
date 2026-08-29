package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeQRConstructionSurface measures, against the OBSERVATION profile
// (read-only path — see realspa_test.go's observationProfileDir), whether the
// modules whatsapp-web.js 's Client.js uses to build the pairing QR string
// (src/Client.js, initialize(), the qr branch) exist in this build.
//
// # Why this is safe against a profile whose paired state is unknown
//
// Every check here is MODULE/FUNCTION PRESENCE only (`window.require(name)` +
// `typeof m.fn === "function"`) — never an invocation. HOUSEKEEP H122's own
// pairing-surface probe (TestProbePairingSurface) established that presence
// checks are meaningful regardless of pairing state; only the FUNCTIONAL
// behaviour of some of these (state-gated flows) depends on being UNPAIRED.
// Reading Conn.ref's VALUE would leak the current QR/session material,
// so this probe reports only whether Conn.ref is populated (bool), never the
// string.
//
// wwebjs's construction (Client.js, current main branch):
//
//	registrationInfo = await window.require('WAWebSignalStoreApi').waSignalStore.getRegistrationInfo()
//	noiseKeyPair     = await window.require('WAWebUserPrefsInfoStore').waNoiseInfo.get()
//	staticKeyB64     = window.require('WABase64').encodeB64(noiseKeyPair.staticKeyPair.pubKey)
//	identityKeyB64   = window.require('WABase64').encodeB64(registrationInfo.identityKeyPair.pubKey)
//	advSecretKey     = await window.require('WAWebUserPrefsMultiDevice').getADVSecretKey()
//	platform         = window.require('WAWebCompanionRegClientUtils').DEVICE_PLATFORM
//	qr = ref + ',' + staticKeyB64 + ',' + identityKeyB64 + ',' + advSecretKey + ',' + platform
//	ref rotation: window.require('WAWebConnModel').Conn.on('change:ref', (_, ref) => ...)
//	refresh (state UNPAIRED_IDLE): window.require('WAWebCmd').Cmd.refreshQR()
func TestProbeQRConstructionSurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_QR") == "" {
		t.Skip("set WA_PROBE_QR=1")
	}
	dir, overridden, err := observationProfileDir()
	if err != nil {
		t.Fatalf("resolving observation profile: %v", err)
	}
	if overridden {
		if err := requireExistingProfile(dir); err != nil {
			t.Fatal(err)
		}
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: dir, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		// MEASURED, 2026-08-29: core.StartSession (the ONLY boot path
		// Holder.Session calls — internal/wa-headless/runtime/holder.go:121,
		// unconditionally, regardless of registry.KindPairing vs
		// KindOperational) refuses any page classification other than
		// spa.ClassAppReady, by explicit design — see
		// internal/wa-headless/core/session.go:8-10,321-323: "QR pairing is
		// a separate, human-authorised slice: nothing here shows a QR code,
		// waits for one, or mutates an unpaired profile."
		//
		// Against .lab/test-account-profile (currently UNPAIRED — page
		// classified PAIRING_LOADING), the boot fails exactly there. This
		// means registry.KindPairing's quota (registry.go) is bookkeeping
		// with no boot capability behind it yet: there is NO code path in
		// this repository, production or test, that boots into a
		// QR-showing page and interacts with it. Building a QRReader for
		// wa_headless requires a NEW boot primitive at the core/ layer
		// (e.g. core.StartPairingSession, tolerant of the QR/pairing page
		// classes) BEFORE any adapter can read WAWebConnModel.Conn.ref —
		// this is bigger than an adapter, and it crosses a boundary this
		// package's own authors marked as needing deliberate authorisation.
		t.Skipf("MEASURED (not a bug): core.StartSession refuses non-APP_READY pages "+
			"by design — %v. See this test's doc comment: QR reading needs a new "+
			"core-layer boot primitive that does not exist yet.", err)
	}
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__qrprobe = null;
	const out = {modules:{}, funcs:{}, fields:{}};
	const look = (name, fns) => {
		try {
			const m = window.require(name);
			out.modules[name] = !!m;
			for (const f of (fns||[])) { out.funcs[name+"."+f] = !!(m && typeof m[f] === "function"); }
			return m;
		} catch (e) { out.modules[name] = false; return null; }
	};
	look("WAWebSignalStoreApi", []);
	try {
		const sig = window.require("WAWebSignalStoreApi");
		out.funcs["WAWebSignalStoreApi.waSignalStore.getRegistrationInfo"] =
			!!(sig && sig.waSignalStore && typeof sig.waSignalStore.getRegistrationInfo === "function");
	} catch (e) {}
	look("WAWebUserPrefsInfoStore", []);
	try {
		const ps = window.require("WAWebUserPrefsInfoStore");
		out.funcs["WAWebUserPrefsInfoStore.waNoiseInfo.get"] =
			!!(ps && ps.waNoiseInfo && typeof ps.waNoiseInfo.get === "function");
	} catch (e) {}
	look("WABase64", ["encodeB64"]);
	look("WAWebUserPrefsMultiDevice", ["getADVSecretKey"]);
	look("WAWebCompanionRegClientUtils", []);
	try {
		const p = window.require("WAWebCompanionRegClientUtils");
		out.fields["WAWebCompanionRegClientUtils.DEVICE_PLATFORM"] = p ? (typeof p.DEVICE_PLATFORM) : "absent";
	} catch (e) {}
	look("WAWebCmd", ["refreshQR"]);
	look("WAWebLaunchSocketUtils", ["refreshQR"]);
	const conn = look("WAWebConnModel", []);
	try {
		const C = conn && conn.Conn;
		out.funcs["WAWebConnModel.Conn.on"] = !!(C && typeof C.on === "function");
		out.funcs["WAWebConnModel.Conn.off"] = !!(C && typeof C.off === "function");
		// PRESENCE only — never the value, which is live QR/session material.
		out.fields["WAWebConnModel.Conn.ref_populated"] = !!(C && !!C.ref);
	} catch (e) {}
	const sock = look("WAWebSocketModel", []);
	try {
		const sk = sock && (sock.Socket || sock.default || sock);
		out.fields["socket_state"] = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
	} catch (e) {}
	window.__qrprobe = JSON.stringify(out);
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__qrprobe", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(300 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("QR construction surface (profile=%s, overridden=%v):\n%s", dir, overridden, out)
}

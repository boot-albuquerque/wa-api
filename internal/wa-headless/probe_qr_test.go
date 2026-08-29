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
// (see realspa_test.go's observationProfileDir) booted through
// core.StartPairingSession (HOUSEKEEP H145 — the restoration-only path
// refuses a QR-showing page by design), whether the modules and the FULL
// CHAIN whatsapp-web.js's Client.js uses to build the pairing QR string
// (src/Client.js, initialize(), the qr branch) work in this build.
//
// # Why this stays read-only despite calling the getters
//
// It calls getRegistrationInfo/waNoiseInfo.get/getADVSecretKey/encodeB64 —
// not just checks their presence — because presence alone does not prove the
// chain composes (CLAUDE.md's own lesson: measure the real thing, a caricature
// measures something else). None of these MUTATE the profile: they are
// getters over material the page already generated at profile creation.
// What this probe never does is print a VALUE: every field reported is a
// bool (succeeded / matches expected shape) or a typeof string, never the
// key bytes, the ref, or the assembled QR string itself — all of which are
// live session material for whatever profile this runs against.
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
	sess, err := h.PairingSession(ctx)
	if err != nil {
		t.Fatalf("boot (pairing-tolerant): %v", err)
	}
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__qrprobe = null;
	(async () => {
	const out = {modules:{}, funcs:{}, fields:{}, chain:{}};
	const look = (name, fns) => {
		try {
			const m = window.require(name);
			out.modules[name] = !!m;
			for (const f of (fns||[])) { out.funcs[name+"."+f] = !!(m && typeof m[f] === "function"); }
			return m;
		} catch (e) { out.modules[name] = false; return null; }
	};
	look("WAWebSignalStoreApi", []);
	look("WAWebUserPrefsInfoStore", []);
	look("WABase64", ["encodeB64"]);
	look("WAWebUserPrefsMultiDevice", ["getADVSecretKey"]);
	look("WAWebCompanionRegClientUtils", []);
	const cmd = look("WAWebCmd", []);
	out.funcs["WAWebCmd.Cmd.refreshQR"] = !!(cmd && cmd.Cmd && typeof cmd.Cmd.refreshQR === "function");
	look("WAWebLaunchSocketUtils", ["refreshQR"]);
	const conn = look("WAWebConnModel", []);
	const connRef = () => { try { return (conn && conn.Conn && conn.Conn.ref) || ""; } catch (e) { return ""; } };
	try {
		const C = conn && conn.Conn;
		out.funcs["WAWebConnModel.Conn.on"] = !!(C && typeof C.on === "function");
		out.funcs["WAWebConnModel.Conn.off"] = !!(C && typeof C.off === "function");
	} catch (e) {}
	const sock = look("WAWebSocketModel", []);
	try {
		const sk = sock && (sock.Socket || sock.default || sock);
		out.fields["socket_state"] = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
	} catch (e) {}

	// Wait for the QR ref to actually populate (measured: it arrives around
	// t+15s on the pairing screen — spa/page.go's own ClassPairingLoading
	// comment) before exercising the chain that consumes it. PRESENCE only
	// in the report — never the value, which is live QR/session material.
	const refDeadline = Date.now() + 25000;
	while (!connRef() && Date.now() < refDeadline) {
		await new Promise(r => setTimeout(r, 500));
	}
	out.fields["WAWebConnModel.Conn.ref_populated"] = !!connRef();

	// The FULL CHAIN, invoked for real — booleans/typeof only, never a value.
	try {
		const registrationInfo = await window.require('WAWebSignalStoreApi').waSignalStore.getRegistrationInfo();
		out.chain.getRegistrationInfo_ok = !!registrationInfo;
		out.chain.identityKeyPair_pubKey_present = !!(registrationInfo && registrationInfo.identityKeyPair && registrationInfo.identityKeyPair.pubKey);
	} catch (e) { out.chain.getRegistrationInfo_error = String((e && e.message) || e).slice(0, 140); }
	try {
		const noiseKeyPair = await window.require('WAWebUserPrefsInfoStore').waNoiseInfo.get();
		out.chain.waNoiseInfo_get_ok = !!noiseKeyPair;
		out.chain.staticKeyPair_pubKey_present = !!(noiseKeyPair && noiseKeyPair.staticKeyPair && noiseKeyPair.staticKeyPair.pubKey);
	} catch (e) { out.chain.waNoiseInfo_error = String((e && e.message) || e).slice(0, 140); }
	try {
		const advSecretKey = await window.require('WAWebUserPrefsMultiDevice').getADVSecretKey();
		out.chain.getADVSecretKey_ok = (typeof advSecretKey === "string" && advSecretKey.length > 0);
	} catch (e) { out.chain.getADVSecretKey_error = String((e && e.message) || e).slice(0, 140); }
	try {
		const platform = window.require('WAWebCompanionRegClientUtils').DEVICE_PLATFORM;
		out.chain.DEVICE_PLATFORM_type = typeof platform;
	} catch (e) { out.chain.DEVICE_PLATFORM_error = String((e && e.message) || e).slice(0, 140); }
	try {
		const b64 = window.require('WABase64').encodeB64;
		out.chain.encodeB64_roundtrip_ok = (typeof b64 === "function") && (typeof b64(new Uint8Array([1,2,3])) === "string");
	} catch (e) { out.chain.encodeB64_error = String((e && e.message) || e).slice(0, 140); }

	window.__qrprobe = JSON.stringify(out);
	})();
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
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

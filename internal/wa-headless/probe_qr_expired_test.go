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

// TestProbeQRExpiredRecovery measures whether
// WAWebLaunchSocketUtils.refreshQR(), called PROGRAMMATICALLY while the SPA
// is showing its own "code expired, click to refresh" overlay
// (data-testid="link_device_qr_expired_refresh_button" — measured present
// after 6 automatic rotations, ~2m30s, in TestProbeQRRetryPattern), recovers
// promptly (a new Conn.ref within a few seconds) or not (leaving the
// existing capabilities/qr.Reader nudge blind to this state, since it only
// fires on an EMPTY ref, and ref stays populated — just stale — while
// "expired" is showing).
//
// READ-ONLY except for the one refreshQR() call under test: everything
// reported is a hash/boolean, never the ref value itself.
func TestProbeQRExpiredRecovery(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_QR_EXPIRED") == "" {
		t.Skip("set WA_PROBE_QR_EXPIRED=1 (long-running: ~3-4 minutes)")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.PairingSession(ctx)
	if err != nil {
		t.Fatalf("boot (pairing-tolerant): %v", err)
	}
	eval := sess.Tab().Evaluate

	const stateScript = `JSON.stringify((() => {
	const out = { refHash: "", expired: false };
	try {
		const conn = window.require('WAWebConnModel');
		const ref = (conn && conn.Conn && conn.Conn.ref) || "";
		let h = 0;
		for (let i = 0; i < ref.length; i++) { h = ((h << 5) - h + ref.charCodeAt(i)) | 0; }
		out.refHash = String(h) + ":" + ref.length;
	} catch (e) {}
	out.expired = !!document.querySelector('[data-testid="link_device_qr_expired_refresh_button"]');
	return out;
})())`

	type state struct {
		RefHash string `json:"refHash"`
		Expired bool   `json:"expired"`
	}
	readState := func() state {
		var raw string
		if err := eval(ctx, stateScript, &raw); err != nil {
			t.Fatalf("eval: %v", err)
		}
		var s state
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Fatalf("unmarshal: %v (raw=%s)", err, raw)
		}
		return s
	}

	start := time.Now()
	// Phase 1: wait for the expired overlay (measured to appear around
	// 2m30s; budget generously to 4 minutes).
	waitDeadline := start.Add(4 * time.Minute)
	var s state
	for {
		s = readState()
		if s.Expired {
			t.Logf("[%s] expired overlay appeared (refHash=%s)", time.Since(start).Round(time.Second), s.RefHash)
			break
		}
		if !time.Now().Before(waitDeadline) {
			t.Fatal("expired overlay never appeared within 4 minutes")
		}
		time.Sleep(5 * time.Second)
	}
	refHashBeforeNudge := s.RefHash

	// Phase 2: fire the SAME call capabilities/qr.Reader fires, and time
	// how long recovery takes.
	nudgeAt := time.Now()
	var kicked string
	if err := eval(ctx, `(() => {
		try {
			const ls = window.require('WAWebLaunchSocketUtils');
			if (ls && typeof ls.refreshQR === 'function') { ls.refreshQR(); return 'fired'; }
			return 'absent';
		} catch (e) { return 'error:' + String((e && e.message) || e).slice(0,140); }
	})()`, &kicked); err != nil {
		t.Fatalf("nudge eval: %v", err)
	}
	t.Logf("[%s] fired WAWebLaunchSocketUtils.refreshQR() (result=%s)", time.Since(start).Round(time.Second), kicked)

	// Phase 3: poll every second for up to 60s, looking for EITHER the ref
	// to change OR the expired overlay to clear — whichever the nudge
	// actually produces.
	recoveryDeadline := time.Now().Add(60 * time.Second)
	for {
		s = readState()
		if s.RefHash != refHashBeforeNudge {
			t.Logf("RESULT: ref changed %s after the nudge — refreshQR() recovers promptly, "+
				"same call our production code already makes just needs to fire on 'expired', not only on empty ref",
				time.Since(nudgeAt).Round(100*time.Millisecond))
			return
		}
		if !s.Expired {
			t.Logf("RESULT: expired overlay cleared %s after the nudge, but ref hash unchanged — "+
				"investigate further (overlay state and ref may be decoupled)", time.Since(nudgeAt).Round(100*time.Millisecond))
			return
		}
		if !time.Now().Before(recoveryDeadline) {
			t.Logf("RESULT: no change %s after the nudge (still expired, ref unchanged) — "+
				"refreshQR() does NOT bypass the expired gate; a real click on the button is likely required",
				time.Since(nudgeAt).Round(time.Second))
			return
		}
		time.Sleep(1 * time.Second)
	}
}

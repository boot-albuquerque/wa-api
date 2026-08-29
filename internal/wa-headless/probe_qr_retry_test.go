package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeQRRetryPattern watches an UNPAIRED pairing screen over an
// EXTENDED window (default 10 minutes) to measure a pattern the user
// reported: after some number of automatic QR rotations, the SPA itself
// stops auto-refreshing and requires a manual (human/DOM) action to get a
// new code, instead of continuing indefinitely.
//
// READ-ONLY and identity-free: every sample is a HASH of Conn.ref (never
// the value — it is live pairing material), a boolean/short-string
// classification, and the current data-testid inventory (interface
// scaffolding, not account data — EVIDENCIA-SPA.md M1.4 already
// catalogued this same list from a healthy boot).
//
// Long-running by nature: set WA_PROBE_QR_RETRY=1 and, optionally,
// WA_PROBE_QR_RETRY_MINUTES (default 10) to extend or shorten the window.
func TestProbeQRRetryPattern(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_QR_RETRY") == "" {
		t.Skip("set WA_PROBE_QR_RETRY=1 (long-running: several minutes)")
	}
	minutes := 10
	if raw := os.Getenv("WA_PROBE_QR_RETRY_MINUTES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			minutes = n
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(minutes+1)*time.Minute)
	defer cancel()
	sess, err := h.PairingSession(ctx)
	if err != nil {
		t.Fatalf("boot (pairing-tolerant): %v", err)
	}
	eval := sess.Tab().Evaluate

	const sampleScript = `JSON.stringify((() => {
	const out = { refHash: "", refPresent: false, testids: [], socketState: "", hasCanvas: false, canvasCount: 0 };
	try {
		const conn = window.require('WAWebConnModel');
		const ref = (conn && conn.Conn && conn.Conn.ref) || "";
		out.refPresent = !!ref;
		// A cheap non-cryptographic hash: enough to detect CHANGE, never to
		// recover the value.
		let h = 0;
		for (let i = 0; i < ref.length; i++) { h = ((h << 5) - h + ref.charCodeAt(i)) | 0; }
		out.refHash = String(h) + ":" + ref.length;
	} catch (e) {}
	try {
		const sock = window.require('WAWebSocketModel');
		const sk = sock && (sock.Socket || sock.default || sock);
		out.socketState = (sk && typeof sk.__x_state === "string") ? sk.__x_state : "";
	} catch (e) {}
	try {
		const nodes = document.querySelectorAll('[data-testid]');
		const seen = new Set();
		nodes.forEach(n => seen.add(n.getAttribute('data-testid')));
		out.testids = Array.from(seen).sort();
	} catch (e) {}
	try {
		const canvases = document.querySelectorAll('canvas');
		out.canvasCount = canvases.length;
		out.hasCanvas = canvases.length > 0;
	} catch (e) {}
	return out;
})())`

	type sample struct {
		RefHash     string   `json:"refHash"`
		RefPresent  bool     `json:"refPresent"`
		Testids     []string `json:"testids"`
		SocketState string   `json:"socketState"`
		HasCanvas   bool     `json:"hasCanvas"`
		CanvasCount int      `json:"canvasCount"`
	}

	var lastRefHash string
	var lastTestids string
	rotations := 0
	start := time.Now()
	deadline := start.Add(time.Duration(minutes) * time.Minute)

	for time.Now().Before(deadline) {
		var raw string
		if err := eval(ctx, sampleScript, &raw); err != nil {
			t.Logf("[%s] eval error: %v", time.Since(start).Round(time.Second), err)
			time.Sleep(5 * time.Second)
			continue
		}
		var s sample
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Logf("[%s] unmarshal error: %v (raw=%s)", time.Since(start).Round(time.Second), err, raw)
			time.Sleep(5 * time.Second)
			continue
		}
		testidsJoined, _ := json.Marshal(s.Testids)
		if string(testidsJoined) != lastTestids {
			t.Logf("[%s] testid inventory changed: %s", time.Since(start).Round(time.Second), testidsJoined)
			lastTestids = string(testidsJoined)
		}
		if s.RefHash != lastRefHash {
			if lastRefHash != "" {
				rotations++
				t.Logf("[%s] ROTATION #%d: ref changed (present=%v canvases=%d socket=%q)",
					time.Since(start).Round(time.Second), rotations, s.RefPresent, s.CanvasCount, s.SocketState)
			} else {
				t.Logf("[%s] initial ref state: present=%v canvases=%d socket=%q",
					time.Since(start).Round(time.Second), s.RefPresent, s.CanvasCount, s.SocketState)
			}
			lastRefHash = s.RefHash
		}
		time.Sleep(5 * time.Second)
	}
	t.Logf("done: %d rotations observed over %s", rotations, time.Since(start).Round(time.Second))
}

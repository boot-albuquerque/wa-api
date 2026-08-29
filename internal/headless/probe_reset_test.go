package headless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/liveness"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
	"wa-api/internal/headless/spa"
)

// TestProbeResetState measures whether Socket.reconnect() — which is all
// Client.resetState does — produces an OBSERVABLE transition.
//
// The question matters because a reset that cannot be observed cannot have a
// postcondition, and a write with no postcondition is what invariant 14 forbids.
// If the socket never leaves CONNECTED, the honest outcome is a row that says so
// rather than a capability that reports success for doing nothing.
func TestProbeResetState(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_RESET") == "" {
		t.Skip("set WA_PROBE_RESET=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__rs = null;
	const safe = e => String((e && e.message) || e).slice(0, 140);
	const read = () => {
		try {
			const m = window.require('WAWebSocketModel');
			const sk = m && (m.Socket || m.default || m);
			return (sk && typeof sk.__x_state === 'string') ? sk.__x_state : '';
		} catch (e) { return ''; }
	};
	const out = {before: read(), distinct: {}, samples: 0, control: {}};
	try {
		const m = window.require('WAWebSocketModel');
		const sk = m && (m.Socket || m.default || m);
		// CONTROLE POSITIVO: o instrumento consegue ver ALGUMA transicao?
		// Forcar uma desconexao de verdade e' invasivo; em vez disso, o controle
		// e' provar que o amostrador enxerga uma mudanca de valor qualquer no
		// mesmo campo, escrevendo um valor sentinela e lendo de volta.
		const original = sk.__x_state;
		try {
			sk.__x_state = "PROBE_SENTINEL";
			out.control.sawSentinel = (read() === "PROBE_SENTINEL");
		} finally {
			sk.__x_state = original;
		}
		out.control.restored = (read() === original);

		sk.reconnect();
		let n = 0;
		const t = setInterval(() => {
			const v = read();
			out.distinct[v] = (out.distinct[v] || 0) + 1;
			out.samples++;
			if (++n >= 100) { clearInterval(t); window.__rs = JSON.stringify(out); }
		}, 50);
	} catch (e) { out.err = safe(e); window.__rs = JSON.stringify(out); }
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
		if err := eval(ctx, "window.__rs", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the sampling never finished")
		}
		time.Sleep(500 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("socket reset:\n%s", out)
}

// TestProbeResetCapability proves capabilities/liveness Reset against the live
// socket, three times — because the transition is ~450ms and a single run cannot
// distinguish "usually catches it" from "caught it once".
func TestProbeResetCapability(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_RESET") == "" {
		t.Skip("set WA_PROBE_RESET=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	var sawOpening int
	for i := 0; i < 3; i++ {
		got, err := liveness.Reset(ctx, runner, eval, "probe/reset")
		if err != nil {
			t.Fatalf("Reset %d: %v", i+1, err)
		}
		if !got.Settled {
			t.Errorf("Reset %d did not settle", i+1)
		}
		if got.SawOpening {
			sawOpening++
		}
		t.Logf("reset %d: %s", i+1, got)
	}
	// SawOpening is INFORMATION and never a requirement, so this is a log and not
	// an assertion: the window measured ~450ms and a busy machine steps over it.
	t.Logf("the transition was caught in %d of 3 resets", sawOpening)

	// The session must still work afterwards — a reset that leaves the socket
	// usable is the only kind worth having.
	var after string
	if err := eval(ctx, spa.SocketStateReadExpr, &after); err != nil {
		t.Fatalf("reading the socket after three resets: %v", err)
	}
	if after != string(spa.SocketStateConnected) {
		t.Errorf("after three resets the socket reads %q", after)
	}
}

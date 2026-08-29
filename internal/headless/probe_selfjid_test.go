package headless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeSelfJID writes THIS profile's own phone jid to the file named by
// WA_PROBE_SELF_OUT and never logs it. It exists so a live test on the other
// lab account can be given a peer without the identity passing through a
// transcript.
func TestProbeSelfJID(t *testing.T) {
	requireRealSPA(t)
	out := os.Getenv("WA_PROBE_SELF_OUT")
	if out == "" {
		t.Skip("set WA_PROBE_SELF_OUT=/path")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	var raw string
	script := `(() => {
		const Me = window.require('WAWebUserPrefsMeUser');
		for (const k of ['getMaybeMeUser','getMeUser','getMe','getMeUserOrThrow']) {
			try {
				if (typeof Me[k] === 'function') {
					const u = Me[k]();
					if (u && u.user) { return u.user + '@c.us'; }
				}
			} catch (e) {}
		}
		return 'KEYS:' + Object.keys(Me).join(',').slice(0, 400);
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/self", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if raw == "" {
		t.Fatal("no me user")
	}
	if err := os.WriteFile(out, []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %d bytes to the requested file (identity not logged)", len(raw))
}

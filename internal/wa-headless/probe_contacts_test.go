package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeContactShape is a measurement bench, not a guard. Its script is
// rewritten as questions come up; what stays is the harness.
//
// It prints counts and shapes, never a name, a number or an identity.
func TestProbeContactShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONTACTS") == "" {
		t.Skip("set WA_PROBE_CONTACTS=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// IS THIS SESSION'S PAGE STABLE AT ALL?
	//
	// A previous probe found window.__waHeadlessSub "missing" for eighty
	// seconds on this profile — and that global is assigned on the FIRST line
	// of the script that plants it, so its absence means the page it was
	// assigned in is gone. A reloading page would explain both that and a
	// presence subscription that never persists, and attributing the presence
	// failure to the subscription without checking this would blame the wrong
	// thing.
	//
	// So: plant a marker, read it repeatedly, and see whether it survives. A
	// reads counter that keeps climbing means one page; a counter that resets
	// means the page is being replaced under us.
	const script = `(() => {
		if (!window.__waHeadlessLife) {
			window.__waHeadlessLife = { reads: 0, path: location.pathname };
		}
		window.__waHeadlessLife.reads++;
		return JSON.stringify(window.__waHeadlessLife);
	})()`

	var raw string
	for i := 0; i < 8; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/life", func(c context.Context) error {
			return sess.Tab().Evaluate(c, script, &raw)
		}); err != nil {
			t.Fatalf("probe: %v", err)
		}
		t.Logf("sample %d: %s", i, raw)
		time.Sleep(3 * time.Second)
	}
}

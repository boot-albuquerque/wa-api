package waheadless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeIdleResponsiveness measures WHEN a session stops answering while
// nobody is talking to it.
//
// It exists because a control disproved a finding I was about to write. A probe
// that bound a catch-all event handler could not read its own results back —
// every attempt hit the 5s budget — and the obvious conclusion was that the
// handler had saturated the page. The control, which waited the same 90 seconds
// with NOTHING bound, failed identically. So the handler was innocent and the
// real question is this one.
//
// It matters beyond the probe: every long-lived capability in this module
// assumes a session stays usable between calls. Invariant 15 says a session's
// lifetime ends only at Stop, and that is about the CONTEXT, not about whether
// the page still answers.
func TestProbeIdleResponsiveness(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_IDLE") == "" {
		t.Skip("set WA_PROBE_IDLE=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// A trivial expression: this measures whether the page ANSWERS, not whether
	// it can do work. Anything heavier would confuse "slow" with "silent".
	const ping = `JSON.stringify({ok:true, n: Date.now() % 1000})`

	ask := func(label string) (time.Duration, error) {
		var raw string
		start := time.Now()
		err := runner.Do(ctx, engine.OpStateProbe, label, func(c context.Context) error {
			return sess.Tab().Evaluate(c, ping, &raw)
		})
		return time.Since(start), err
	}

	// Gaps chosen to bracket the 90s that failed, so the answer is a THRESHOLD
	// and not another single data point.
	gaps := []time.Duration{0, 10 * time.Second, 20 * time.Second, 30 * time.Second, 60 * time.Second, 90 * time.Second}
	var idle time.Duration
	for i, gap := range gaps {
		if gap > 0 {
			time.Sleep(gap)
			idle += gap
		}
		d, err := ask(fmt.Sprintf("probe/idle/%d", i))
		if err != nil {
			t.Logf("after %s idle: FAILED in %s — %v", idle, d.Round(time.Millisecond), err)
			// Does it recover, or is it gone for good? Those are different
			// defects with different repairs.
			for r := 0; r < 3; r++ {
				time.Sleep(3 * time.Second)
				d2, err2 := ask(fmt.Sprintf("probe/idle/%d/retry%d", i, r))
				if err2 == nil {
					t.Logf("  recovered on retry %d after %s", r+1, d2.Round(time.Millisecond))
					idle = 0
					break
				}
				t.Logf("  retry %d still failing: %v", r+1, err2)
			}
			continue
		}
		t.Logf("after %s idle: answered in %s", idle, d.Round(time.Millisecond))
		// A successful call resets the clock, which is the point: the question
		// is idleness, not wall time since boot.
		idle = 0
	}
}

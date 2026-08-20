package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeContactShape measures the roster BEFORE any listContacts capability
// is designed, and it measures SHAPES, never values.
//
// A contact's name and number are the most personal data this module can
// touch, so the probe counts how many contacts have each field populated and
// how the identities are distributed — it never returns a name, a pushname or
// a jid. That is not caution for its own sake: the briefing forbids PII in
// logs, and a probe is a log.
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

	// Third pass: WHICH ROWS ARE NOT PEOPLE?
	//
	// The avatar proof hung on exactly one contact, and the redacted shape said
	// server=c.us userLen=1 — a single digit, which is a system sentinel and
	// not a person. listContacts had returned it as one.
	//
	// The fix must not be a length heuristic. The page carries its own
	// predicates on a wid (isUser, isServer, isPSA, isGroup, isNewsletter), so
	// this pass counts which of them separate that row from the rest. Counters
	// only; no identity leaves the page.
	// DOES THE CANDIDATE MARK CHANGE ON ITS OWN?
	//
	// Diffing all of localStorage across a sync showed exactly one interesting
	// key moving: contact-sync-refresh-seconds. (WAWebTimeSpentSession also
	// moves, but it moves constantly and means nothing here.)
	//
	// "It changed after I called the sync" is not attribution. This pass reads
	// it three times — before an idle wait, after the idle wait, and after the
	// sync — so a key that drifts on its own is caught before it becomes a
	// detector. The idle wait is the CONTROL and it is the whole point of the
	// experiment.
	//
	// The value is a refresh interval in seconds, not account data.
	const script = `(() => {
		window.__waHeadlessMark = { stage: 'pending' };
		const read = () => {
			try { return String(localStorage.getItem('contact-sync-refresh-seconds')); }
			catch (e) { return 'THREW'; }
		};
		(async () => {
			const r = { stage: 'done' };
			try {
				r.t0 = read();
				// CONTROL: idle for longer than the sync takes, touching nothing.
				await new Promise(res => setTimeout(res, 45000));
				r.t1_afterIdle = read();
				const start = Date.now();
				await window.require('WAWebContactSyncBridge').doFullContactSync();
				r.syncMs = Date.now() - start;
				r.t2_afterSync = read();
				r.movedWhileIdle = r.t0 !== r.t1_afterIdle;
				r.movedBySync = r.t1_afterIdle !== r.t2_afterSync;
			} catch (e) {
				r.error = String((e && e.message) || e).slice(0, 180);
			}
			window.__waHeadlessMark = r;
		})();
		return 'kicked';
	})()`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/contacts/kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe kick: %v", err)
	}
	for i := 0; ; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/contacts/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessMark || {stage:"missing"})`, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) {
			break
		}
		if i > 90 {
			t.Fatal("the sync never settled")
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("mark attribution: %s", raw)
}

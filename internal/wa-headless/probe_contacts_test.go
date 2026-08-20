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
	// DOES A SYNC CHANGE ANYTHING? The previous pass answered the cheaper
	// question first and the answer was NO GAP: of 391 contacts the chats
	// reference, 391 are already in the roster of 944. So membership is not
	// what priming would fix.
	//
	// What IS thin is names: getName answered for 1 of 944. This pass measures
	// whether the page's own contact sync populates them, with a snapshot
	// before and after — because "we called it and things look fine" is not a
	// measurement.
	//
	// It runs on a LAB account, and the call is the same periodic refresh the
	// page performs on its own every 86400s. Async, so store-and-poll.
	const script = `(() => {
		window.__waHeadlessPrime = { stage: 'pending' };
		const snap = () => {
			const CC = window.require('WAWebContactCollection').ContactCollection;
			const G = window.require('WAWebContactGetters');
			const all = CC.getModelsArray();
			let name = 0, pushname = 0, shortName = 0, verified = 0;
			const servers = {};
			for (const c of all) {
				try {
					const sv = (c.id && c.id.server) || '?';
					servers[sv] = (servers[sv] || 0) + 1;
					if (G.getName && G.getName(c)) name++;
					if (G.getPushname && G.getPushname(c)) pushname++;
					if (G.getShortName && G.getShortName(c)) shortName++;
					if (G.getVerifiedName && G.getVerifiedName(c)) verified++;
				} catch (e) {}
			}
			return { total: all.length, withName: name, withPushname: pushname,
				withShortName: shortName, withVerifiedName: verified, servers: servers };
		};
		(async () => {
			const out = { stage: 'done' };
			try {
				out.before = snap();
				const t0 = Date.now();
				const B = window.require('WAWebContactSyncBridge');
				try {
					const r = await B.doFullContactSync();
					out.syncReturned = (r === undefined) ? 'undefined'
						: (r === null ? 'null' : (typeof r === 'object' ? Object.keys(r).join(',') : String(r).slice(0, 60)));
				} catch (e) {
					out.syncError = String((e && e.message) || e).slice(0, 200);
				}
				out.syncMs = Date.now() - t0;
				out.after = snap();
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 200);
			}
			window.__waHeadlessPrime = out;
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
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessPrime || {stage:"missing"})`, &raw)
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
	t.Logf("prime before/after: %s", raw)
}

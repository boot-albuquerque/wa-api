package waheadless

import (
	"context"
	"os"
	"testing"

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
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
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
	const script = `JSON.stringify((() => {
		const coll = window.require('WAWebContactCollection').ContactCollection;
		const all = coll.getModelsArray();
		const out = { total: all.length, byPredicate: {}, shortUsers: {}, oddballs: [] };
		const preds = ['isUser','isServer','isPSA','isGroup','isNewsletter','isBroadcast','isLid'];
		for (const p of preds) { out.byPredicate[p] = 0; }
		for (let i = 0; i < all.length; i++) {
			const id = all[i].id;
			if (!id) { continue; }
			for (const p of preds) {
				try { if (typeof id[p] === 'function' && id[p]()) { out.byPredicate[p]++; } } catch (e) {}
			}
			const len = (id.user || '').length;
			out.shortUsers[len] = (out.shortUsers[len] || 0) + 1;
			// Anything with an implausibly short user: report its PREDICATES,
			// never its value, so the filter can be written against a rule the
			// page owns instead of against a length.
			if (len <= 4 && out.oddballs.length < 10) {
				const d = { len: len, server: id.server };
				for (const p of preds) {
					try { d[p] = (typeof id[p] === 'function') ? !!id[p]() : 'n/a'; } catch (e) { d[p] = 'THREW'; }
				}
				try { d.isMe = !!all[i].isMe; } catch (e) {}
				out.oddballs.push(d);
			}
		}
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/contacts", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("contact shape: %s", raw)
}

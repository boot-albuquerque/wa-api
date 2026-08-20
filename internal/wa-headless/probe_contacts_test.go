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

	// Second pass: is the roster DOUBLE-COUNTING people? The first pass
	// measured 454 c.us against 489 lid, which is nothing like the message
	// collection's 397-of-399 lid, and a listing that returned both
	// representations of one person would be wrong in a way no caller could
	// detect. Counters and hashes only — never an identity in the clear.
	const script = `JSON.stringify((() => {
		const out = {};
		const coll = window.require('WAWebContactCollection').ContactCollection;
		const all = coll.getModelsArray();
		out.total = all.length;

		// Which cross-identity fields does the model actually carry?
		const fieldHits = {};
		const candidates = ['phoneNumber','lid','pn','userLid','pnForLid','lidForPn','alternateWid','deviceJid'];
		const pnUsers = new Set(), lidUsers = new Set();
		let linkedPnFromLid = 0, linkedLidFromPn = 0;
		for (let i = 0; i < all.length; i++) {
			const c = all[i];
			for (const f of candidates) {
				try {
					const v = c[f];
					if (v !== undefined && v !== null && v !== '') {
						fieldHits[f] = (fieldHits[f] || 0) + 1;
					}
				} catch (e) { /* getter may throw */ }
			}
			try {
				const id = c.id;
				if (!id) { continue; }
				if (id.server === 'c.us') {
					pnUsers.add(id.user);
					if (c.lid && c.lid.user) { linkedLidFromPn++; }
				} else if (id.server === 'lid') {
					lidUsers.add(id.user);
					if (c.phoneNumber && c.phoneNumber.user) { linkedPnFromLid++; }
				}
			} catch (e) { /* skip */ }
		}
		out.fieldHits = fieldHits;
		out.distinctPn = pnUsers.size;
		out.distinctLid = lidUsers.size;
		out.lidRowsCarryingAPhone = linkedPnFromLid;
		out.pnRowsCarryingALid = linkedLidFromPn;

		// THE QUESTION: do the two halves describe the same people? Count how
		// many phone identities are reachable from the lid rows.
		const phonesFromLidRows = new Set();
		for (let i = 0; i < all.length; i++) {
			try {
				const c = all[i];
				if (c.id && c.id.server === 'lid' && c.phoneNumber && c.phoneNumber.user) {
					phonesFromLidRows.add(c.phoneNumber.user);
				}
			} catch (e) { /* skip */ }
		}
		let overlap = 0;
		phonesFromLidRows.forEach(u => { if (pnUsers.has(u)) { overlap++; } });
		out.phonesReachableFromLidRows = phonesFromLidRows.size;
		out.overlapWithPnRows = overlap;
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

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
	// CONFIRMING THE SHAPE: the helper wants a CONTACT MODEL, not a wid.
	//
	// Its source destructures t.id.isLid(), t.phoneNumber and t.username — all
	// fields of a contact, none of a wid. This is the THIRD time today the same
	// mistake shape appeared: the avatar bridge died on 'isNewsletter' and this
	// on 'isLid', both because an argument that CARRIES an id was handed the id.
	const script = `JSON.stringify((() => {
		const out = {};
		const PU = window.require('WAWebGroupMutationParticipantUtils');
		const CC = window.require('WAWebContactCollection').ContactCollection;
		let contact = null;
		for (const c of CC.getModelsArray()) {
			try { if (c.id && c.id.server === 'lid' && c.phoneNumber) { contact = c; break; } } catch (e) {}
		}
		out.foundContact = !!contact;
		if (!contact) { return out; }
		// The wid — what was passed before, and what failed.
		try {
			PU.getGroupMutationParticipant(contact.id, true, 'createGroup');
			out.withWid = 'ACCEPTED';
		} catch (e) { out.withWid = String((e && e.message) || e).slice(0, 90); }
		// The contact MODEL — what the source asks for.
		try {
			const p = PU.getGroupMutationParticipant(contact, true, 'createGroup');
			out.withContact = p && typeof p === 'object' ? Object.keys(p).join(',') : String(p);
		} catch (e) { out.withContact = 'THREW: ' + String((e && e.message) || e).slice(0, 120); }
		// Can a contact be reached from a resolved wid? That is the step the
		// capability will need.
		try {
			const got = CC.get(contact.id);
			out.contactFromWid = !!got;
		} catch (e) { out.getErr = String((e && e.message) || e).slice(0, 100); }
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/groupcreate", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("participant shape: %s", raw)
}

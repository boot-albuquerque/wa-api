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
	// THE TWO CALLS THAT ACTUALLY SEND, read rather than guessed.
	//
	// MediaPrep.prototype carries sendToChat and waitForPrep; prepRawMedia
	// takes (file, opts) and its source branches on opts.isPtt and
	// opts.asDocument. What sendToChat expects is the remaining unknown, and
	// the text path already paid four rounds for guessing at this layer (H34).
	const script = `JSON.stringify((() => {
		const out = {};
		try {
			const MP = window.require('WAWebMediaPrep').MediaPrep;
			out.sendToChat = { arity: MP.prototype.sendToChat.length,
				src: String(MP.prototype.sendToChat).slice(0, 500) };
			out.waitForPrep = { arity: MP.prototype.waitForPrep.length,
				src: String(MP.prototype.waitForPrep).slice(0, 220) };
		} catch (e) { out.mpErr = String((e && e.message) || e).slice(0, 160); }
		try {
			const P = window.require('WAWebPrepRawMedia');
			out.prepRawMedia = { arity: P.prepRawMedia.length,
				src: String(P.prepRawMedia).slice(0, 700) };
		} catch (e) { out.prepErr = String((e && e.message) || e).slice(0, 160); }
		try {
			const O = window.require('WAWebMediaOpaqueData');
			out.createFromData = { arity: O.createFromData.length,
				src: String(O.createFromData).slice(0, 260) };
		} catch (e) { out.opErr = String((e && e.message) || e).slice(0, 160); }
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/media2", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("send signatures: %s", raw)
}

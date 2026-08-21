package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMentionsAndReactions asks where mentions and reactions live on a
// message. It reads only.
//
// No identity is printed: mentioned people are reported as a COUNT and the
// servers of their jids, never the jids.
func TestProbeMentionsAndReactions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGREAD") == "" {
		t.Skip("set WA_PROBE_MSGREAD=1")
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

	var raw string
	script := `JSON.stringify((() => {
		const out = {};
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		const all = MC.getModelsArray();
		out.total = all.length;

		// WHICH FIELDS EXIST AT ALL, counted across the whole collection: a
		// field that is present on zero messages is a field that does not exist
		// on this build, and one present on a few is the one to read.
		const seen = {};
		let withMentions = 0, withReactions = 0;
		for (const m of all) {
			try {
				for (const k of Object.keys(m)) {
					if (/mention|reaction/i.test(k)) { seen[k] = (seen[k] || 0) + 1; }
				}
				const ml = m.mentionedJidList;
				if (ml && ml.length) { withMentions++; }
				if (m.hasReaction) { withReactions++; }
			} catch (e) {}
		}
		out.fields = seen;
		out.withMentionedJidList = withMentions;
		out.withHasReaction = withReactions;

		// One mentioning message, described without naming anybody.
		for (const m of all) {
			try {
				const ml = m.mentionedJidList;
				if (ml && ml.length) {
					out.sampleMention = {
						count: ml.length,
						servers: ml.map(w => (w && w.server) || typeof w),
						fromMe: !!(m.id && m.id.fromMe)
					};
					break;
				}
			} catch (e) {}
		}
		// And one reacted message.
		for (const m of all) {
			try {
				if (m.hasReaction) {
					const U = window.require('WAWebReactionsUtils');
					out.sampleReaction = {
						utilKeys: Object.keys(U).slice(0, 12),
						fromMe: !!(m.id && m.id.fromMe)
					};
					break;
				}
			} catch (e) {}
		}
		try {
			const R = window.require('WAWebReactionsCollection');
			const coll = R.ReactionsCollection || R;
			out.reactionsCollection = (coll && coll.getModelsArray) ? coll.getModelsArray().length : 'no array';
		} catch (e) { out.reactionsCollection = 'THREW'; }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/msgread", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}

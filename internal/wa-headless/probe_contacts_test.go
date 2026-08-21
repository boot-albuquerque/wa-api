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

	// THE REACTION ROW, read instead of guessed.
	//
	// hasReaction is sticky within a session; getReactionEmojisAndSum did not
	// return the {sum} I assumed. Two guesses is one too many, so this reads the
	// actual state: the account holds a message that was reacted to and then
	// un-reacted, which is exactly the case that needs distinguishing.
	//
	// No emoji and no identity leave the page — only lengths and flags.
	const script = `JSON.stringify((() => {
		const out = {};
		const coll = window.require('WAWebMsgCollection').MsgCollection;
		const RC = window.require('WAWebReactionsCollection').ReactionsCollection;
		const U = window.require('WAWebReactionsUtils');

		out.reactionRows = RC.getModelsArray().length;
		const rows = RC.getModelsArray().slice(0, 3).map(r => {
			const o = { keys: Object.keys(r).filter(k => k.indexOf('__x_') === 0).slice(0, 20).join(',') };
			try {
				const s = r.senders && typeof r.senders.getModelsArray === 'function'
					? r.senders.getModelsArray() : [];
				o.senders = s.map(x => ({
					emojiLen: (x.reactionText || '').length,
					hasEmoji: !!(x.reactionText && x.reactionText.length),
					ack: (typeof x.ack === 'number') ? x.ack : null
				}));
			} catch (e) { o.sendersErr = String((e && e.message) || e).slice(0, 100); }
			try { o.aggCount = r.aggregateEmoji ? r.aggregateEmoji.length : -1; } catch (e) {}
			return o;
		});
		out.rows = rows;

		// What do the display helpers actually RETURN? Shape, not content.
		for (const m of coll.getModelsArray()) {
			try {
				if (!m.hasReaction) { continue; }
				out.sample = { sticky: true };
				for (const fn of ['getReactionEmojisAndSum', 'getReactionAggregates', 'getReactionForDisplay']) {
					try {
						const v = U[fn] && U[fn](m);
						out.sample[fn] = (v === undefined) ? 'undefined'
							: (v === null ? 'null'
							: (Array.isArray(v) ? 'array/' + v.length
							: (typeof v === 'object' ? Object.keys(v).join(',') : typeof v + ':' + String(v).slice(0, 20))));
					} catch (e) { out.sample[fn] = 'THREW: ' + String((e && e.message) || e).slice(0, 60); }
				}
				break;
			} catch (e) {}
		}
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/chats", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("reaction rows: %s", raw)
}

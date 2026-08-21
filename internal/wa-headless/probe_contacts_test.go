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

	// THE NEXT BATCH OF MESSAGE OPERATIONS, read before design.
	//
	// Three capabilities behave differently and each has been guessed wrong by
	// somebody: sendConversationSeen, sendReactionToMsg and the quoted-message
	// helper. The last four capabilities all cost a correction at exactly this
	// step when the argument shape was assumed (H40, H46, H49) — so this reads.
	const script = `JSON.stringify((() => {
		const out = {};
		const read = (mod, names) => {
			let m = null;
			try { m = window.require(mod); } catch (e) { out[mod] = 'REQUIRE_FAILED'; return; }
			if (!m) { out[mod] = 'NULL'; return; }
			const bag = {};
			for (const n of (names || Object.keys(m))) {
				try {
					const f = m[n];
					bag[n] = (typeof f === 'function')
						? { arity: f.length, src: String(f).slice(0, 330) }
						: { kind: typeof f };
				} catch (e) { bag[n] = 'THREW'; }
			}
			out[mod] = bag;
		};
		read('WAWebChatSendConversationSeen', ['sendConversationSeen']);
		read('WAWebSendReactionMsgAction', ['sendReactionToMsg']);
		read('WAWebQuotedMsgModelUtils', ['createQuotedMsgObj']);
		read('WAWebSendTextMsgChatAction', ['sendTextMsgToChat']);
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/chats", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("op signatures: %s", raw)
}

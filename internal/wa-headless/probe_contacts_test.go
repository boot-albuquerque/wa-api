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

	// CHAT STATE: archive, pin, mute — signatures read before design.
	//
	// The action layer is preferred over the bridges, the same choice groups and
	// presence made: the actions carry the app's own guards, and reaching past
	// them is deciding we know better than the app.
	//
	// Pin has a LIMIT, which the bridge exposes. A capability that ignored it
	// would fail at the page for a reason the caller could have been told.
	const script = `JSON.stringify((() => {
		const out = {};
		const read = (mod) => {
			let m = null;
			try { m = window.require(mod); } catch (e) { out[mod] = 'ABSENT'; return; }
			if (!m) { out[mod] = 'NULL'; return; }
			const bag = {};
			for (const n of Object.keys(m)) {
				try {
					const f = m[n];
					bag[n] = (typeof f === 'function')
						? { arity: f.length, src: String(f).slice(0, 260) } : { kind: typeof f };
				} catch (e) { bag[n] = 'THREW'; }
			}
			out[mod] = bag;
		};
		read('WAWebSetArchiveChatAction');
		read('WAWebSetPinChatAction');
		read('WAWebChatMuteBridge');
		read('WAWebChatPinBridge');
		// The pin limit and how many are used, which decides whether a refusal
		// can be explained before the page refuses.
		try {
			const B = window.require('WAWebChatPinBridge');
			out.pinLimit = B.getPinLimit ? B.getPinLimit() : 'n/a';
			out.pinnedNow = B.getNumConversationsPinned ? B.getNumConversationsPinned() : 'n/a';
		} catch (e) { out.pinErr = String((e && e.message) || e).slice(0, 120); }
		// Is there a mute ACTION, or only the bridge?
		for (const n of ['WAWebSetMuteChatAction', 'WAWebMuteChatAction', 'WAWebSendChatMuteAction']) {
			try { const m = window.require(n); out[n] = m ? Object.keys(m).join(',') : 'NULL'; }
			catch (e) { out[n] = 'ABSENT'; }
		}
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/chats", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("chat state surface: %s", raw)
}

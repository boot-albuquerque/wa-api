package headless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeForwardShape reads before experimenting, and it reads the CHAT MODEL
// first because that layer won twice already: it was the honest surface for
// starring and for muting, both times after the module the enumeration
// nominated turned out to be the wrong one.
//
// No outward effect: it reads sources and keys, it forwards nothing.
func TestProbeForwardShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_FORWARD") == "" {
		t.Skip("set WA_PROBE_FORWARD=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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
		const CC = window.require('WAWebChatCollection').ChatCollection;
		let chat = null;
		for (const c of CC.getModelsArray()) {
			try { if (c && c.id && c.id.server !== 'g.us') { chat = c; break; } } catch (e) {}
		}
		if (!chat) { out.why = 'NO_CHAT'; return out; }

		out.chatForwardMethods = Object.keys(Object.getPrototypeOf(chat))
			.filter(k => /forward/i.test(k));
		for (const k of out.chatForwardMethods) {
			try { out['chat_' + k] = String(chat[k]).slice(0, 500); } catch (e) {}
		}

		const src = (mod, fn) => {
			try {
				const m = window.require(mod);
				const f = m && m[fn];
				return typeof f === 'function' ? String(f).slice(0, 500) : ('NOT_A_FUNCTION:' + typeof f);
			} catch (e) { return 'THREW:' + String((e && e.message) || e).slice(0, 100); }
		};
		out.forwardMessages = src('WAWebChatForwardMessage', 'forwardMessages');
		out.getForwardedMessageFields = src('WAWebChatForwardMessage', 'getForwardedMessageFields');
		out.forwardMessagesToChats = src('WAWebForwardMessagesToChat', 'forwardMessagesToChats');
		try {
			out.forwardModuleKeys = Object.keys(window.require('WAWebChatForwardMessage'));
		} catch (e) { out.forwardModuleKeys = 'THREW'; }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/forward", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}

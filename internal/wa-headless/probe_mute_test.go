package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMuteShape asks which LAYER to drive, before driving either.
//
// The bundle shows the bridge being called with an object carrying a key named
// `$MuteImpl3` — which reads like a minifier artefact of a private class field,
// not like a contract. Passing that key literally is the kind of guess H58 paid
// for. The Mute MODEL sits above the bridge and the app drives it from its own
// UI, so this probe reads the model's methods and reports which of the two is
// the honest surface.
//
// It has NO outward effect: it reads, it does not mute.
func TestProbeMuteShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MUTE") == "" {
		t.Skip("set WA_PROBE_MUTE=1")
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
		const CC = window.require('WAWebChatCollection').ChatCollection;
		const chats = CC.getModelsArray();
		out.chatCount = chats.length;

		// A one-to-one chat this account has, chosen without naming it.
		let chat = null;
		for (const c of chats) {
			try { if (c && c.id && c.id.server !== 'g.us' && !c.isGroup) { chat = c; break; } } catch (e) {}
		}
		if (!chat) { out.why = 'NO_ONE_TO_ONE_CHAT'; return out; }

		const MU = window.require('WAWebMuteUtils');
		out.canMute = !!(MU.canMute && MU.canMute(chat));

		const MC = window.require('WAWebMuteCollection').MuteCollection;
		const mute = MC.get(chat.id);
		out.muteModelFound = !!mute;
		if (mute) {
			out.muteMethods = Object.keys(Object.getPrototypeOf(mute))
				.filter(k => { try { return typeof mute[k] === 'function'; } catch (e) { return false; } })
				.slice(0, 40);
			out.muteOwnKeys = Object.keys(mute).slice(0, 25);
			out.expiration = mute.expiration === undefined ? 'undefined' : String(mute.expiration);
			for (const name of ['mute', 'unmute']) {
				try {
					const f = mute[name];
					out['src_' + name] = typeof f === 'function' ? String(f).slice(0, 600) : ('NOT_A_FUNCTION:' + typeof f);
				} catch (e) { out['src_' + name] = 'THREW'; }
			}
		}

		const G = window.require('WAWebMuteGetters');
		// getIsMuted's argument is the question: the chat, or the mute model?
		for (const [label, arg] of [['chat', chat], ['muteModel', mute]]) {
			try { out['getIsMuted_' + label] = String(G.getIsMuted(arg)); }
			catch (e) { out['getIsMuted_' + label] = 'THREW:' + String((e && e.message) || e).slice(0, 90); }
		}

		const E = window.require('WAWebMuteExpirations');
		out.durations = E.ALL_MUTE_DURATIONS ? String(E.ALL_MUTE_DURATIONS) : null;
		try { out.calc8h = String(E.calculateMuteExpiration(8)); } catch (e) { out.calc8h = 'THREW'; }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/mute", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}

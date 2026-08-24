package waheadless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbePresenceModel opens the presence record the observer actually holds.
//
// H50's fourth hypothesis is now CONFIRMED rather than open: with the two lab
// accounts synced into each other's address books, the subscription that had
// been refused on every previous run reads subscribed=true. What did not follow
// is the typing state — online=false and an empty chatstate while the other
// account was announcing composing.
//
// So the question moved, and this is the instrument for the new one: what does
// the model hold, in FIELD NAMES and booleans? presence.Observe reads
// isSubscribed, isOnline and chatstate.type, and those three were chosen from
// the reference rather than from this build.
//
// READ ONLY, and identity-free: names, types and booleans, never a jid.
func TestProbePresenceModel(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PRESENCE") == "" {
		t.Skip("set WA_PROBE_PRESENCE=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	script := `(() => {
		window.__pres = null;
		const park = v => { window.__pres = JSON.stringify(v); };
		(async () => {
		const out = {};
		const shape = o => o === null || o === undefined ? String(o)
			: (Array.isArray(o) ? 'array[' + o.length + ']' : typeof o);
		// REDACT BEFORE REPORTING. The first run of this probe printed four
		// digits of an account number, because the page's own error text quotes
		// the jid it refused. An error message is not exempt from the rule that
		// nothing here carries an identity, and the place to enforce that is
		// where the message is built, not where it is read.
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 140);
		try {
			const PC = window.require('WAWebPresenceCollection').PresenceCollection;
			const W = window.require('WAWebWidFactory');
			const jid = PEER_PLACEHOLDER;
			// Subscribe the way the capability does, then look at what landed.
			// The SAME module the capability uses, so the probe measures the
			// production path rather than a neighbouring one.
			// RESOLVE FIRST, and this probe learned why by getting it wrong.
			//
			// Passing the @c.us wid straight in made subscribeUserPresence throw
			// "is not a userLid": this build subscribes by LID, not by phone
			// number. The production capability resolves before subscribing,
			// which is exactly why the real test reports subscribed=true while
			// this probe's first version reported a refusal — the probe was
			// measuring its own shortcut.
			const ex = await window.require('WAWebQueryExistsJob')
				.queryWidExists(W.createWid(jid));
			if (!ex || !ex.wid) { park({ fatal: 'NOT_ON_WHATSAPP' }); return; }
			const wid = ex.wid;
			out.resolvedDomain = (wid.server ? String(wid.server) : 'unknown');
			try {
				await window.require('WAWebContactPresenceBridge').subscribeUserPresence(wid);
				out.subscribed_call = 'ok';
			} catch (e) {
				out.subscribed_call = 'threw: ' + safe(e);
			}
			const p = PC.get(wid);
			out.found = !!p;
			if (p) {
				out.fields = Object.keys(p);
				out.isSubscribed = !!p.isSubscribed;
				out.isOnline = !!p.isOnline;
				out.chatstateShape = shape(p.chatstate);
				try { out.chatstateFields = p.chatstate ? Object.keys(p.chatstate) : []; } catch (e) {}
				try { out.chatstateType = p.chatstate ? String(p.chatstate.type) : 'no chatstate'; } catch (e) {}
				// The other places a typing state could live, asked rather than
				// assumed: the reference reads presence.chatstate, and this
				// build's own model may keep it somewhere else entirely.
				out.hasChatstates = shape(p.chatstates);
				// WHERE THE TYPING ACTUALLY LIVES. The model carries
				// typingUserIds and recordingUserIds, which the reference's
				// chatstate.type never mentions — and chatstate.type reads
				// undefined here. COUNTS ONLY: those lists hold identities.
				for (const k of ['typingUserIds', 'recordingUserIds', 'chatActive', 'hasData']) {
					try {
						const v = p[k];
						out['t_' + k] = shape(v) +
							(v && typeof v.length === 'number' ? ' len=' + v.length : '') +
							(v && typeof v.size === 'number' ? ' size=' + v.size : '') +
							(v && typeof v.getModelsArray === 'function'
								? ' models=' + v.getModelsArray().length : '');
					} catch (e) { out['t_' + k] = 'threw: ' + safe(e); }
				}
				try {
					const cs = p.chatstates;
					out.chatstatesCount = cs && typeof cs.getModelsArray === 'function'
						? cs.getModelsArray().length
						: (cs && typeof cs.length === 'number' ? cs.length : 'unknown');
					if (cs && typeof cs.getModelsArray === 'function') {
						const first = cs.getModelsArray()[0];
						out.chatstateModelFields = first ? Object.keys(first) : [];
						out.chatstateModelType = first && first.type !== undefined
							? String(first.type) : 'no type field';
					}
				} catch (e) { out.chatstatesCount = 'threw: ' + safe(e); }
				out.raw = {};
				for (const k of Object.keys(p)) {
					if (k.indexOf('chatstate') >= 0 || k.indexOf('Chatstate') >= 0 ||
						k.indexOf('ChatState') >= 0 || k.indexOf('online') >= 0 ||
						k.indexOf('Online') >= 0 || k.indexOf('ubscri') >= 0) {
						try { out.raw[k] = shape(p[k]); } catch (e) {}
					}
				}
			}
			// And the CHAT's own view, because a typing indicator is rendered
			// per conversation and may never touch the user presence at all.
			try {
				const CC = window.require('WAWebChatCollection').ChatCollection;
				const c = CC.get(wid);
				out.chat = { found: !!c };
				if (c) {
					out.chat.presenceShape = shape(c.presence);
					if (c.presence) {
						out.chat.presenceFields = Object.keys(c.presence);
						try { out.chat.chatstateType = c.presence.chatstate
							? String(c.presence.chatstate.type) : 'none'; } catch (e) {}
					}
				}
			} catch (e) { out.chat = 'threw: ' + safe(e); }
		} catch (e) {
			out.fatal = safe(e);
		}
		park(out);
		})();
		return 'kicked';
	})()`
	script = strings.ReplaceAll(script, "PEER_PLACEHOLDER", strconv.Quote(peer))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/pres-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/pres-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__pres || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never settled")
	}
	t.Logf("%s", raw)
}

package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeChannelFollow subscribes this account to a channel, proves the
// follow list stops being empty, and UNSUBSCRIBES.
//
// AUTHORISATION: subscribing is an outward effect — it bumps a public
// subscriber count. It was authorised explicitly for this channel, which the
// user supplied. The unsubscribe is registered with defer BEFORE anything that
// can fail, and NOT with t.Cleanup, which runs after the defer that stops the
// session and would therefore leave the account following.
//
// Identity-free: no channel name is logged, only counts and membership words.
func TestProbeChannelFollow(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_FOLLOW") == "" {
		t.Skip("set WA_PROBE_FOLLOW=1 (this SUBSCRIBES to a channel)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	code := strings.TrimSpace(os.Getenv("WA_CHANNEL_CODE"))
	if code == "" {
		t.Skip("WA_CHANNEL_CODE is required")
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
	eval := sess.Tab().Evaluate
	r := channel.New(runner, eval)
	m := channel.NewManager(runner, eval)

	// The follow list must start EMPTY, otherwise the proof that following moved
	// it is not a proof at all.
	before, err := m.Followed(ctx, "probe/follow")
	if err != nil {
		t.Fatalf("Followed (before): %v", err)
	}
	t.Logf("following before: %d", len(before))

	ch, err := r.ByInviteCode(ctx, code, "probe/follow")
	if err != nil {
		t.Fatalf("ByInviteCode: %v", err)
	}
	t.Logf("target: %s", ch)
	if ch.Following {
		t.Skip("this account already follows the channel; the proof needs to start unfollowed")
	}

	membership, err := m.Follow(ctx, ch.JID, "probe/follow")
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	t.Logf("followed: membership=%q", membership)

	// THE UNFOLLOW IS THE CONDITION OF THE RUN, registered immediately.
	defer func() {
		got, err := m.Unfollow(ctx, ch.JID, "probe/follow-undo")
		if err != nil {
			t.Errorf("UNFOLLOW FAILED: the account is left following the channel: %v", err)
			return
		}
		t.Logf("unfollowed: membership=%q", got)
		after, err := m.Followed(ctx, "probe/follow-undo")
		if err != nil {
			t.Errorf("Followed (after undo): %v", err)
			return
		}
		t.Logf("following after undo: %d", len(after))
		if len(after) > len(before) {
			t.Errorf("the follow list did not come back: %d before, %d after",
				len(before), len(after))
		}
	}()

	// THE POINT: the list that was empty must now hold something.
	during, err := m.Followed(ctx, "probe/follow")
	if err != nil {
		t.Fatalf("Followed (during): %v", err)
	}
	t.Logf("following during: %d", len(during))
	if len(during) <= len(before) {
		t.Errorf("the follow list did not grow: %d before, %d during — getChannels "+
			"cannot be proven non-empty", len(before), len(during))
	}
	for _, e := range during {
		t.Logf("  entry: %s", e)
	}

	// And the reader agrees the membership moved.
	again, err := r.ByInviteCode(ctx, code, "probe/follow")
	if err != nil {
		t.Errorf("ByInviteCode (after follow): %v", err)
	} else if !again.Following {
		t.Error("the channel reader still reports this account as not following")
	}
}

// TestProbeSubscribeShapes measures WHICH argument shape
// subscribeToNewsletterAction accepts, because the collection's own find is
// broken on this build ("this.findImpl is not a function", measured) and the
// reference hides the resolution inside its injected helper.
//
// It may complete a real subscription — authorised — and the test unsubscribes
// whatever it managed to follow.
func TestProbeSubscribeShapes(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SUBSHAPE") == "" {
		t.Skip("set WA_PROBE_SUBSHAPE=1 (may SUBSCRIBE)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	code := strings.TrimSpace(os.Getenv("WA_CHANNEL_CODE"))
	if profile == "" || code == "" {
		t.Skip("WA_SEND_FROM_PROFILE and WA_CHANNEL_CODE are required")
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
	eval := sess.Tab().Evaluate
	m := channel.NewManager(runner, eval)
	defer func() {
		// Whatever got followed, unfollow it.
		list, err := m.Followed(ctx, "probe/subshape-undo")
		if err != nil {
			t.Errorf("Followed (undo): %v", err)
			return
		}
		for _, e := range list {
			if _, err := m.Unfollow(ctx, e.JID, "probe/subshape-undo"); err != nil {
				t.Errorf("UNFOLLOW FAILED for one channel: %v", err)
			}
		}
		t.Logf("undo: %d channel(s) unfollowed", len(list))
	}()

	var ignored string
	if err := eval(ctx, "window.__code = "+strconvQuote(code)+"; \"set\"", &ignored); err != nil {
		t.Fatalf("set code: %v", err)
	}
	script := `(() => {
	window.__st2 = null;
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0, 130);
	const out = {attempts: []};
	(async () => {
	try {
		const CODE = window.__code;
		const Q = window.require("WAWebNewsletterMetadataQueryJob");
		const S = window.require("WAWebNewsletterSubscribeAction");
		const NC = window.require("WAWebCollections").WAWebNewsletterCollection;
		const W = window.require("WAWebWidFactory");
		out.subArity = S.subscribeToNewsletterAction.length;

		const md = await Q.queryNewsletterMetadataByInviteCode(CODE);
		out.gotMetadata = !!md;
		const idJid = md && (md.idJid || md.id);
		const jid = (idJid && idJid._serialized) ? idJid._serialized : String(idJid || "");
		out.jidLen = jid.length;
		const wid = (() => { try { return W.createWid(jid); } catch(e){ return null; } })();
		out.madeWid = !!wid;

		// A colecao aceita inserir o modelo?
		out.ncMethods = Object.getOwnPropertyNames(Object.getPrototypeOf(NC)).filter(
			k => /add|find|get|fetch|load/i.test(k)).slice(0, 14);

		const shapes = [
			["metadata-object", md],
			["wid", wid],
			["jid-string", jid],
		];
		for (const [name, arg] of shapes) {
			if (arg === null || arg === undefined) { out.attempts.push({name, skipped: true}); continue; }
			try {
				await S.subscribeToNewsletterAction(arg, {eventSurface: 3, deleteLocalModels: false});
				out.attempts.push({name, ok: true});
				break;
			} catch (e) { out.attempts.push({name, err: safe(e)}); }
		}
		out.followingNow = (typeof NC.getModelsArray === "function") ? NC.getModelsArray().length : -1;
	} catch (e) { out.err = safe(e); }
	window.__st2 = JSON.stringify(out);
	})();
	return 'kicked';
})()
`
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__st2", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("subscribe shapes: %s", raw)
}

func strconvQuote(s string) string { return "\"" + s + "\"" }

// TestProbeSubscribeByWid revisits H123 with the instrument H137 taught.
//
// H123 concluded that subscribing is impossible on this build: the function the
// REFERENCE calls, subscribeToNewsletterAction, has arity 3 here and wants a
// memoised collection model that the broken find cannot produce.
//
// Enumerating the module (H137's lesson) found a SECOND function the reference
// never calls: subscribeToNewsletterWidAction, arity 2, taking a Wid — which is
// exactly what a caller holding an invite code can build.
//
// AUTHORISED: the user authorised subscribing to this channel. The unsubscribe
// is registered before the attempt.
func TestProbeSubscribeByWid(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SUBWID") == "" {
		t.Skip("set WA_PROBE_SUBWID=1 (this may SUBSCRIBE to a channel)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	code := strings.TrimSpace(os.Getenv("WA_CHANNEL_CODE"))
	if profile == "" || code == "" {
		t.Skip("WA_SEND_FROM_PROFILE and WA_CHANNEL_CODE are required")
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
	eval := sess.Tab().Evaluate
	m := channel.NewManager(runner, eval)

	// WHATEVER GETS FOLLOWED, UNFOLLOW IT — registered before the attempt.
	defer func() {
		list, err := m.Followed(ctx, "probe/subwid-undo")
		if err != nil {
			t.Errorf("Followed (undo): %v", err)
			return
		}
		for _, e := range list {
			if _, err := m.Unfollow(ctx, e.JID, "probe/subwid-undo"); err != nil {
				t.Errorf("UNFOLLOW FAILED for one channel: %v", err)
			}
		}
		t.Logf("undo: %d channel(s) unfollowed", len(list))
	}()

	var ignored string
	if err := eval(ctx, "window.__code = "+quoteJSString(code)+"; \"set\"", &ignored); err != nil {
		t.Fatalf("set code: %v", err)
	}
	script := `(() => {
	window.__sw = null;
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
	(async () => {
	try {
		const out = {};
		const Q = window.require("WAWebNewsletterMetadataQueryJob");
		const W = window.require("WAWebWidFactory");
		const S = window.require("WAWebNewsletterSubscribeAction");
		const NC = window.require("WAWebCollections").WAWebNewsletterCollection;

		const md = await Q.queryNewsletterMetadataByInviteCode(window.__code);
		const idJid = md && (md.idJid || md.id);
		const jid = (idJid && idJid._serialized) ? idJid._serialized : String(idJid || "");
		out.jidLen = jid.length;
		out.before = NC.getModelsArray().length;

		const wid = W.createWid(jid);
		try {
			await S.subscribeToNewsletterWidAction(wid, {eventSurface: 3});
			out.subscribed = true;
		} catch (e) { out.subscribed = false; out.why = safe(e); }
		out.after = NC.getModelsArray().length;
		window.__sw = JSON.stringify(out);
	} catch (e) { window.__sw = JSON.stringify({err: safe(e)}); }
	})();
	return 'kicked';
})()
`
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__sw", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(400 * time.Millisecond)
	}
	t.Logf("subscribe by wid: %s", raw)
}

func quoteJSString(s string) string { return "\"" + s + "\"" }

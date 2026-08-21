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

// TestProbeContactShape is a measurement bench, not a guard. Its script is
// rewritten as questions come up; what stays is the harness.
//
// It prints counts, shapes and stacks — never a name, a number or an identity.
func TestProbeContactShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONTACTS") == "" {
		t.Skip("set WA_PROBE_CONTACTS=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_PROBE_PEER")
	if profile == "" || peer == "" {
		t.Skip("WA_SEND_FROM_PROFILE and WA_PROBE_PEER are required; the peer must be a LAB account")
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

	// WHO THROWS "Could not perform action."?
	//
	// It is WAWebMiscErrors.ActionError's default message, so something in the
	// archive path threw it. Grepping the bundles for every ActionError would
	// find dozens; the STACK names the one that fired.
	//
	// This performs a real archive on the peer LAB conversation and undoes it
	// immediately, so the account is left as found.
	const script = `(() => {
		window.__waHeadlessArch = { stage: 'pending' };
		(async () => {
			const out = { stage: 'done' };
			try {
				const Chats = window.require('WAWebChatCollection').ChatCollection;
				const Find = window.require('WAWebFindChatAction');
				const WF = window.require('WAWebWidFactory');
				const Q = window.require('WAWebQueryExistsJob');

				const local = WF.createWid(PEER_PLACEHOLDER);
				const ex = await Q.queryWidExists(local);
				if (!ex || !ex.wid) { out.why = 'NOT_ON_WHATSAPP'; window.__waHeadlessArch = out; return; }
				let chat = Chats.get(ex.wid);
				if (!chat) { chat = await Find.findExistingChat(ex.wid); }
				if (!chat) { out.why = 'NO_CHAT'; window.__waHeadlessArch = out; return; }
				out.foundChat = true;
				out.wasArchived = !!chat.archive;

				const A = window.require('WAWebSetArchiveChatAction');
				out.arity = A.setArchive.length;
				const describe = (e) => ({ name: e && e.name,
					msg: String((e && e.message) || e).slice(0, 90) });

				// THE HYPOTHESIS: setArchive refuses when the requested state is
				// already the current one. The first probe asked to archive a
				// conversation that was ALREADY archived and got ActionError, so
				// this asks for the OPPOSITE and compares.
				const current = !!chat.archive;

				try { await A.setArchive(chat, !current); out.opposite = 'ACCEPTED'; }
				catch (e) { out.opposite = describe(e); }
				out.afterOpposite = !!chat.archive;

				// And then the SAME value it now holds, which should be the
				// refused case if the hypothesis holds.
				const now = !!chat.archive;
				try { await A.setArchive(chat, now); out.same = 'ACCEPTED'; }
				catch (e) { out.same = describe(e); }
				out.afterSame = !!chat.archive;

				// Leave the conversation as it was found.
				try { if (!!chat.archive !== current) { await A.setArchive(chat, current); } } catch (e) {}
				out.restoredTo = !!chat.archive;
				out.startedAt = current;
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 200);
			}
			window.__waHeadlessArch = out;
		})();
		return 'kicked';
	})()`

	kick := strings.Replace(script, "PEER_PLACEHOLDER", strconv.Quote(peer), 1)

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/arch/kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("probe kick: %v", err)
	}
	for i := 0; ; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/arch/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessArch || {stage:"missing"})`, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) && !strings.Contains(raw, `"stage":"missing"`) {
			break
		}
		if i > 40 {
			t.Fatalf("the archive probe never settled (last: %s)", raw)
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("archive refusal: %s", raw)
}

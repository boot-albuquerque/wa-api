package waheadless

import (
	"context"
	"os"
	"strings"
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
	// DOES THE EXISTING SEND PATH WORK FOR A GROUP?
	//
	// resolveChatExpr goes through queryWidExists, which resolves USERS. A
	// group jid may not survive it, and if it does not, the send capabilities
	// silently only work for individuals — a gap nobody would notice until a
	// message to a group failed in production.
	//
	// NOTHING IS SENT. The resolution is exercised up to the point of dispatch
	// and stops there: a group is real people, and measuring must not message
	// them. Only counts and shapes leave the page.
	const script = `(() => {
		window.__waHeadlessGroupProbe = { stage: 'pending' };
		(async () => {
			const out = { stage: 'done' };
			try {
				const Chats = window.require('WAWebChatCollection').ChatCollection;
				const all = Chats.getModelsArray();
				out.chats = all.length;
				const groups = all.filter(c => {
					try { return c.id && c.id.server === 'g.us'; } catch (e) { return false; }
				});
				out.groups = groups.length;
				if (!groups.length) { out.why = 'NO_GROUP_CHAT'; window.__waHeadlessGroupProbe = out; return; }

				const g = groups[0];
				out.groupIdShape = {
					server: g.id.server,
					userLen: (g.id.user || '').length,
					hasSerialized: typeof g.id._serialized === 'string'
				};

				// Step 1: does createWid survive a group jid?
				const WF = window.require('WAWebWidFactory');
				let local = null;
				try { local = WF.createWid(g.id._serialized); } catch (e) { out.createWidErr = String((e && e.message) || e).slice(0, 120); }
				out.createWidOk = !!local;
				if (local) { out.createWidServer = local.server; }

				// Step 2: does queryWidExists — the USER resolution — accept it?
				if (local) {
					try {
						const Q = window.require('WAWebQueryExistsJob');
						const ex = await Q.queryWidExists(local);
						out.queryWidExists = ex ? { hasWid: !!ex.wid, server: ex.wid && ex.wid.server } : 'NULL';
					} catch (e) {
						out.queryWidExistsErr = String((e && e.message) || e).slice(0, 160);
					}
				}

				// Step 3: can the chat be obtained directly, skipping resolution?
				try {
					const got = Chats.get(g.id);
					out.chatCollectionGet = !!got;
				} catch (e) { out.chatGetErr = String((e && e.message) || e).slice(0, 120); }
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 180);
			}
			window.__waHeadlessGroupProbe = out;
		})();
		return 'kicked';
	})()`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/group/kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe kick: %v", err)
	}
	for i := 0; ; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/group/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessGroupProbe || {stage:"missing"})`, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) {
			break
		}
		if i > 40 {
			t.Fatal("the group probe never settled")
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("group send path: %s", raw)
}

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

	// THE GROUP METADATA IS NOT LOADED, and that is why the invite code could
	// not be read: queryGroupInviteCode reads iAmAdmin off the metadata, and
	// the chat carries the flag while chat.groupMetadata is undefined.
	//
	// So the question is how the app loads it. Codes are CREDENTIALS — anyone
	// holding one can join — so nothing here prints a code, only lengths.
	const script = `(() => {
		window.__waHeadlessArch = { stage: 'pending' };
		(async () => {
			const out = { stage: 'done' };
			try {
				const Chats = window.require('WAWebChatCollection').ChatCollection;
				let lab = null;
				for (const c of Chats.getModelsArray()) {
					try {
						if (c.id && c.id.server === 'g.us' &&
							typeof c.formattedTitle === 'string' &&
							c.formattedTitle.indexOf('wa-headless-lab') === 0) { lab = c; break; }
					} catch (e) {}
				}
				if (!lab) { out.why = 'NO_LAB_GROUP'; window.__waHeadlessArch = out; return; }
				out.metadataBefore = !!lab.groupMetadata;

				// Is the metadata in its own collection, keyed by the group id?
				try {
					const GM = window.require('WAWebGroupMetadataCollection');
					const coll = GM.GroupMetadataCollection || GM.default || GM;
					out.metaCollKeys = Object.keys(GM).slice(0, 8).join(',');
					if (coll && typeof coll.get === 'function') {
						const md = coll.get(lab.id);
						out.inCollection = !!md;
						if (md) { out.mdHasIAmAdmin = md.iAmAdmin !== undefined; }
					}
					if (coll && typeof coll.getModelsArray === 'function') {
						out.metaRows = coll.getModelsArray().length;
					}
					// A find/fetch entry point?
					for (const n of ['find', 'findQuery', 'fetch', 'update']) {
						out['coll_' + n] = typeof (coll && coll[n]);
					}
				} catch (e) { out.gmErr = String((e && e.message) || e).slice(0, 140); }

				// Modules whose names suggest they load it.
				for (const n of ['WAWebQueryGroupJob', 'WAWebGroupQueryJob', 'WAWebGroupMetadataUpdateJob', 'WAWebFetchGroupMetadataJob']) {
					try { const m = window.require(n); out[n] = m ? Object.keys(m).slice(0, 8).join(',') : 'NULL'; }
					catch (e) { out[n] = 'ABSENT'; }
				}
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 200);
			}
			window.__waHeadlessArch = out;
		})();
		return 'kicked';
	})()`

	kick := script
	_ = peer
	_ = strconv.Quote

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
	t.Logf("group metadata loading: %s", raw)
}

package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
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
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// PARTICIPANTS AND SUBJECT: does the corrected reading unlock them?
	//
	// The bet behind reading the layer was that whatever blocked the invite
	// blocked these too. This checks it — signatures only, NOTHING is added,
	// removed or renamed.
	const script = `JSON.stringify((() => {
		const out = {};
		try {
			const J = window.require('WAWebGroupParticipantsJob');
			const bag = {};
			for (const n of Object.keys(J)) {
				try {
					const f = J[n];
					bag[n] = (typeof f === 'function')
						? { arity: f.length, src: String(f).slice(0, 240) } : { kind: typeof f };
				} catch (e) {}
			}
			out.participantsJob = bag;
		} catch (e) { out.jobErr = String((e && e.message) || e).slice(0, 140); }
		// Where does the subject live? Try the names an app would use.
		for (const n of ['WAWebSetGroupSubjectAction', 'WAWebGroupSubjectAction',
			'WAWebSendSetGroupSubjectJob', 'WAWebSetGroupSubjectJob', 'WAWebGroupSetSubjectJob',
			'WAWebGroupPropertiesJob', 'WAWebSetGroupPropertyJob']) {
			try { const m = window.require(n); out[n] = m ? Object.keys(m).join(',') : 'NULL'; }
			catch (e) { out[n] = 'ABSENT'; }
		}
		// And what the metadata itself offers, now that we know it is the
		// object these calls want.
		try {
			const Chats = window.require('WAWebChatCollection').ChatCollection;
			for (const c of Chats.getModelsArray()) {
				if (c.id && c.id.server === 'g.us' && typeof c.formattedTitle === 'string'
					&& c.formattedTitle.indexOf('wa-headless-lab') === 0) {
					const md = c.groupMetadata;
					out.metadataMethods = md ? Object.getOwnPropertyNames(Object.getPrototypeOf(md)).slice(0, 30).join(',') : 'none';
					out.participantsMethods = (md && md.participants)
						? Object.getOwnPropertyNames(Object.getPrototypeOf(md.participants)).slice(0, 25).join(',') : 'none';
					break;
				}
			}
		} catch (e) { out.mdErr = String((e && e.message) || e).slice(0, 120); }
		return out;
	})())`

	kick := script
	_ = peer
	_ = strconv.Quote

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/write", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("write surface: %s", raw)
}

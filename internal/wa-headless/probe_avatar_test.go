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

// TestProbeAvatarShape measures the avatar surface before fetchContactAvatar is
// designed. It never returns a URL: a profile-picture eurl identifies a person
// AND carries an access token, so it is exactly the kind of value the briefing
// keeps out of logs. Counts, field names and hosts only.
func TestProbeAvatarShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_AVATAR") == "" {
		t.Skip("set WA_PROBE_AVATAR=1")
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

	// Part 1: what does the local thumb collection already hold?
	const localScript = `JSON.stringify((() => {
		const out = {};
		const mod = window.require('WAWebProfilePicThumbCollection');
		const coll = mod && mod.ProfilePicThumbCollection;
		out.hasCollection = !!coll;
		if (!coll || typeof coll.getModelsArray !== 'function') { return out; }
		const all = coll.getModelsArray();
		out.total = all.length;
		const fieldHits = {}; const servers = {}; const hosts = {};
		let sampleKeys = '';
		for (let i = 0; i < all.length; i++) {
			const m = all[i];
			try {
				for (const f of ['eurl','img','imgFull','tag','id','previewEurl','raw','stale']) {
					const v = m[f];
					if (v !== undefined && v !== null && v !== '') { fieldHits[f] = (fieldHits[f]||0)+1; }
				}
				const id = m.id;
				if (id && id.server) { servers[id.server] = (servers[id.server]||0)+1; }
				// Host only. Never the path, which carries the token.
				const u = m.eurl || m.img;
				if (typeof u === 'string' && u.indexOf('http') === 0) {
					try { const h = new URL(u).host; hosts[h] = (hosts[h]||0)+1; } catch (e) {}
				}
			} catch (e) {}
			if (i === 0) { try { sampleKeys = Object.keys(m).filter(k => k.indexOf('__x_') === 0).slice(0,20).join(','); } catch (e) {} }
		}
		out.fieldHits = fieldHits; out.servers = servers; out.hosts = hosts;
		out.modelKeys = sampleKeys;
		const G = window.require('WAWebProfilePicThumbGetters');
		out.getters = Object.keys(G).slice(0, 20).join(',');
		const B = window.require('WAWebContactProfilePicThumbBridge');
		out.bridge = Object.keys(B).slice(0, 20).join(',');
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/avatar/local", func(c context.Context) error {
		return sess.Tab().Evaluate(c, localScript, &raw)
	}); err != nil {
		t.Fatalf("probe local: %v", err)
	}
	t.Logf("local thumbs: %s", raw)

	// Part 2: DOES THE SERVER ANSWER FOR A PHONE IDENTITY AT ALL?
	//
	// The live proof failed on exactly one of twelve contacts — the only
	// phone-ONLY one — with the request never settling and never throwing. The
	// earlier pass had reported "12 asked, 0 threw" and I read that as "both
	// identity spaces work". It did not say that: the probe never recorded
	// WHICH server it had asked for, so a sample that happened to be all lid
	// looked like a sample that covered both. The counter below exists so that
	// cannot happen again.
	//
	// Two questions, both with a timeout so a hang is DATA and not a stall:
	//   1. do phone identities hang as a class, or was that one contact?
	//   2. does resolving the phone to a lid first make it answer?
	const askScript = `(() => {
		window.__waHeadlessAvatarProbe = { stage: 'pending' };
		const withTimeout = (p, ms) => Promise.race([
			p.then(v => ({ outcome: 'answered', v: v })),
			new Promise(res => setTimeout(() => res({ outcome: 'hung' }), ms))
		]).catch(e => ({ outcome: 'threw', why: String((e && e.message) || e).slice(0, 90) }));
		(async () => {
			const out = { stage: 'done', byServer: {}, resolved: {} };
			try {
				const CC = window.require('WAWebContactCollection').ContactCollection;
				const B = window.require('WAWebContactProfilePicThumbBridge');
				const Q = window.require('WAWebQueryExistsJob');
				const all = CC.getModelsArray();
				const pick = (server, n) => {
					const r = [];
					for (let i = 0; i < all.length && r.length < n; i++) {
						const id = all[i].id;
						if (id && id.server === server) { r.push(id); }
					}
					return r;
				};
				for (const server of ['c.us', 'lid']) {
					const bucket = { answered: 0, hung: 0, threw: 0, withEurl: 0 };
					for (const wid of pick(server, 4)) {
						const r = await withTimeout(B.requestProfilePicFromServer({ id: wid }), 8000);
						bucket[r.outcome] = (bucket[r.outcome] || 0) + 1;
						if (r.outcome === 'answered' && r.v && r.v.eurl) { bucket.withEurl++; }
					}
					out.byServer[server] = bucket;
				}
				// Question 2: resolve the phone first, the way sending does.
				const bucket = { resolvedOk: 0, resolveFailed: 0, answered: 0, hung: 0, threw: 0, withEurl: 0 };
				for (const wid of pick('c.us', 4)) {
					let lid = null;
					try {
						const ex = await Q.queryWidExists(wid);
						if (ex && ex.wid) { lid = ex.wid; bucket.resolvedOk++; } else { bucket.resolveFailed++; }
					} catch (e) { bucket.resolveFailed++; }
					if (!lid) { continue; }
					const r = await withTimeout(B.requestProfilePicFromServer({ id: lid }), 8000);
					bucket[r.outcome] = (bucket[r.outcome] || 0) + 1;
					if (r.outcome === 'answered' && r.v && r.v.eurl) { bucket.withEurl++; }
				}
				out.resolved = bucket;
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 180);
			}
			window.__waHeadlessAvatarProbe = out;
		})();
		return 'kicked';
	})()`

	if err := runner.Do(ctx, engine.OpStateProbe, "probe/avatar/ask", func(c context.Context) error {
		return sess.Tab().Evaluate(c, askScript, &raw)
	}); err != nil {
		t.Fatalf("probe ask: %v", err)
	}
	for i := 0; ; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/avatar/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessAvatarProbe || {stage:"missing"})`, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) {
			break
		}
		if i > 60 {
			t.Fatal("the server request never settled")
		}
		time.Sleep(time.Second)
	}
	t.Logf("call shapes: %s", raw)
}

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

// TestProbeCalls measures the calls surface before anything is designed.
//
// The reference does two things here that are worth checking rather than
// copying. It rejects a call by hand-building a `call` stanza and casting it
// through WADeprecatedSendIq — a name that announces its own expiry — and it
// observes incoming calls by MONKEY-PATCHING an internal Map's set method on
// WAWebCallCollection, having apparently found no listener that fires.
//
// Both are the kind of thing that is either necessary or a workaround for a
// build that is not ours, and the only way to tell is to ask this page which
// doors are open.
//
// READ ONLY. Nothing is rejected, no link is created, no stanza is cast. The
// call collection is INSPECTED, not patched: a probe that installs a hook into
// a live app is not a probe.
func TestProbeCalls(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CALLS") == "" {
		t.Skip("set WA_PROBE_CALLS=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	const callsProbeScript = `(() => {
		window.__calls = null;
		const park = v => { window.__calls = JSON.stringify(v); };
		(async () => {
		const out = { modules: {}, arity: {}, collection: {}, alternatives: {} };
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m).slice(0, 40) : 'falsy';
				return m;
			} catch (e) {
				out.modules[name] = 'ABSENT: ' + (e && e.message);
				return null;
			}
		};
		try {
			const Wap = load('WAWap');
			const Iq = load('WADeprecatedSendIq');
			const Link = load('WAWebGenerateEventCallLink');
			const Me = load('WAWebUserPrefsMeUser');
			const CallColl = load('WAWebCallCollection');
			if (Link && Link.createEventCallLink) out.arity.createEventCallLink = Link.createEventCallLink.length;
			if (Iq && Iq.deprecatedCastStanza) out.arity.deprecatedCastStanza = Iq.deprecatedCastStanza.length;

			// IS THERE A LISTENER, or is the reference's Map patch the only
			// door? This is the question the whole event half turns on.
			if (CallColl) {
				const holder = CallColl.CallCollection || CallColl.default || CallColl;
				out.collection.holderKeys = Object.keys(holder || {}).slice(0, 40);
				out.collection.hasOn = typeof (holder && holder.on) === 'function';
				out.collection.hasModelsArray = typeof (holder && holder.getModelsArray) === 'function';
				out.collection.size = (holder && typeof holder.length === 'number') ? holder.length : 'no length';
				// The reference looks for a property that IS a Map and wraps
				// its set. Report whether such a property exists, by NAME.
				const mapKeys = [];
				for (const k of Object.keys(CallColl)) {
					try { if (CallColl[k] instanceof Map) mapKeys.push(k); } catch (e) {}
				}
				out.collection.mapProps = mapKeys;
			}

			// WHAT THE BUNDLE SCAN FOUND AND THE REFERENCE NEVER MENTIONS.
			// Enumerating every loadable module whose name contains "Call"
			// turned up a first-class VOIP surface here: this build can ORIGINATE
			// a call, cancel one, and create a plain call link — three things
			// whatsapp-web.js has no equivalent for. Their arities are what says
			// whether any of it is callable from here.
			for (const [name, fns] of [
				['WAWebVoipStartCall', ['startWAWebVoipCall', 'startWAWebVoipGroupCallFromChat', 'startWAWebVoipGroupCallFromWids']],
				['WAWebVoipCreateCallLink', ['createCallLink']],
				['WAWebVoipCreateCallLinkJob', ['createCallLinkJob']],
				['WAWebVoipCancelOutgoingCall', ['cancelPendingOutgoingCall']],
				['WAWebCallCollectionUtils', ['buildCallPropsFromOffer', 'assignCallPropsToCall', 'createCallModel']],
				['WAWebVoipCallStateUtils', ['isCallIncoming', 'isCallRinging', 'isCallTerminal']],
			]) {
				try {
					const m = window.require(name);
					const rec = {};
					for (const f of fns) {
						rec[f] = (m && typeof m[f] === 'function') ? m[f].length : 'absent';
					}
					out.voip = out.voip || {};
					out.voip[name] = rec;
				} catch (e) {
					out.voip = out.voip || {};
					out.voip[name] = 'absent';
				}
			}

			// THE ARGUMENT SHAPE OF createCallLink, asked by trying.
			//
			// Arity 1 here against the reference's two-argument
			// createEventCallLink, which is a DIFFERENT function for a different
			// thing (scheduled events). Nothing documents this one, and the
			// argument probe is the wrong instrument for a call that reaches the
			// server: a recording proxy would be handed to something that may
			// send before it reads.
			//
			// A call link is a URL and notifies nobody, so trying real shapes is
			// cheap and honest. THE LINK ITSELF IS NEVER REPORTED — only its
			// type and length, because a link anybody who reads a log can join
			// is exactly the shape of the invite-code rule.
			try {
				// TWO DIFFERENT FUNCTIONS, AND THE LEDGER ROW MEANS THE SECOND.
				//
				// WAWebVoipCreateCallLink.createCallLink is this build's own
				// VOIP call-link path and it HANGS on the first call — measured:
				// createCallLink('video') never settled in 40s, which is the same
				// class of answer as H57's invite hang. It needs a VOIP stack
				// this headless session apparently never brings up.
				//
				// whatsapp-web.js's Client.createCallLink does NOT call that. It
				// calls WAWebGenerateEventCallLink.createEventCallLink, which
				// belongs to scheduled events. That is the function the upstream
				// row is about, so it is the one measured here.
				const CL = window.require('WAWebGenerateEventCallLink');
				out.link = {};
				const shapes = {
					refShape: [START_TS_PLACEHOLDER, 'video'],
					refShapeVoice: [START_TS_PLACEHOLDER, 'voice'],
					withThird: [START_TS_PLACEHOLDER, 'video', null],
				};
				// PARK AFTER EVERY ATTEMPT. The first version parked once at the
				// end, and one of these shapes HUNG — so the probe reported
				// "never settled" and threw away the four answers it already
				// had. A hang is an answer too, but only if the answers before
				// it survive to name which call did the hanging.
				for (const [name, arg] of Object.entries(shapes)) {
					out.link[name] = 'PENDING';
					park(out);
					try {
						const r = await CL.createEventCallLink.apply(null, arg);
						out.link[name] = 'resolved: ' + (typeof r) +
							(typeof r === 'string' ? ' len=' + r.length
								: (r && typeof r === 'object' ? ' keys=[' + Object.keys(r).join(',') + ']' : ''));
					} catch (e) {
						out.link[name] = 'threw: ' + String((e && e.message) || e).slice(0, 120);
					}
					park(out);
				}
			} catch (e) {
				out.link = 'module absent: ' + String(e && e.message);
			}

			// WHY A PLACED CALL RANG NOTHING.
			//
			// startWAWebVoipCall resolved and no handset rang — the NOTHING class
			// (H82), confirmed by the only instrument that could confirm it: a
			// human saying the phone stayed quiet. These readers are the page's
			// OWN answer to "can this browser do calls at all", and they are pure
			// predicates: nothing here dials, initialises or allocates.
			try {
				const G = window.require('WAWebVoipGatingUtils');
				out.gating = {};
				for (const f of ['isCallingEnabled', 'isUnsupportedBrowserForWebCalling',
					'getUnsupportedBrowserReason', 'getCrossOriginIsolatedState', 'isWebKitBrowser']) {
					try {
						out.gating[f] = typeof G[f] === 'function' ? String(G[f]()) : 'absent';
					} catch (e) {
						out.gating[f] = 'threw: ' + String((e && e.message) || e).slice(0, 100);
					}
				}
			} catch (e) {
				out.gating = 'module absent';
			}
			// The browser's own view, which is what the gating reads.
			out.env = {
				crossOriginIsolated: typeof crossOriginIsolated !== 'undefined' ? crossOriginIsolated : 'undefined',
				hasSharedArrayBuffer: typeof SharedArrayBuffer !== 'undefined',
				hasWasm: typeof WebAssembly !== 'undefined',
			};

			// THE INIT, which nothing has ever run in this session.
			//
			// The gating says calling is enabled and the browser is fine, so the
			// environment hypothesis is dead. What remains is that the VOIP
			// stack is initialised on DEMAND — the app has an explicit
			// ensureVoipInitialized and a previewCallLinkWithVoipInit — and a
			// headless driver that calls startWAWebVoipCall has never been
			// through the UI path that would have run it.
			//
			// This ALLOCATES a WASM stack, which is heavier than everything else
			// in this probe. It rings nobody.
			try {
				const EV = window.require('WAWebEnsureVoipInited');
				out.init = { pending: true };
				park(out);
				try {
					const r = await EV.ensureVoipInitialized();
					out.init = { resolved: true, kind: typeof r,
						keys: (r && typeof r === 'object') ? Object.keys(r).slice(0, 20) : [] };
				} catch (e) {
					out.init = { resolved: false, why: String((e && e.message) || e).slice(0, 200),
						name: (e && e.constructor && e.constructor.name) || typeof e };
				}
				park(out);
			} catch (e) {
				out.init = 'module absent: ' + String(e && e.message);
			}

			// THE READER THAT HAS NEVER COUNTED ANYTHING.
			//
			// H93 stopped exactly here: conta-A read zero calls after dialling,
			// and that zero could not be used as a negative because the reader
			// had never been observed counting a call. Before any conclusion
			// about the dial, this dumps EVERY plausible container on the call
			// collection, by name and size — so a later run with a call in
			// flight can say which one moves.
			try {
				const C = window.require('WAWebCallCollection');
				out.containers = {};
				for (const k of Object.keys(C)) {
					try {
						const v = C[k];
						if (v instanceof Map) { out.containers[k] = 'Map size=' + v.size; continue; }
						if (Array.isArray(v)) { out.containers[k] = 'array len=' + v.length; continue; }
						if (v && typeof v.getModelsArray === 'function') {
							out.containers[k] = 'collection models=' + v.getModelsArray().length; continue;
						}
						if (v && typeof v.size === 'number') { out.containers[k] = 'sized=' + v.size; continue; }
						if (v && typeof v.length === 'number') { out.containers[k] = 'lengthed=' + v.length; continue; }
						out.containers[k] = shape(v);
					} catch (e) { out.containers[k] = 'threw'; }
				}
				const holder = C.CallCollection || C.default || C;
				out.containers['<holder>.getModelsArray'] =
					holder && typeof holder.getModelsArray === 'function'
						? 'models=' + holder.getModelsArray().length : 'absent';
			} catch (e) {
				out.containers = 'module absent';
			}

			// Alternatives the reference does not use, in case this build has a
			// first-class path where it had to improvise.
			for (const name of [
				'WAWebCallModel', 'WAWebRejectCallAction', 'WAWebEndCallAction',
				'WAWebCallActions', 'WAWebOfferCallAction', 'WAWebCallCollectionGetters',
			]) {
				try {
					const m = window.require(name);
					out.alternatives[name] = m ? Object.keys(m).slice(0, 30) : 'falsy';
				} catch (e) {
					out.alternatives[name] = 'absent';
				}
			}
		} catch (e) {
			out.fatal = String(e && e.message);
		}
		out.finished = true;
		park(out);
		})();
		return 'kicked';
	})()`

	// THE TIMESTAMP COMES FROM GO. A probe is still a page script, and the one
	// rule that does not bend is that the page does not read its own clock.
	kick := strings.ReplaceAll(callsProbeScript, "START_TS_PLACEHOLDER",
		strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/calls-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	// THE LAST PARK IS THE ANSWER, FINISHED OR NOT. One of the shapes this
	// probe tries hangs, and a loop that waited for completion would report
	// "never settled" while discarding everything measured before the hang.
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/calls-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__calls || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(raw, `"finished":true`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never parked anything at all")
	}
	if !strings.Contains(raw, `"finished":true`) {
		t.Logf("the probe did NOT finish; whatever is marked PENDING below is the call that hung")
	}
	t.Logf("%s", raw)
}

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

// TestProbeModuleRegistry is the measurement that makes every LATER capability
// cheap: is there a way to ENUMERATE this build's module registry?
//
// Every capability so far paid the same tax — guessing module names from the
// reference implementation and finding they do not exist here (four out of four
// for sendText). If the registry can be listed, that tax disappears: the names
// come from the page instead of from a foreign build.
//
// It prints only module NAMES and counts. No message content, no identity.
func TestProbeModuleRegistry(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MODMAP") == "" {
		t.Skip("set WA_PROBE_MODMAP=1")
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

	// Several strategies, because which one works is exactly the unknown.
	// Each reports whether it is available and how many ids it can see.
	// The registry is reachable through the webpack chunk array, which the
	// first pass measured as the ONLY surviving door: require has no .m, and
	// window.__debug — the hook whatsapp-web.js enumerates through — does not
	// exist on this build at all.
	//
	// Pushing a chunk is a WRITE to the page, and it is the same technique the
	// reference uses. It is done here on a lab profile, it adds no modules of
	// its own, and it only captures the runtime require to read names from it.
	// THE UNIVERSE OF NAMES LIVES IN THE BUNDLES. Both enumeration doors the
	// reference implementation uses are shut on this build, and that was
	// measured, not assumed:
	//
	//	window.require.m                        absent (require has only length,name,prototype)
	//	window.__debug                          absent  <- what whatsapp-web.js enumerates through
	//	webpackChunkwhatsapp_web_client         present but an EMPTY array with a NATIVE push,
	//	                                        so injecting a chunk appends and never runs
	//
	// What remains is the source. Module names are string literals in the
	// bundles the page already downloaded, so fetching them hits cache and
	// yields the real inventory of THIS build instead of a foreign one's
	// guesses. First run: 65 bundles, 38 MB, 12,674 distinct names, 16s.
	//
	// A name in a bundle is NOT proof of a loadable module, so every candidate
	// is then handed to window.require and reported with what it exports.
	// Only names are returned — never content, never identity.
	const script = `(() => {
		window.__waHeadlessModProbe = { stage: 'pending' };
		const pattern = new RegExp(PATTERN_PLACEHOLDER);
		const GREP = GREP_PLACEHOLDER;
		// HOW MUCH CONTEXT AROUND A HIT. Fixed at 260 until H101, where the
		// answer sat in the HEAD of a minified function and 260 characters showed
		// only its tail — which is precisely the truncation H69 recorded as "the
		// bundle grep cut it off" and then stopped at.
		const WINDOW = WINDOW_PLACEHOLDER;
		const hits = [];
		(async () => {
			try {
				const urls = performance.getEntriesByType('resource')
					.map(e => e.name).filter(n => /\.js(\?|$)/.test(n));
				const seen = new Set();
				let bytes = 0, fetched = 0, failed = 0;
				for (const u of urls) {
					try {
						const r = await fetch(u);
						if (!r.ok) { failed++; continue; }
						const txt = await r.text();
						bytes += txt.length; fetched++;
							const m = txt.match(/\bWA[A-Z][A-Za-z0-9_]{3,60}\b/g);
						if (m) { for (const n of m) seen.add(n); }
						// GREP MODE: when a literal is given, return the text
						// AROUND each hit. Module names answer "what exists";
						// this answers "how is it called", which is the other
						// half and which no name list can give.
						if (GREP) {
							let at = -1;
							while ((at = txt.indexOf(GREP, at + 1)) !== -1 && hits.length < 6) {
								hits.push(txt.slice(Math.max(0, at - WINDOW), at + WINDOW));
							}
						}
					} catch (e) { failed++; }
				}
				const all = Array.from(seen).sort();
				const matchedNames = all.filter(n => pattern.test(n));
				// Resolvability, which is the part a bundle scan cannot answer.
				const resolved = [];
				for (const n of matchedNames) {
					let mod = null;
					try { mod = window.require(n); } catch (e) { continue; }
					if (!mod) { continue; }
					let keys = [];
					try { keys = Object.keys(mod).slice(0, 14); } catch (e) { keys = ['<opaque>']; }
					resolved.push(n + ' :: ' + keys.join(','));
				}
				window.__waHeadlessModProbe = {
					stage: 'done', urls: urls.length, fetched: fetched, failed: failed,
					bytes: bytes, distinct: all.length,
					matched: matchedNames.length, loadable: resolved.length,
					modules: resolved, grepHits: hits
				};
			} catch (e) {
				window.__waHeadlessModProbe = { stage: 'error', why: String((e && e.message) || e) };
			}
		})();
		return 'kicked';
	})()`

	const pollScript = `JSON.stringify(window.__waHeadlessModProbe || {stage:'missing'})`

	pattern := os.Getenv("WA_PROBE_MODMAP_RE")
	if pattern == "" {
		pattern = "^WAWeb(Contact|ProfilePic|Presence)"
	}
	kick := strings.Replace(script, "PATTERN_PLACEHOLDER", strconv.Quote(pattern), 1)
	kick = strings.Replace(kick, "GREP_PLACEHOLDER", strconv.Quote(os.Getenv("WA_PROBE_MODMAP_GREP")), 1)
	window := 260
	if raw := os.Getenv("WA_PROBE_MODMAP_WINDOW"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			window = n
		}
	}
	kick = strings.Replace(kick, "WINDOW_PLACEHOLDER", strconv.Itoa(window), 1)
	t.Logf("pattern: %s", pattern)

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/modmap/kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("probe kick: %v", err)
	}
	// Store-and-poll: Evaluate does not await promises, so the page parks the
	// answer and Go drains it. Same shape as every other async capability here.
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/modmap/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, pollScript, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the bundle scan never settled")
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("module universe: %s", raw)
}

package waheadless

// Observation against the REAL web.whatsapp.com, with a throwaway test account.
//
// Everything else in this module is proved against pages this repository wrote.
// That was the right way to get here, and it has a hard limit: it cannot say
// whether WhatsApp's actual markup still matches our selectors. Only the real
// SPA can, and only by looking.
//
// The rule for this file, from the phase it opens:
//
//	DO NOT INFER SPA BEHAVIOUR -> MEASURE IT -> FREEZE THE EVIDENCE
//
// whatsapp-web.js is a map of where to look and a source of internal names. It
// is NOT automatic truth about object shapes, invariants, behaviour, lifecycle
// or errors. What the page does is what is true.
//
// SKIPPED BY DEFAULT. `make check` must never reach the real target: it boots a
// profile, and phase 4C measured that repeated boots are how a paired session
// degrades. Opt in explicitly:
//
//	WA_HEADLESS_REAL_SPA=1 go test -tags= -run TestRealSPA ./internal/wa-headless/ -v
//
// The observation probes default to the disposable lab profile and can be
// pointed at another one with WA_HEADLESS_PROFILE_DIR, which is how a profile
// of unknown paired state gets looked at by an instrument already proven to
// work. That override reaches the READ-ONLY path only: pairing mutates the
// profile it opens and stays bound to the lab one.
//
// The profile resolution itself is covered by ordinary unit tests at the bottom
// of this file. They run in every `go test` — they open no browser and touch no
// profile.
//
// NO PII, EVER. These probes report booleans, counts and tag names. They do not
// read message text, contact names, phone numbers or the QR payload. The QR
// image itself is a credential for the seconds it lives: it is never captured,
// never logged, never written to disk.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

const (
	// realSPAToggle opts a run in to touching the real target.
	realSPAToggle = "WA_HEADLESS_REAL_SPA"
	// realSPAURL is the target.
	realSPAURL = "https://web.whatsapp.com/"
	// labProfileDir is the throwaway profile of the TEST account, gitignored
	// via internal/wa-headless/.gitignore. It is never the study's paired
	// profile: that one belongs to another phase and is not disposable.
	labProfileDir = ".lab/test-account-profile"
	// profileDirOverride points the READ-ONLY observation probes at a profile
	// other than the lab one, so a profile whose paired state is unknown can be
	// looked at with the instrument that is already proven to work.
	//
	// It reaches the observation path ONLY. The pairing path refuses it — see
	// pairingProfileDir.
	profileDirOverride = "WA_HEADLESS_PROFILE_DIR"
	// labProfilePerm keeps the profile readable by its owner alone: from the
	// moment an account links, the directory holds a session credential.
	labProfilePerm = 0o700
	// realSPAUserAgent is required for compatibility, not for evasion.
	//
	// WhatsApp refuses the HeadlessChrome token outright and serves an "update
	// your browser" page; the study lost a run to launching without this. The
	// token matches the installed browser's real version, and nothing else
	// about the fingerprint is touched. Recorded in every report that uses it.
	realSPAUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
)

func requireRealSPA(t *testing.T) {
	t.Helper()
	if os.Getenv(realSPAToggle) == "" {
		t.Skipf("set %s=1 to run against the real web.whatsapp.com with the test account", realSPAToggle)
	}
}

// observationProfileDir resolves, as an absolute path, the profile the
// READ-ONLY probes open, and reports whether the caller overrode it.
//
// Resolution only: it touches no filesystem, so calling it is free of side
// effects on either profile.
func observationProfileDir() (dir string, overridden bool, err error) {
	if override := os.Getenv(profileDirOverride); override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s=%s: %w", profileDirOverride, override, err)
		}
		return abs, true, nil
	}
	abs, err := filepath.Abs(labProfileDir)
	if err != nil {
		return "", false, fmt.Errorf("resolving the lab profile: %w", err)
	}
	return abs, false, nil
}

// requireExistingProfile refuses an overridden profile that is not already
// there.
//
// The lab profile is created on demand because it is disposable. An overridden
// one must NOT be: creating it would answer the very question the observation
// asks. An empty profile always shows a QR, so a typo in the path would be
// recorded as evidence that the profile is unpaired — the instrument inventing
// its own result.
func requireExistingProfile(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s=%s: the profile must already exist, and an absent one is not "+
			"created here (an empty profile looks unpaired, which is the answer this "+
			"observation is supposed to measure): %w", profileDirOverride, dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s=%s: not a directory", profileDirOverride, dir)
	}
	return nil
}

// pairingProfileDir resolves the profile the PAIRING path may open, and refuses
// an overridden one.
//
// Pairing is the one operation in this file that MUTATES the profile: it links
// an account into it. The observation probes only read, so they can be pointed
// anywhere; pairing pointed at a real profile could re-pair or overwrite a
// credential that is not disposable.
//
// The refusal is an ERROR rather than a skip on purpose. A skip says "this run
// had nothing to do", and it would be silent about the fact that a human is
// waiting at a window to scan a code that is never coming. The caller opted in
// to pairing explicitly with two toggles; a request the instrument will not
// honour has to be loud.
func pairingProfileDir() (string, error) {
	if override := os.Getenv(profileDirOverride); override != "" {
		return "", fmt.Errorf("refusing to pair: %s=%s is set, and pairing MUTATES the "+
			"profile it opens. Only %s is disposable. Unset %s to pair the lab profile",
			profileDirOverride, override, labProfileDir, profileDirOverride)
	}
	abs, err := filepath.Abs(labProfileDir)
	if err != nil {
		return "", fmt.Errorf("resolving the lab profile: %w", err)
	}
	return abs, nil
}

// openRealSPA launches the observation profile and navigates to the target.
func openRealSPA(t *testing.T, runner *engine.Runner) (*engine.Browser, *engine.Tab) {
	t.Helper()
	requireRealSPA(t)
	binary := findChrome(t)

	profile, overridden, err := observationProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	if overridden {
		if err := requireExistingProfile(profile); err != nil {
			t.Fatal(err)
		}
	} else if err := os.MkdirAll(profile, labProfilePerm); err != nil {
		t.Fatalf("creating the lab profile: %v", err)
	}
	// The PATH, never the contents: any evidence this run produces has to say
	// what was looked at, and a path carries no account data.
	t.Logf("profile_dir=%s overridden=%v", profile, overridden)

	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    profile,
		DebuggingPort: freePort(t),
		UserAgent:     realSPAUserAgent,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	t.Cleanup(func() {
		// Invariant 2: the profile holds a credential, so it goes down through
		// the protocol. A signal here would corrupt the very session the next
		// loop depends on.
		t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser))
	})

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	t.Cleanup(tab.Close)

	if err := tab.Navigate(runner, realSPAURL, "real/navigate"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	return browser, tab
}

// markerReport is what the observation gathers: SHAPE only.
//
// Every field is a boolean, a count or a tag name. Nothing here can carry a
// message, a name or a number — that is the property that makes it safe to
// print in a test log.
type markerReport struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	ReadyState string `json:"ready_state"`
	DOMNodes   int    `json:"dom_nodes"`

	// The selectors this module currently believes in.
	HasPaneSide bool `json:"has_pane_side"`
	HasQRScan   bool `json:"has_qr_scan_aria"`

	// Candidates, so a broken selector can be replaced with evidence rather
	// than with another guess.
	CanvasCount     int    `json:"canvas_count"`
	CanvasAriaLabel string `json:"first_canvas_aria_label"`
	CanvasParentTag string `json:"first_canvas_parent_tag"`
	HasDataRefIn    bool   `json:"has_data_ref_attr"`
	AppCount        int    `json:"app_div_count"`
	LinkDeviceText  bool   `json:"has_link_device_wording"`
	TestIDs         string `json:"data_testids_present"`
}

// markerScript reads structure and nothing else.
//
// The one string it returns from the page is a canvas aria-label, which is UI
// chrome ("Scan this QR code…"), not user content. data-testid VALUES are
// collected because they are Meta's own stable-ish hooks and carry no user
// data; they are what a less fragile selector would key on.
const markerScript = `JSON.stringify((() => {
	const q = (s) => document.querySelector(s);
	const canvases = Array.from(document.querySelectorAll('canvas'));
	const first = canvases[0];
	const ids = Array.from(document.querySelectorAll('[data-testid]'))
		.map(e => e.getAttribute('data-testid'))
		.filter((v, i, a) => a.indexOf(v) === i)
		.slice(0, 40);
	const body = document.body ? document.body.innerText : '';
	return {
		url: location.href,
		title: document.title,
		ready_state: document.readyState,
		dom_nodes: document.getElementsByTagName('*').length,
		has_pane_side: !!q('#pane-side'),
		has_qr_scan_aria: !!q('canvas[aria-label*="Scan"]'),
		canvas_count: canvases.length,
		first_canvas_aria_label: first ? (first.getAttribute('aria-label') || '') : '',
		first_canvas_parent_tag: first && first.parentElement ? first.parentElement.tagName : '',
		has_data_ref_attr: !!q('[data-ref]'),
		app_div_count: document.querySelectorAll('#app').length,
		has_link_device_wording: /link(ing)? (a )?device|conectar um aparelho|vincular/i.test(body),
		data_testids_present: ids.join(',')
	};
})())`

func readMarkers(t *testing.T, runner *engine.Runner, tab *engine.Tab, label string) markerReport {
	t.Helper()
	var raw string
	err := runner.Do(context.Background(), engine.OpStateProbe, label, func(ctx context.Context) error {
		return tab.Evaluate(ctx, markerScript, &raw)
	})
	if err != nil {
		t.Fatalf("%s: the page did not answer: %v", label, err)
	}
	var out markerReport
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("%s: unexpected answer shape: %v", label, err)
	}
	return out
}

func (m markerReport) log(t *testing.T, stage string) {
	t.Helper()
	t.Logf("[%s] url=%s ready=%s nodes=%d", stage, m.URL, m.ReadyState, m.DOMNodes)
	t.Logf("[%s] selectors we believe in: #pane-side=%v canvas[aria-label*=Scan]=%v",
		stage, m.HasPaneSide, m.HasQRScan)
	t.Logf("[%s] candidates: canvases=%d parent=%s aria=%q [data-ref]=%v #app=%d linkwording=%v",
		stage, m.CanvasCount, m.CanvasParentTag, m.CanvasAriaLabel,
		m.HasDataRefIn, m.AppCount, m.LinkDeviceText)
	if m.TestIDs != "" {
		t.Logf("[%s] data-testid present: %s", stage, m.TestIDs)
	}
}

// LOOP B1.1 — an unpaired profile against the real SPA.
//
// This is OBSERVATION. It asserts almost nothing on purpose: the point is to
// see what the page is, before deciding whether any selector needs to change.
// The one thing it does refuse is a page that never answers, because then there
// is nothing to observe and saying otherwise would be inventing the result.
func TestRealSPAUnpairedBootObservation(t *testing.T) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	// The SPA takes seconds to mount. Sampling over a window shows what it
	// becomes, not just what it was at one instant — and a single early sample
	// is how a loading page gets recorded as a broken one.
	deadline := time.Now().Add(45 * time.Second)
	var last markerReport
	for i := 0; time.Now().Before(deadline); i++ {
		last = readMarkers(t, runner, tab, fmt.Sprintf("real/markers%d", i))
		if last.HasQRScan || last.HasPaneSide || last.CanvasCount > 0 {
			last.log(t, fmt.Sprintf("settled@%ds", i*3))
			break
		}
		if i%3 == 0 {
			last.log(t, fmt.Sprintf("t+%ds", i*3))
		}
		time.Sleep(3 * time.Second)
	}
	last.log(t, "final")

	snap, class := spa.Probe(context.Background(), runner, tab.Evaluate, "real/classify")
	t.Logf("CLASSIFIED AS: %s (has_pane=%v has_qr=%v url=%s)",
		class, snap.HasPane, snap.HasQR, snap.URL)

	if class == spa.ClassUnresponsive {
		t.Fatal("the real SPA never answered; nothing was observed, so nothing can be " +
			"concluded about the selectors")
	}
	if last.DOMNodes == 0 {
		t.Fatal("the page rendered nothing")
	}
}

// pairToggle opts a run in to showing a window and waiting for a human.
const pairToggle = "WA_HEADLESS_PAIR"

// pairingBudget is how long the window stays up waiting for the scan. Generous
// because the other end of this budget is a person finding their phone.
const pairingBudget = 5 * time.Minute

// readinessBudget and readinessTick bound the timeline sampling. The tick is
// tight because this loop compares INSTANTS: a coarse tick would smear the very
// gap it exists to detect.
const (
	readinessBudget = 90 * time.Second
	readinessTick   = 250 * time.Millisecond
)

// LOOP B1.3 — pairing, which only a person can do.
//
// NOTHING about authentication is automated here. The QR is not captured, not
// decoded, not screenshotted and not logged: it is a credential that lives for
// seconds, and the only safe place for it is a screen a human is looking at.
// That is why this launch is HEADFUL — the one operation in this module that is.
//
// What gets recorded is the three facts the phase asks for, and no more:
//
//	PAIRING_STARTED     the code is on screen
//	PAIRING_COMPLETED   the account linked
//	APP_READY_AT        how long the application took to become usable
//
// The browser stays up until the session is READY or the budget runs out, and
// it always goes down through Browser.close, because from the moment the scan
// lands this profile holds a credential.
func TestRealSPAPairing(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv(pairToggle) == "" {
		t.Skipf("set %s=1 to open a window and pair the TEST account by QR", pairToggle)
	}
	runner := engine.NewRunner()
	tab := openLabProfileHeadful(t, runner)

	qrShownAt, readyAt := awaitPairing(t, runner, tab)

	if qrShownAt == 0 {
		t.Fatal("the QR never appeared, so there was nothing to scan")
	}
	if readyAt == 0 {
		t.Fatalf("PAIRING_NOT_COMPLETED within the budget (QR shown at t+%.1fs). "+
			"The profile is left as it is; run again to retry.", qrShownAt.Seconds())
	}
}

// openLabProfileHeadful launches the lab profile with a visible window and
// navigates to the target. Headful is only ever for pairing.
//
// The LAB profile, always: this is the mutating path, so pairingProfileDir
// refuses the observation override instead of following it.
func openLabProfileHeadful(t *testing.T, runner *engine.Runner) *engine.Tab {
	t.Helper()
	binary := findChrome(t)

	profile, err := pairingProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(profile, labProfilePerm); err != nil {
		t.Fatalf("creating the lab profile: %v", err)
	}
	t.Logf("profile_dir=%s (pairing is bound to the lab profile)", profile)

	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    profile,
		DebuggingPort: freePort(t),
		UserAgent:     realSPAUserAgent,
		Headful:       true,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	t.Cleanup(func() {
		t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser))
	})

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	t.Cleanup(tab.Close)

	if err := tab.Navigate(runner, realSPAURL, "pair/navigate"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	return tab
}

// awaitPairing watches the classification until the account links or the budget
// runs out. It reports WHEN, never WHAT: no QR, no identity, no page text.
func awaitPairing(t *testing.T, runner *engine.Runner, tab *engine.Tab) (qrShownAt, readyAt time.Duration) {
	t.Helper()
	start := time.Now()

	for deadline := start.Add(pairingBudget); time.Now().Before(deadline); {
		_, class := spa.Probe(context.Background(), runner, tab.Evaluate, "pair/classify")

		switch class {
		case spa.ClassLoginRequired:
			if qrShownAt == 0 {
				qrShownAt = time.Since(start)
				t.Logf("PAIRING_STARTED at t+%.1fs — the QR is on screen. "+
					"Scan it with the TEST account. Nothing here reads it.", qrShownAt.Seconds())
			}
		case spa.ClassAppReady:
			readyAt = time.Since(start)
			t.Logf("PAIRING_COMPLETED — the account linked")
			t.Logf("APP_READY_AT t+%.1fs", readyAt.Seconds())
			return qrShownAt, readyAt
		case spa.ClassUnresponsive:
			t.Fatalf("the page stopped answering at t+%.1fs; pairing cannot be "+
				"observed through a wedged renderer", time.Since(start).Seconds())
		}
		time.Sleep(2 * time.Second)
	}
	return qrShownAt, readyAt
}

// readinessSample is one instant of the boot, in signals only.
//
// Every field is a boolean or a count. Nothing here can carry a phone number, a
// name or a message — which is what makes it safe to print a whole timeline.
type readinessSample struct {
	At time.Duration `json:"-"`

	DOMNodes    int  `json:"dom_nodes"`
	HasPaneSide bool `json:"has_pane_side"`
	HasQR       bool `json:"has_qr"`
	HasQRLoad   bool `json:"has_qr_loading"`

	// HasRequire is the SPA's module loader being reachable at all.
	HasRequire bool `json:"has_require"`
	// ModulesResolved counts how many of the startup inventory answer.
	ModulesResolved int `json:"modules_resolved"`
	// HasOwnerIdentity is the owner identity being materialised, read WHERE
	// whatsapp-web.js reads it — see moduleUserPrefsMeUser.
	//
	// Presence only. The value is the account's phone identity and is never
	// read, logged or asserted on anywhere in this file.
	//
	// LOOP B1.4b measured this getter on both profiles with one instrument:
	// EMPTY for a full 75s on the unpaired lab profile, PRESENT at T+0.01s on
	// the paired one. It discriminates, which is what makes it usable at all.
	HasOwnerIdentity bool `json:"has_owner_identity"`
	// HasConnWID is the field the FIRST version of this probe asserted on, kept
	// as a RECORDED control rather than deleted.
	//
	// It reported false for the whole 90s budget on a session that was
	// demonstrably connected, and B1.4b found out why: WAWebConnModel has no
	// wid key in this build, on EITHER profile. The field does not exist, so
	// the probe was reading a place the identity never lived. Keeping it
	// sampled means the day Meta adds it back, the evidence says so.
	HasConnWID bool `json:"has_conn_wid"`
	// HasConnWIDAccessor is the SAME question asked of the accessor rather than
	// of the backing field, and it exists because the two are not the same
	// question.
	//
	// __x_wid and Object.keys(Conn) both see only OWN ENUMERABLE properties. A
	// prototype getter Conn.wid — one delegating to the user-prefs store, say —
	// would be invisible to both while keeping whatsapp-web.js's one surviving
	// read working. This file already depends on that duality in the other
	// direction: it reads sk.__x_state where wwebjs reads Socket.state.
	//
	// Recorded, never asserted, exactly like HasConnWID: it is the falsifier of
	// the claim that the field is gone from this build.
	HasConnWIDAccessor bool `json:"has_conn_wid_accessor"`
	// MeReadyTriggered is the SPA's own flag for "the account finished
	// loading". A boolean of Meta's, not an inference of ours.
	MeReadyTriggered bool `json:"me_ready_triggered"`
	// SocketState is an enum of Meta's (CONNECTED, OPENING…), not user data.
	SocketState string `json:"socket_state"`
	// NavigatorOnline is the browser's own view of connectivity.
	//
	// It is here for LOOP 04.3A, which severs the page's network and must be
	// able to say the sever LANDED. Without it, a run where nothing changed
	// would be indistinguishable from a run where nothing was severed — the
	// instrument reporting its own finding by construction.
	//
	// On a boot it is a constant true and says nothing about readiness, which is
	// why it is recorded and never asserted on in the timeline.
	NavigatorOnline bool `json:"navigator_online"`
}

// socketStateConnected is Meta's own name for a live socket, as observed in
// WAWebSocketModel.Socket.__x_state. whatsapp-web.js reads the same field to
// decide whether a session needs authenticating (1.34.7, src/Client.js:146-179,
// where UNPAIRED and UNPAIRED_IDLE are the negative verdicts).
const socketStateConnected = "CONNECTED"

// readinessScript samples every signal in ONE evaluation.
//
// One round trip per tick matters: two would smear the timeline by however long
// the first took, and this test exists to compare instants.
func readinessScript(modules []spa.Module) string {
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = `'` + string(m) + `'`
	}
	return `JSON.stringify((() => {
		const q = (s) => !!document.querySelector(s);
		const hasRequire = typeof window.require === 'function';
		let resolved = 0;
		let hasIdentity = false, hasConnWid = false, hasConnWidAccessor = false;
		let meReady = false, socketState = '';
		if (hasRequire) {
			for (const name of [` + strings.Join(names, ",") + `]) {
				try { if (window.require(name)) resolved++; } catch (e) {}
			}
			try {
				// Where whatsapp-web.js takes the owner identity from, and the
				// only place this file looks for it. Presence only: the value
				// is the account's phone identity.
				const me = window.require('` + moduleUserPrefsMeUser + `');
				const pn = (me && typeof me.getMaybeMePnUser === 'function') ? me.getMaybeMePnUser() : null;
				const lid = (me && typeof me.getMaybeMeLidUser === 'function') ? me.getMaybeMeLidUser() : null;
				hasIdentity = !!(pn || lid);
			} catch (e) {}
			try {
				const m = window.require('` + string(spa.ModuleConnModel) + `');
				const c = m && (m.Conn || m.default || m);
				// Recorded controls, both coerced to booleans in the page: one
				// of these values would BE the account. The own field and the
				// accessor are asked separately because only the second would
				// see a prototype getter — see HasConnWIDAccessor.
				hasConnWid = !!(c && c.__x_wid);
				hasConnWidAccessor = !!(c && c.wid);
				meReady = !!(c && c.__x_meReadyTriggered === true);
			} catch (e) {}
			try {
				const m = window.require('` + string(spa.ModuleSocketModel) + `');
				const sk = m && (m.Socket || m.default || m);
				socketState = (sk && typeof sk.__x_state === 'string') ? sk.__x_state : '';
			} catch (e) {}
		}
		return {
			dom_nodes: document.getElementsByTagName('*').length,
			has_pane_side: q('#pane-side'),
			has_qr: q('[data-testid="link-device-qr-code"]') || q('canvas[aria-label*="Scan"]'),
			has_qr_loading: q('[data-testid^="link-device-qrcode-alt-linking"]'),
			has_require: hasRequire,
			modules_resolved: resolved,
			has_owner_identity: hasIdentity,
			has_conn_wid: hasConnWid,
			has_conn_wid_accessor: hasConnWidAccessor,
			me_ready_triggered: meReady,
			socket_state: socketState,
			navigator_online: navigator.onLine === true
		};
	})())`
}

// LOOP B1.4 — does #pane-side mean READY, or does it arrive early?
//
// The question is not rhetorical. Six seconds separate the pairing screen's
// markers from the QR itself (EVIDENCIA-SPA.md M1.3), so "an element appeared"
// has already proved once, in this same SPA, not to mean "the thing is usable".
// Treating presence as readiness is the exact mistake this loop exists to test
// for, and the answer decides whether the classifier has a false positive.
//
// The first version of this test could never have answered it. It compared the
// pane against WAWebConnModel's __x_wid, a field that does not exist in this
// build, so it reported EARLY_MARKER for any profile whatsoever — including one
// measured CONNECTED, unpaired-QR-free and 2691 DOM nodes deep. A probe with
// only one reachable answer is not measuring anything. B1.4b found where the
// identity really lives; this test now reads it there. EVIDENCIA-SPA.md M3
// holds both measurements.
func TestRealSPAReadinessTimeline(t *testing.T) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	script := readinessScript(spa.RequiredAtStartup)
	want := len(spa.RequiredAtStartup)

	samples, marks := sampleReadiness(t, runner, tab, script, want)

	logTimeline(t, samples, want)
	// "never" must not print as T+0.00s. A zero mark means the signal never
	// turned on, and rendering that as an instant reads like "immediately" —
	// an instrument that lies in the direction of optimism.
	t.Logf("  #pane-side at             %s", markString(marks.pane))
	t.Logf("  module inventory at       %s", markString(marks.modules))
	t.Logf("  owner identity at         %s", markString(marks.identity))
	t.Logf("  meReadyTriggered at       %s", markString(marks.meReady))
	t.Logf("  socket %-10s at      %s", socketStateConnected, markString(marks.connected))

	if marks.pane == 0 {
		t.Fatal("#pane-side never appeared on a paired profile")
	}
	if marks.modules == 0 {
		t.Fatal("the module inventory never resolved; the SPA never finished booting")
	}
	// The precondition of the whole comparison, and the reason record() waits
	// on the socket. Without it this test PASSES on a paired-but-offline
	// profile: the pane comes back from cache, the identity comes back from
	// prefs, and a timeline in which nothing ever reached the server gets
	// printed as a healthy boot.
	if marks.connected == 0 {
		t.Fatalf("the socket never reported %s within %s. This profile is paired — the "+
			"identity and the pane say so — but the session never reached the server, "+
			"so there is no boot here to compare instants in. A pane rendered from "+
			"cache is not readiness.", socketStateConnected, readinessBudget)
	}

	// The verdict this loop exists to produce.
	const tolerance = 250 * time.Millisecond
	switch {
	case marks.identity == 0:
		t.Error("EARLY_MARKER: #pane-side appeared but the owner identity never did — " +
			"the classifier would report READY without the one signal that shows the " +
			"profile is even PAIRED")
	case marks.identity > marks.pane+tolerance:
		t.Errorf("EARLY_MARKER: #pane-side at T+%.2fs but the owner identity only at "+
			"T+%.2fs — a %.2fs window where the classifier says READY and the SPA "+
			"cannot yet name the profile's owner", marks.pane.Seconds(),
			marks.identity.Seconds(), (marks.identity - marks.pane).Seconds())
	default:
		t.Logf("SAFE_READY_MARKER on the identity axis: the owner identity was already "+
			"there when #pane-side appeared (identity T+%.2fs <= pane T+%.2fs + tolerance)",
			marks.identity.Seconds(), marks.pane.Seconds())
	}

	// Recorded, NOT asserted — and the distinction is the finding.
	//
	// The identity comes out of a user-prefs store, so it is back at T+0.01s,
	// before the socket has even opened. That makes it proof the profile is
	// PAIRED, not proof the session is live: a machine offline since last week
	// would answer it just as fast. The same disqualification as the module
	// inventory in M2.1, arrived at by measuring instead of by assuming.
	//
	// What IS an event of this boot are the two below. Both landed before the
	// pane on the three boots measured so far — which is an observation about
	// those boots, not a property of the SPA. The run above no longer depends
	// on it either way: the socket gates the stop, so a boot where CONNECTED
	// arrives AFTER the pane is waited for rather than missed.
	t.Logf("  (module inventory at %s — measured true on the LOGIN screen too, "+
		"so it is not evidence of readiness)", markString(marks.modules))
	t.Logf("  (WAWebConnModel identity, recorded and never asserted: own field __x_wid "+
		"seen=%v, accessor Conn.wid seen=%v, over %d samples. Both false is the measured "+
		"state of this build; either turning true is what would falsify that.)",
		anySample(samples, func(s readinessSample) bool { return s.HasConnWID }),
		anySample(samples, func(s readinessSample) bool { return s.HasConnWIDAccessor }),
		len(samples))
	t.Logf("  (the owner identity is PERSISTED, not connection-derived: it proves the "+
		"profile is paired, not that the session is live. The live-session instants are "+
		"meReadyTriggered %s and socket %s %s)",
		markString(marks.meReady), socketStateConnected, markString(marks.connected))
}

// readinessMarks are the instants this loop compares.
//
// identity, meReady and connected are the three things that could mean "this
// session can act". They are all recorded because B1.4b measured that they
// arrive at very different times and mean different things: identity is
// PERSISTED and back before the socket opens, while the other two are events of
// this boot.
type readinessMarks struct{ pane, modules, identity, meReady, connected time.Duration }

// record stamps the first time each signal turned on, and reports whether the
// run has everything the verdict needs. First-time-only: a signal that flickers
// must keep its earliest instant, because the question is when readiness BECAME
// true.
//
// The socket gates the stop, and that is the correction of a real defect. An
// earlier version stopped on pane+modules+identity, and every one of those
// three is reachable WITHOUT the session ever meeting the server: the identity
// is PERSISTED (M3.3), the module inventory resolves on the login screen (M2.1)
// and the pane renders from cache. On a paired-but-OFFLINE profile the loop
// therefore broke the instant the pane appeared and the run was called a
// success with socket NEVER — one reachable verdict per profile class, which is
// the same defect as the __x_wid probe this instrument replaced.
//
// It costs the healthy path nothing: across the three healthy boots measured so
// far, CONNECTED landed at T+5.82s / T+6.86s / T+7.07s and the pane at T+7.40s
// / T+8.35s / T+8.37s. The socket mark is already in when the pane arrives, so
// the stop still trips on the pane and the run is no longer than before.
//
// meReady does NOT gate. It preceded CONNECTED on all three, which is three
// samples and not a law; the socket state is Meta's own statement that the
// session reached the server, so it is the one worth waiting on.
func (m *readinessMarks) record(s readinessSample, want int) bool {
	if m.pane == 0 && s.HasPaneSide {
		m.pane = s.At
	}
	if m.modules == 0 && s.ModulesResolved == want {
		m.modules = s.At
	}
	if m.identity == 0 && s.HasOwnerIdentity {
		m.identity = s.At
	}
	if m.meReady == 0 && s.MeReadyTriggered {
		m.meReady = s.At
	}
	if m.connected == 0 && s.SocketState == socketStateConnected {
		m.connected = s.At
	}
	return m.pane > 0 && m.modules > 0 && m.identity > 0 && m.connected > 0
}

// sampleReadiness polls the boot until every mark is in, or the budget ends.
//
// It FAILS the run on a QR: a login screen has no readiness to observe, and
// producing a timeline from one would be inventing the result.
func sampleReadiness(t *testing.T, runner *engine.Runner, tab *engine.Tab,
	script string, want int) ([]readinessSample, readinessMarks) {
	t.Helper()

	start := time.Now()
	var samples []readinessSample
	var marks readinessMarks

	for deadline := start.Add(readinessBudget); time.Now().Before(deadline); {
		s, err := oneReadinessSample(runner, tab, script)
		s.At = time.Since(start)
		if err != nil {
			t.Logf("t+%.2fs probe failed: %v", s.At.Seconds(), err)
			time.Sleep(readinessTick)
			continue
		}
		samples = append(samples, s)

		if s.HasQR && marks.pane == 0 {
			t.Fatalf("the lab profile shows a QR at t+%.2fs: this profile is NOT paired, "+
				"so readiness cannot be observed. Pair it first (TestRealSPAPairing).", s.At.Seconds())
		}
		if marks.record(s, want) {
			break
		}
		time.Sleep(readinessTick)
	}
	return samples, marks
}

func oneReadinessSample(runner *engine.Runner, tab *engine.Tab, script string) (readinessSample, error) {
	var raw string
	err := runner.Do(context.Background(), engine.OpStateProbe, "ready/sample",
		func(ctx context.Context) error { return tab.Evaluate(ctx, script, &raw) })
	if err != nil {
		return readinessSample{}, err
	}
	var s readinessSample
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return readinessSample{}, fmt.Errorf("unexpected sample shape: %w", err)
	}
	return s, nil
}

// logTimeline prints only the samples where a signal changed.
func logTimeline(t *testing.T, samples []readinessSample, want int) {
	t.Helper()
	t.Log("TIMELINE (signals only, no account data)")
	prev := readinessSample{DOMNodes: -1}
	for _, s := range samples {
		if s.HasPaneSide == prev.HasPaneSide && s.HasQRLoad == prev.HasQRLoad &&
			s.HasRequire == prev.HasRequire && s.ModulesResolved == prev.ModulesResolved &&
			s.HasOwnerIdentity == prev.HasOwnerIdentity && s.HasConnWID == prev.HasConnWID &&
			s.HasConnWIDAccessor == prev.HasConnWIDAccessor &&
			s.MeReadyTriggered == prev.MeReadyTriggered && s.SocketState == prev.SocketState {
			continue
		}
		t.Logf("  T+%6.2fs nodes=%-6d pane=%-5v modules=%d/%d identity=%-5v connWid=%-5v "+
			"connWidGet=%-5v meReady=%-5v socket=%s",
			s.At.Seconds(), s.DOMNodes, s.HasPaneSide, s.ModulesResolved, want,
			s.HasOwnerIdentity, s.HasConnWID, s.HasConnWIDAccessor,
			s.MeReadyTriggered, s.SocketState)
		prev = s
	}
}

// LOOP B1.4a — which signals are ALREADY true without a session?
//
// The negative control of the timeline probe produced a finding that changes
// what READY can be built from: on the login screen, with no account paired,
// window.require answers and all eight startup modules resolve at T+0.01s. An
// inventory that is true before pairing cannot be evidence of readiness.
//
// So this measures the login screen deliberately, to DISQUALIFY candidates. It
// needs no pairing, and what it produces is the shortlist the paired run will
// check: whatever is absent here is a candidate for meaning "operational".
//
// Key NAMES only — never values. A key name is Meta's schema; a value could be
// the account's identity.
func TestRealSPADisqualifyReadinessSignalsOnLogin(t *testing.T) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	const shapeScript = `JSON.stringify((() => {
		const out = { require: typeof window.require === 'function', modules: {} };
		if (!out.require) return out;
		for (const name of ['WAWebConnModel', 'WAWebSocketModel', 'WAWebUserPrefsInfoStore']) {
			try {
				const m = window.require(name);
				const target = m && (m.Conn || m.Socket || m.default || m);
				out.modules[name] = target ? Object.keys(target).sort().slice(0, 60) : null;
			} catch (e) { out.modules[name] = 'THREW'; }
		}
		return out;
	})())`

	// Give the SPA the ~15s it needs to settle into the QR screen.
	time.Sleep(20 * time.Second)

	var raw string
	if err := runner.Do(context.Background(), engine.OpStateProbe, "shape/login",
		func(ctx context.Context) error { return tab.Evaluate(ctx, shapeScript, &raw) }); err != nil {
		t.Fatalf("the page did not answer: %v", err)
	}

	var out struct {
		Require bool           `json:"require"`
		Modules map[string]any `json:"modules"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unexpected shape: %v", err)
	}

	t.Logf("UNPAIRED LOGIN SCREEN — window.require present: %v", out.Require)
	for name, keys := range out.Modules {
		t.Logf("  %s keys: %v", name, keys)
	}
	if !out.Require {
		t.Fatal("window.require absent; nothing to disqualify")
	}
}

// moduleUserPrefsMeUser is the module whatsapp-web.js reads the owner identity
// from. It is NOT part of spa.RequiredAtStartup, so it has no constant there
// and gets one here, next to the only code that names it.
//
//	whatsapp-web.js 1.34.7 — src/Client.js:351-364
//	  wid: window.require('WAWebUserPrefsMeUser').getMaybeMePnUser()
//	    || window.require('WAWebUserPrefsMeUser').getMaybeMeLidUser()
//
// The same module is where the injected helpers take the sender identity from
// (src/util/Injected/Utils.js:420-424 and :1267-1269). Nowhere in that release
// does the client read the identity off WAWebConnModel.
const moduleUserPrefsMeUser = "WAWebUserPrefsMeUser"

// identityGetters are the accessor names to try on moduleUserPrefsMeUser.
//
// The two whatsapp-web.js actually calls come first; the rest are there so a
// build that renamed them is DETECTED rather than silently read as "no
// identity". The probe reports which ones exist, which is how a rename shows up
// as evidence instead of as a false negative.
var identityGetters = []string{
	"getMaybeMePnUser",
	"getMaybeMeLidUser",
	"getMaybeMeUser",
	"getMe",
	"getMeUser",
}

// jsQuotedList renders names as a JavaScript array body: 'a','b','c'.
func jsQuotedList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	return strings.Join(quoted, ",")
}

// identityShapeScript enumerates WHERE an owner identity could live, in key
// names and presence verdicts only.
//
// PII rule, and the reason every verdict is a word rather than a value: the
// thing being looked for IS the account's phone identity. So the getters report
// PRESENT / EMPTY / ABSENT / THREW, and objects report their KEY NAMES. A key
// name is Meta's schema; a value would be the account.
func identityShapeScript() string {
	return `JSON.stringify((() => {
		const out = {
			require: typeof window.require === 'function',
			pane: !!document.querySelector('#pane-side'),
			modules: {},
			getters: {},
			identity_key_names: [],
			has_identity: false,
			conn_wid_own: false,
			conn_wid_accessor: false
		};
		if (!out.require) return out;

		const keysOf = (name, pick) => {
			try {
				const m = window.require(name);
				if (!m) { out.modules[name] = 'UNRESOLVED'; return; }
				const target = pick(m) || m;
				out.modules[name] = Object.keys(target).sort().slice(0, 80).join(' ');
			} catch (e) { out.modules[name] = 'THREW'; }
		};
		keysOf('` + string(spa.ModuleConnModel) + `', (m) => m.Conn);
		keysOf('` + string(spa.ModuleSocketModel) + `', (m) => m.Socket);
		keysOf('` + moduleUserPrefsMeUser + `', () => null);

		// The wid question asked BOTH ways, because keysOf above cannot answer
		// it on its own: Object.keys sees only own enumerable properties, so a
		// prototype getter Conn.wid would be missing from the key list and
		// still work. Booleans only — the value would be the account.
		try {
			const c = window.require('` + string(spa.ModuleConnModel) + `').Conn;
			out.conn_wid_own = !!(c && c.__x_wid);
			out.conn_wid_accessor = !!(c && c.wid);
		} catch (e) {}

		try {
			const me = window.require('` + moduleUserPrefsMeUser + `');
			for (const name of [` + jsQuotedList(identityGetters) + `]) {
				const fn = me && me[name];
				if (typeof fn !== 'function') { out.getters[name] = 'ABSENT'; continue; }
				try {
					const v = fn();
					out.getters[name] = v ? 'PRESENT' : 'EMPTY';
					if (v) {
						out.has_identity = true;
						if (out.identity_key_names.length === 0 && typeof v === 'object') {
							// Key names only. One of these values IS the account.
							out.identity_key_names = Object.keys(v).sort();
						}
					}
				} catch (e) { out.getters[name] = 'THREW'; }
			}
		} catch (e) { out.modules['` + moduleUserPrefsMeUser + `'] = 'THREW'; }
		return out;
	})())`
}

// identityShape is what identityShapeScript answers with.
type identityShape struct {
	Require          bool              `json:"require"`
	Pane             bool              `json:"pane"`
	Modules          map[string]string `json:"modules"`
	Getters          map[string]string `json:"getters"`
	IdentityKeyNames []string          `json:"identity_key_names"`
	HasIdentity      bool              `json:"has_identity"`

	// ConnWIDOwn and ConnWIDAccessor are the same question about
	// WAWebConnModel.Conn asked of the own field and of the accessor. Only the
	// second would see a prototype getter, and the key listing above sees
	// neither — see readinessSample.HasConnWIDAccessor.
	ConnWIDOwn      bool `json:"conn_wid_own"`
	ConnWIDAccessor bool `json:"conn_wid_accessor"`
}

// identityShapeBudget and identityShapeTick bound the settle wait. Coarse on
// purpose: this probe answers WHERE the identity lives, not WHEN it arrives —
// the instant is TestRealSPAReadinessTimeline's job, at a 250ms tick.
const (
	identityShapeBudget = 75 * time.Second
	identityShapeTick   = 2 * time.Second
)

// LOOP B1.4b — where does the owner identity actually live in this build?
//
// It exists because the first readiness probe asserted on WAWebConnModel's
// __x_wid and that field was FALSE for a full 90s budget on a session measured
// as CONNECTED, with #pane-side up and 2691 DOM nodes. Two readings competed:
// the session cannot identify its owner, or the probe reads the wrong place.
//
// It is ONE instrument meant to be run against BOTH profiles — the paired one
// via WA_HEADLESS_PROFILE_DIR, the unpaired lab one by default. That is the
// point, and it is what B1.4a could not do: a signal is only a
// discriminator if it was measured false on one and true on the other, and one
// instrument on two profiles is what makes that comparison mean something.
func TestRealSPAOwnerIdentityShape(t *testing.T) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	script := identityShapeScript()
	start := time.Now()

	var shape identityShape
	var identityAt time.Duration
	for deadline := start.Add(identityShapeBudget); time.Now().Before(deadline); {
		var raw string
		err := runner.Do(context.Background(), engine.OpStateProbe, "identity/shape",
			func(ctx context.Context) error { return tab.Evaluate(ctx, script, &raw) })
		if err != nil {
			t.Logf("t+%.1fs the page did not answer: %v", time.Since(start).Seconds(), err)
			time.Sleep(identityShapeTick)
			continue
		}
		if err := json.Unmarshal([]byte(raw), &shape); err != nil {
			t.Fatalf("unexpected shape: %v", err)
		}
		if shape.HasIdentity {
			identityAt = time.Since(start)
			break
		}
		time.Sleep(identityShapeTick)
	}

	t.Logf("window.require present: %v · #pane-side: %v", shape.Require, shape.Pane)
	for name, keys := range shape.Modules {
		t.Logf("  %s keys: %s", name, keys)
	}
	for _, name := range identityGetters {
		t.Logf("  %s.%s() -> %s", moduleUserPrefsMeUser, name, shape.Getters[name])
	}
	t.Logf("  identity object key names: %v", shape.IdentityKeyNames)
	t.Logf("  %s.Conn wid — own field __x_wid: %v · accessor .wid: %v",
		spa.ModuleConnModel, shape.ConnWIDOwn, shape.ConnWIDAccessor)
	t.Logf("OWNER IDENTITY PRESENT: %v (first seen at %s, coarse %s tick)",
		shape.HasIdentity, markString(identityAt), identityShapeTick)

	if !shape.Require {
		t.Fatal("window.require absent; nothing could be enumerated")
	}
}

// LOOP 04.3A — does anything notice a session that has LOST ITS SERVER?
//
// Everything measured up to here is BOOT. EVIDENCIA-SPA.md M3.5 says so in as
// many words: "nada sobre reconexão, expiração de sessão ou sessão revogada no
// servidor: só o boot foi observado." And CAP-04's declared limit is that the
// current liveness probe "descarta o modo de falha medido; não descarta UI
// montada sobre socket morto".
//
// A UI mounted over a dead socket is exactly the untested case, and the only
// honest way to look at it is to PRODUCE it. So this severs the page's network
// with the browser's own emulation — what happens when a laptop lid closes —
// and watches every signal the module has.
//
// It is not a logout, not a signal and not a revocation. Nothing here unlinks
// the account, nothing writes to the profile, and the network is restored before
// the browser goes down so the profile is never left mid-sever.
//
// THREE LEGS, and neither of the other two means anything without the control:
//
//	severed         the same probe over the same window, offline
//	transport-only  the bytes dead, navigator.onLine left TRUE
//	control         the same probe over the same window, untouched
//
// Without the control, "the signal changed" could be the signal drifting on its
// own. The legs are subtests so each can be run alone — together they are about
// six minutes of browser.
//
// The transport-only leg is LOOP 04.3B, and it exists because the severed leg
// cannot answer the question that decides whether its own finding is usable.
// Chrome does not close the socket under the emulation — the frames are silently
// black-holed and readyState stays OPEN, proved in this repository by
// TestBrowserChainSeversTheWebSocketTransport. So the SPA leaving CONNECTED is
// the SPA's own decision, and there are two candidates for it: the `offline` DOM
// event that overrideNetworkState fires, or the SPA noticing its own traffic has
// stalled. The severed leg fires both, so it cannot separate them.
//
// The distinction is not academic. The failures a fleet actually meets — a
// black-holed route, a dead upstream, a captive portal, a hung server — leave
// navigator.onLine TRUE and fire no event at all. If the SPA only reacts to the
// event, the socket is not a discriminator in production and CAP-04 has no
// signal left, screen signals having already been disqualified by F-18. This leg
// withholds the announcement and watches whether the SPA notices anyway.
func TestRealSPALivenessUnderSeveredNetwork(t *testing.T) {
	requireRealSPA(t)

	// Method expressions, so the legs differ by exactly the command they send.
	// A nil set is the control: no command is issued at all, which is what makes
	// it a control rather than a leg that asks for offline=false.
	for _, leg := range []struct {
		name string
		set  func(*engine.Tab, *engine.Runner, bool, string) error
	}{
		{severedLeg, (*engine.Tab).SetNetworkOffline},
		{transportLeg, (*engine.Tab).SetTransportOffline},
		{controlLeg, nil},
	} {
		t.Run(leg.name, func(t *testing.T) { observeSeveredSession(t, leg.name, leg.set) })
	}
}

// The three legs, named once. A literal repeated between the table and the
// reporting is the same bug waiting to diverge.
const (
	severedLeg   = "severed"
	transportLeg = "transport-only"
	controlLeg   = "control"
)

const (
	// severBaseline is how long the steady state is watched before anything is
	// done to it. Short on purpose: it exists to show the signals are STILL, not
	// to characterise them.
	severBaseline = 8 * time.Second
	// severWindow is the observation window — offline for the severed leg,
	// untouched for the control. Long enough to outlast a keepalive: the
	// question is whether the SPA ever notices, and a window shorter than its
	// own heartbeat would answer "no" by construction.
	severWindow = 90 * time.Second
	// severRecovery bounds the wait for the session to come back.
	severRecovery = 60 * time.Second
	// severTick is the sampling interval.
	//
	// It is coarser than the readiness timeline's 250 ms, and deliberately so.
	// F-20 is about a ~500 ms gap that a 250 ms tick cannot resolve; nothing
	// here is at that scale — a network transition and a socket giving up are
	// seconds to tens of seconds. Each tick costs three round trips, so a
	// tighter one would buy resolution this question does not need and pay for
	// it in probe traffic against the account.
	severTick = 1 * time.Second
)

// severSample is one instant of the observation: the SPA's own signals plus
// what each of the module's two verdicts said about them.
type severSample struct {
	at       time.Duration
	signals  readinessSample
	class    spa.PageClass
	liveness spa.LivenessResult
	// probeFailed marks a sample whose SIGNALS could not be read, and it exists
	// because without it the instrument would fabricate findings.
	//
	// A blown StateProbe leaves the signals at their zero values — socket "",
	// pane false, identity false. Scanned naively, that reads as "the socket
	// left CONNECTED and the pane went away at T+n", which is a transition that
	// never happened. The scans below skip these samples for the SPA's own
	// signals; the liveness and page-class verdicts come from their own calls
	// and stay valid, since "the page did not answer" is exactly what they are
	// there to report.
	probeFailed bool
}

// changedFrom reports whether anything worth printing moved.
func (s severSample) changedFrom(prev severSample) bool {
	return s.probeFailed != prev.probeFailed ||
		s.signals.NavigatorOnline != prev.signals.NavigatorOnline ||
		s.signals.HasPaneSide != prev.signals.HasPaneSide ||
		s.signals.HasOwnerIdentity != prev.signals.HasOwnerIdentity ||
		s.signals.MeReadyTriggered != prev.signals.MeReadyTriggered ||
		s.signals.SocketState != prev.signals.SocketState ||
		s.signals.HasQR != prev.signals.HasQR ||
		s.class != prev.class ||
		s.liveness.Alive != prev.liveness.Alive ||
		s.liveness.Class != prev.liveness.Class
}

func (s severSample) log(t *testing.T, phase string) {
	t.Helper()
	if s.probeFailed {
		// The signals are unread, not false. Printing them would be printing
		// zero values as if they were measurements.
		t.Logf("  [%-8s] T+%6.2fs SIGNALS UNREAD (the probe did not answer) "+
			"probe=%-14s liveness=%-5v/%s", phase, s.at.Seconds(), s.class,
			s.liveness.Alive, s.liveness.Class)
		return
	}
	t.Logf("  [%-8s] T+%6.2fs online=%-5v pane=%-5v qr=%-5v identity=%-5v meReady=%-5v "+
		"socket=%-10s nodes=%-5d probe=%-14s liveness=%-5v/%s",
		phase, s.at.Seconds(), s.signals.NavigatorOnline, s.signals.HasPaneSide,
		s.signals.HasQR, s.signals.HasOwnerIdentity, s.signals.MeReadyTriggered,
		s.signals.SocketState, s.signals.DOMNodes, s.class,
		s.liveness.Alive, s.liveness.Class)
}

// observeSeveredSession boots the paired profile, waits for a full ready state,
// and then watches it — with the network cut, or without.
//
// cut is the emulation this leg applies, and nil is the control.
func observeSeveredSession(t *testing.T, leg string,
	cut func(*engine.Tab, *engine.Runner, bool, string) error) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	script := readinessScript(spa.RequiredAtStartup)
	want := len(spa.RequiredAtStartup)

	// The precondition. Watching a session that never came up would measure the
	// boot, not the sever, and the socket gate is what makes "ready" mean the
	// session actually reached the server (M3.7).
	_, marks := sampleReadiness(t, runner, tab, script, want)
	if marks.pane == 0 || marks.connected == 0 {
		t.Fatalf("the session never reached a ready state (pane %s, socket %s %s); "+
			"there is no live session here to sever",
			markString(marks.pane), socketStateConnected, markString(marks.connected))
	}
	t.Logf("READY: pane %s · identity %s · meReady %s · socket %s %s",
		markString(marks.pane), markString(marks.identity), markString(marks.meReady),
		socketStateConnected, markString(marks.connected))

	monitor := &spa.Monitor{Runner: runner, Eval: tab.Evaluate}
	start := time.Now()

	// The event sentinel goes in on EVERY leg, before anything is done to the
	// network, because the comparison between the legs is half the answer: the
	// severed leg is expected to fire an `offline` event, the other two are
	// expected to fire none, and reading the counter in only one of them would
	// leave that expectation untested.
	installNetEventSentinel(t, runner, tab)

	baseline := watchSession(t, runner, tab, monitor, script, "baseline", start, severBaseline)
	steady, haveSteady := lastReadSample(baseline)
	if !haveSteady {
		t.Fatal("no baseline sample with its signals read; the page stopped answering " +
			"before the window opened, so there is no steady state to compare against")
	}
	if steady.signals.SocketState != socketStateConnected || !steady.signals.HasPaneSide {
		t.Fatalf("the steady state is not what this loop needs to sever: socket=%q pane=%v",
			steady.signals.SocketState, steady.signals.HasPaneSide)
	}

	evidence := applyLegCut(t, runner, tab, leg, cut, start)

	window := watchSession(t, runner, tab, monitor, script, leg, start, severWindow)
	evidence.events = readNetEvents(t, runner, tab)
	t.Logf("DOM NETWORK EVENTS over the window: offline=%d online=%d",
		evidence.events.Offline, evidence.events.Online)
	outcome := reportSeverWindow(t, leg, steady, window, evidence)

	if cut == nil {
		return
	}

	if err := cut(tab, runner, false, "sever/restore"); err != nil {
		t.Fatalf("restoring the network: %v", err)
	}
	restoredAt := time.Since(start)
	t.Logf("NETWORK RESTORED at T+%.2fs", restoredAt.Seconds())

	recovery := watchSession(t, runner, tab, monitor, script, "recovery", start, severRecovery)
	reportRecovery(t, restoredAt, recovery, outcome)
}

// severNetwork cuts the page off and guarantees it will be reconnected.
//
// The restore is a t.Cleanup registered IMMEDIATELY, and therefore runs BEFORE
// the tab and browser cleanups openRealSPA registered earlier — cleanups are
// LIFO. That is what keeps the profile from ever being left mid-sever, however
// the rest of the test ends.
func severNetwork(t *testing.T, runner *engine.Runner, tab *engine.Tab,
	cut func(*engine.Tab, *engine.Runner, bool, string) error, leg string, start time.Time) {
	t.Helper()
	if err := cut(tab, runner, true, "sever/offline"); err != nil {
		t.Fatalf("severing the network: %v", err)
	}
	t.Cleanup(func() {
		if err := cut(tab, runner, false, "sever/restore-cleanup"); err != nil {
			t.Errorf("restoring the network: %v", err)
		}
	})
	t.Logf("NETWORK SEVERED (%s) at T+%.2fs", leg, time.Since(start).Seconds())
}

// applyLegCut applies this leg's cut, and takes the reachability readings that
// only the transport-only leg needs.
//
// The two readings BRACKET the cut, and both halves are load-bearing: the one
// before is the internal control — a probe that cannot say OK with the network
// untouched could never prove anything by saying FAIL — and the one after is
// the precondition proper. They are skipped on the other legs because each is a
// real request to the SPA's origin, and the severed leg already has a
// precondition that costs nothing.
func applyLegCut(t *testing.T, runner *engine.Runner, tab *engine.Tab, leg string,
	cut func(*engine.Tab, *engine.Runner, bool, string) error, start time.Time) legEvidence {
	t.Helper()
	var evidence legEvidence
	needsReachability := leg == transportLeg

	if needsReachability {
		evidence.reachBefore = reachability(t, runner, tab, "before")
	}
	if cut != nil {
		severNetwork(t, runner, tab, cut, leg, start)
	}
	if needsReachability {
		evidence.reachDuring = reachability(t, runner, tab, "severed")
		t.Logf("REACHABILITY — with the network untouched: %s · with the transport cut: %s",
			evidence.reachBefore, evidence.reachDuring)
	}
	return evidence
}

// legEvidence is what a leg knows about its own cut that the sampled signals
// cannot say: whether the transport really died, and whether anything told the
// page about it.
type legEvidence struct {
	// reachBefore and reachDuring are the reachability probe's verdicts around
	// the cut. Empty on the legs that do not run it.
	reachBefore, reachDuring string
	// events is what the page's own listeners counted over the window.
	events netEventCounts
}

// netEventSlot is where the page parks its event counters. Named once, because
// the installer and the reader are two scripts and a literal in both is the same
// bug waiting to diverge.
const netEventSlot = "__waHeadlessNetEvents"

// installNetEventSentinelScript counts the DOM network events, and nothing else.
//
// It is the direct observation of the mechanism the transport-only leg is about.
// Inferring "no offline event fired" from navigator.onLine staying true would be
// inference; counting the events the page's own listeners receive is the
// measurement. It is idempotent, so a leg that installed it twice would not
// double-count.
//
// Counts only. It reads no event property, so there is nothing here that could
// carry anything about the account.
const installNetEventSentinelScript = `JSON.stringify((() => {
	if (window.` + netEventSlot + `) return 'already';
	const counts = {offline: 0, online: 0};
	window.` + netEventSlot + ` = counts;
	window.addEventListener('offline', () => { counts.offline++; });
	window.addEventListener('online', () => { counts.online++; });
	return 'installed';
})())`

// readNetEventSentinelScript answers -1 when the sentinel is GONE — a navigation
// would take it with it — because zero would then read as "no event fired",
// which is the optimistic lie an instrument must never tell.
const readNetEventSentinelScript = `JSON.stringify(window.` + netEventSlot +
	` || {offline: -1, online: -1})`

// netEventCounts is how many `offline` and `online` events the page received.
type netEventCounts struct {
	Offline int `json:"offline"`
	Online  int `json:"online"`
}

func installNetEventSentinel(t *testing.T, runner *engine.Runner, tab *engine.Tab) {
	t.Helper()
	var raw string
	if err := runner.Do(context.Background(), engine.OpEvaluate, "netevents/install",
		func(ctx context.Context) error {
			return tab.Evaluate(ctx, installNetEventSentinelScript, &raw)
		}); err != nil {
		t.Fatalf("installing the network-event sentinel: %v", err)
	}
}

func readNetEvents(t *testing.T, runner *engine.Runner, tab *engine.Tab) netEventCounts {
	t.Helper()
	var raw string
	if err := runner.Do(context.Background(), engine.OpStateProbe, "netevents/read",
		func(ctx context.Context) error {
			return tab.Evaluate(ctx, readNetEventSentinelScript, &raw)
		}); err != nil {
		t.Fatalf("reading the network-event sentinel: %v", err)
	}
	var counts netEventCounts
	if err := json.Unmarshal([]byte(raw), &counts); err != nil {
		t.Fatalf("unexpected network-event shape %q: %v", raw, err)
	}
	if counts.Offline < 0 || counts.Online < 0 {
		t.Fatal("the network-event sentinel is gone from the page, so the counts are " +
			"unknown rather than zero")
	}
	return counts
}

// reachabilityAsset is what the transport-only leg's precondition asks for: a
// static icon on the SPA's own origin.
//
// Same-origin because a cross-origin request would report CORS failures and
// network failures the same way. A static asset because the precondition must
// not touch anything the account owns — this is one GET for an icon, and when
// the cut has landed it never leaves the browser at all.
//
// The verdict is about REACHABILITY and not about status: fetch resolves on a
// 404 just as it does on a 200, and rejects only when the request could not be
// made. That is precisely the distinction the precondition needs.
const reachabilityAsset = "favicon.ico"

// reachability asks the page whether it can still reach its own origin.
//
// The cache-busting query is load-bearing twice over: an HTTP cache hit and a
// service-worker cache hit would both answer OK with the transport dead, and the
// precondition would then refuse a run whose cut had actually landed.
func reachability(t *testing.T, runner *engine.Runner, tab *engine.Tab, tag string) string {
	t.Helper()
	url := fmt.Sprintf("%s%s?probe=%s-%d", realSPAURL, reachabilityAsset, tag, time.Now().UnixNano())
	return fetchVerdict(t, runner, tab, url)
}

// watchSession samples the page for a window, printing only the instants where
// something moved.
//
// The clock is on the Go side and every probe carries the Runner's budget, so a
// page that stops answering produces samples that say UNRESPONSIVE rather than a
// hang — which is the whole point of watching a severed session at all.
func watchSession(t *testing.T, runner *engine.Runner, tab *engine.Tab, monitor *spa.Monitor,
	script, phase string, start time.Time, window time.Duration) []severSample {
	t.Helper()

	var samples []severSample
	prev := severSample{signals: readinessSample{DOMNodes: -1}}

	for deadline := time.Now().Add(window); time.Now().Before(deadline); {
		s := severSample{at: time.Since(start)}

		signals, err := oneReadinessSample(runner, tab, script)
		if err != nil {
			// Recorded, not fatal: a probe that blows its budget IS an
			// observation here, and it is the one this loop most wants to see.
			t.Logf("  [%-8s] T+%6.2fs the page did not answer: %v", phase, s.at.Seconds(), err)
			s.probeFailed = true
		}
		s.signals = signals
		s.liveness = monitor.Check(context.Background(), "sever/liveness")
		// Only the CLASS is kept. Probe reads up to 200 characters of body text
		// when nothing structural matches, and on this target that text could be
		// a chat list — so the snapshot is discarded here and never logged, the
		// same discipline the other observation probes in this file follow.
		_, s.class = spa.Probe(context.Background(), runner, tab.Evaluate, "sever/probe")

		samples = append(samples, s)
		if s.changedFrom(prev) {
			s.log(t, phase)
			prev = s
		}
		time.Sleep(severTick)
	}
	return samples
}

// firstSampleWhere returns the earliest sample satisfying pick.
func firstSampleWhere(samples []severSample, pick func(severSample) bool) (severSample, bool) {
	for _, s := range samples {
		if pick(s) {
			return s, true
		}
	}
	return severSample{}, false
}

// firstSignalWhere is firstSampleWhere over the SPA's own signals, and it skips
// samples whose signals were never read — see severSample.probeFailed. A zero
// value is not a measurement, and treating it as one is how an instrument
// reports a transition that did not happen.
func firstSignalWhere(samples []severSample, pick func(readinessSample) bool) (severSample, bool) {
	return firstSampleWhere(samples, func(s severSample) bool {
		return !s.probeFailed && pick(s.signals)
	})
}

// countFailedProbes is how many samples of a window had no signals at all. It is
// reported rather than swallowed: a window that mostly failed is a window whose
// "NEVER" lines mean far less than they look like.
func countFailedProbes(samples []severSample) int {
	n := 0
	for _, s := range samples {
		if s.probeFailed {
			n++
		}
	}
	return n
}

// lastReadSample is the most recent sample whose signals were actually read.
func lastReadSample(samples []severSample) (severSample, bool) {
	for i := len(samples) - 1; i >= 0; i-- {
		if !samples[i].probeFailed {
			return samples[i], true
		}
	}
	return severSample{}, false
}

// sinceOrNever renders how long after a reference instant a sample landed, or
// says it never did. "never" must not print as an instant — an instrument that
// renders absence as T+0.00s lies in the direction of optimism.
func sinceOrNever(s severSample, found bool, ref time.Duration) string {
	if !found {
		return "NEVER"
	}
	return fmt.Sprintf("+%.1fs after the reference (T+%.2fs)", (s.at - ref).Seconds(), s.at.Seconds())
}

// transitionTo names the value a signal moved TO, and says nothing at all when
// it never moved. Printing `(to "")` for a signal that held would read like a
// transition into an empty state, which is not what was measured.
func transitionTo(value string, found bool) string {
	if !found {
		return ""
	}
	return fmt.Sprintf(" (to %q)", value)
}

// signalMark is the first sample at which one signal moved, plus whether it
// moved at all. The boolean is not redundant with the sample: "never moved" and
// "moved at T+0" are different statements, and collapsing them is how an
// instrument reports absence as an instant.
type signalMark struct {
	sample severSample
	seen   bool
}

func (m signalMark) since(ref time.Duration) string { return sinceOrNever(m.sample, m.seen, ref) }

// windowMarks are the instants a window is summarised by.
type windowMarks struct {
	offline, socketLeft, paneGone, identityGone, meReadyGone signalMark
	notAlive, notReady                                       signalMark
}

// collectWindowMarks finds the first movement of each signal.
//
// The SPA's own signals go through firstSignalWhere, which skips samples that
// were never read; the two VERDICTS go through firstSampleWhere, because a
// probe that did not answer is a legitimate input to them.
func collectWindowMarks(samples []severSample) windowMarks {
	signal := func(pick func(readinessSample) bool) signalMark {
		s, ok := firstSignalWhere(samples, pick)
		return signalMark{sample: s, seen: ok}
	}
	verdict := func(pick func(severSample) bool) signalMark {
		s, ok := firstSampleWhere(samples, pick)
		return signalMark{sample: s, seen: ok}
	}
	return windowMarks{
		offline:      signal(func(s readinessSample) bool { return !s.NavigatorOnline }),
		socketLeft:   signal(func(s readinessSample) bool { return s.SocketState != socketStateConnected }),
		paneGone:     signal(func(s readinessSample) bool { return !s.HasPaneSide }),
		identityGone: signal(func(s readinessSample) bool { return !s.HasOwnerIdentity }),
		meReadyGone:  signal(func(s readinessSample) bool { return !s.MeReadyTriggered }),
		notAlive:     verdict(func(s severSample) bool { return !s.liveness.Alive }),
		notReady:     verdict(func(s severSample) bool { return s.class != spa.ClassAppReady }),
	}
}

func (m windowMarks) log(t *testing.T, ref time.Duration) {
	t.Helper()
	t.Logf("  navigator.onLine went false     %s", m.offline.since(ref))
	t.Logf("  socket left %-10s          %s%s", socketStateConnected, m.socketLeft.since(ref),
		transitionTo(m.socketLeft.sample.signals.SocketState, m.socketLeft.seen))
	t.Logf("  #pane-side went false           %s", m.paneGone.since(ref))
	t.Logf("  owner identity went false       %s", m.identityGone.since(ref))
	t.Logf("  meReadyTriggered went false     %s", m.meReadyGone.since(ref))
	t.Logf("  spa.Probe left %-14s   %s%s", spa.ClassAppReady, m.notReady.since(ref),
		transitionTo(string(m.notReady.sample.class), m.notReady.seen))
	t.Logf("  liveness reported NOT alive     %s", m.notAlive.since(ref))
}

// reportSeverWindow is the answer this loop exists to produce.
//
// It ASSERTS nothing about what the SPA should do — the point is to find out.
// The two things it refuses are a severed leg where the sever never landed,
// because then there is no measurement, and a control leg that drifted, because
// then the severed leg's changes are not attributable to the sever.
//
// It reports back what the window did to the liveness verdict, because the
// recovery report has to know: "alive again" is only meaningful if it stopped.
func reportSeverWindow(t *testing.T, phase string, steady severSample,
	samples []severSample, evidence legEvidence) windowOutcome {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("%s: no samples in the window", phase)
	}
	// The closing state comes from the last sample whose signals were READ. The
	// very last sample may be one that did not answer, and reporting its zero
	// values as the end state would invent a collapse in the final second.
	end, haveEnd := lastReadSample(samples)
	if !haveEnd {
		t.Fatalf("%s: not one sample in the window had its signals read", phase)
	}

	ref := steady.at
	marks := collectWindowMarks(samples)
	last := samples[len(samples)-1]

	t.Logf("%s WINDOW — %d samples over %.0fs (%d with the signals unread), from the "+
		"steady state at T+%.2fs", strings.ToUpper(phase), len(samples),
		(last.at - ref).Seconds(), countFailedProbes(samples), ref.Seconds())
	marks.log(t, ref)
	t.Logf("  at the end of the window: socket=%q pane=%v identity=%v probe=%s liveness=%v/%s",
		end.signals.SocketState, end.signals.HasPaneSide, end.signals.HasOwnerIdentity,
		last.class, last.liveness.Alive, last.liveness.Class)

	switch phase {
	case severedLeg:
		reportSeveredVerdict(t, ref, marks, evidence)
	case transportLeg:
		reportTransportOnlyVerdict(t, ref, marks, evidence)
	default:
		assertControlHeldStill(t, end, marks, evidence)
	}
	return windowOutcome{livenessDropped: marks.notAlive.seen}
}

// reportSeveredVerdict states what the liveness check did while the server was
// unreachable, and refuses a run where the sever never reached the page.
func reportSeveredVerdict(t *testing.T, ref time.Duration, marks windowMarks, evidence legEvidence) {
	t.Helper()
	// The instrument's own precondition. If the emulation did not reach the
	// page, "nothing changed" is a statement about our emulation and not about
	// the session.
	if !marks.offline.seen {
		t.Fatal("the sever never reached the page: navigator.onLine stayed true for " +
			"the whole window, so nothing measured here is attributable to a lost server")
	}
	// The counterpart of the transport-only leg's guard, and the reason the
	// severed leg cannot answer LOOP 04.3B's question: here the page IS told.
	if evidence.events.Offline < 1 {
		t.Errorf("navigator.onLine went false but the page counted %d offline events; "+
			"the two legs are then not the comparison they are documented to be",
			evidence.events.Offline)
	}
	if marks.notAlive.seen {
		t.Logf("VERDICT: the liveness check stopped reporting ALIVE %s", marks.notAlive.since(ref))
		return
	}
	t.Log("VERDICT: the liveness check reported this session ALIVE for the whole " +
		"window with its network cut. It is NOT sufficient on its own — the " +
		"phase 6 hazard, measured rather than reasoned about.")
}

// reportTransportOnlyVerdict is LOOP 04.3B's answer, and its preconditions are
// the whole reason the answer can be trusted.
//
// The severed leg's precondition — navigator.onLine went false — is exactly what
// this leg must NOT do, so it needs a different one. It has four checks, in two
// pairs, and both pairs are necessary:
//
// THE CUT LANDED. An in-page fetch to a static asset on the SPA's own origin
// answers OK before the cut and FAIL after it. It is a positive check: it
// exercises the real network stack from inside the page, and the before-leg
// proves the probe is capable of saying OK, so the FAIL is a measurement rather
// than a probe that cannot succeed. It measures HTTP, not the WebSocket, and the
// bridge between the two is not an assumption: the same two emulation commands
// are proved to black-hole WebSocket frames in both directions, against a server
// this repository owns, by TestBrowserChainSeversTheWebSocketTransport.
//
// NOTHING ANNOUNCED IT. navigator.onLine must have stayed TRUE for the whole
// window, and the page must have counted ZERO `offline` events. The counter is
// monotonic, so zero at the end is zero throughout — which is what licenses
// reading any socket departure inside the window as the SPA's own detection
// rather than as an event handler firing. Without these two, this leg would just
// be the severed leg with a slower guard.
func reportTransportOnlyVerdict(t *testing.T, ref time.Duration, marks windowMarks, evidence legEvidence) {
	t.Helper()
	if evidence.reachBefore != fetchVerdictOK {
		t.Fatalf("the reachability probe answered %q with the network untouched, want %q; "+
			"a probe that cannot say OK could never prove a cut by saying FAIL",
			evidence.reachBefore, fetchVerdictOK)
	}
	if evidence.reachDuring != fetchVerdictFail {
		t.Fatalf("the transport cut never landed: the reachability probe still answered %q "+
			"with the emulation active, so nothing measured here is attributable to a lost "+
			"server", evidence.reachDuring)
	}
	if marks.offline.seen {
		t.Fatalf("navigator.onLine went false at T+%.2fs; this leg exists to leave it alone, "+
			"so its whole premise is gone and the socket timeline cannot be read as "+
			"self-detection", marks.offline.sample.at.Seconds())
	}
	if evidence.events.Offline != 0 {
		t.Fatalf("the page counted %d offline events with the navigator untouched; something "+
			"announced the cut after all, so a socket departure cannot be attributed to the "+
			"SPA noticing by itself", evidence.events.Offline)
	}

	if marks.socketLeft.seen {
		t.Logf("ANSWER: the SPA detects the stall ITSELF. The socket left %s %s%s with "+
			"navigator.onLine still true and zero offline events fired. The socket state is a "+
			"discriminator for the failures production actually meets, not an artifact of the "+
			"emulation announcing itself.", socketStateConnected, marks.socketLeft.since(ref),
			transitionTo(marks.socketLeft.sample.signals.SocketState, marks.socketLeft.seen))
		return
	}
	t.Logf("ANSWER: the SPA does NOT detect the stall. The socket stayed %s for the whole "+
		"window with its transport dead and nothing announcing it. This CONTRADICTS F-22, "+
		"which measured the departure at ~34s over three runs — either the window was too "+
		"short for this build or the SPA changed. Either way the socket would not be a "+
		"discriminator for a black-holed route, a dead upstream or a hung server, and CAP-04 "+
		"would have no working liveness signal left: F-18 already disqualified the screen "+
		"ones. Do not soften this; re-measure it.", socketStateConnected)
}

// assertControlHeldStill is the leg that makes the severed one mean something.
// Any movement here and the severed timeline is not attributable to the sever,
// so this is the one leg that asserts.
func assertControlHeldStill(t *testing.T, end severSample, marks windowMarks, evidence legEvidence) {
	t.Helper()
	if !end.signals.NavigatorOnline {
		t.Error("the control leg went offline on its own; the legs are then not comparable")
	}
	if evidence.events.Offline != 0 {
		t.Errorf("the control leg counted %d offline events with nothing done to it",
			evidence.events.Offline)
	}
	if marks.socketLeft.seen {
		t.Errorf("the control leg's socket left %s at T+%.2fs with nothing done to it, so a "+
			"socket change in the severed leg cannot be attributed to the sever",
			socketStateConnected, marks.socketLeft.sample.at.Seconds())
	}
	if marks.notAlive.seen {
		t.Errorf("the control leg reported NOT alive at T+%.2fs with nothing done to it",
			marks.notAlive.sample.at.Seconds())
	}
	if marks.paneGone.seen || marks.identityGone.seen {
		t.Errorf("the control leg lost pane (%v) or identity (%v) with nothing done to it",
			marks.paneGone.seen, marks.identityGone.seen)
	}
	t.Log("CONTROL: every signal held still over the same window with the network untouched")
}

// windowOutcome is what a window did to the module's liveness verdict, which
// the recovery report needs: "alive again" is only meaningful if it stopped.
type windowOutcome struct {
	livenessDropped bool
}

// reportRecovery says whether, and how fast, the session came back.
func reportRecovery(t *testing.T, restoredAt time.Duration, samples []severSample, window windowOutcome) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatal("no samples after the network was restored")
	}
	back, recovered := firstSignalWhere(samples, func(s readinessSample) bool {
		return s.SocketState == socketStateConnected
	})
	alive, aliveAgain := firstSampleWhere(samples, func(s severSample) bool {
		return s.liveness.Alive
	})
	last := samples[len(samples)-1]
	end, haveEnd := lastReadSample(samples)
	if !haveEnd {
		t.Fatal("not one sample after the restore had its signals read")
	}

	t.Logf("RECOVERY — %d samples over %.0fs (%d with the signals unread) after the "+
		"restore at T+%.2fs", len(samples), (last.at - restoredAt).Seconds(),
		countFailedProbes(samples), restoredAt.Seconds())
	t.Logf("  socket back to %-10s       %s", socketStateConnected,
		sinceOrNever(back, recovered, restoredAt))
	// Only meaningful if it ever stopped. Printing "+0.0s" for a verdict that
	// never dropped would read as a recovery that did not happen.
	if window.livenessDropped {
		t.Logf("  liveness alive again            %s", sinceOrNever(alive, aliveAgain, restoredAt))
	} else {
		t.Log("  liveness alive again            n/a — it never stopped saying alive")
	}
	t.Logf("  at the end: socket=%q pane=%v identity=%v probe=%s",
		end.signals.SocketState, end.signals.HasPaneSide,
		end.signals.HasOwnerIdentity, last.class)

	// The session must be handed back healthy: this profile is not disposable,
	// and leaving it disconnected would be a side effect of the measurement.
	if !recovered {
		t.Errorf("the socket never returned to %s within %s after the network came back",
			socketStateConnected, severRecovery)
	}
}

// --- Profile resolution: unit tests, no browser, no profile touched ---------
//
// These do not carry the TestRealSPA prefix and are not gated behind the
// toggle, on purpose: they are the part of this file that can be locked without
// going near web.whatsapp.com, and locking it is what makes the override safe
// to ship.

// wantLabProfileSuffix repeats labProfileDir's value on purpose, which is the
// one place in this file where NOT extracting a shared constant is the right
// call. An expectation computed from the value under test agrees with it by
// construction and could never catch a change to it, and "the default is the
// disposable lab profile" is precisely the property worth trapping. If this
// ever diverges from labProfileDir, the failure is the point.
const wantLabProfileSuffix = ".lab/test-account-profile"

// clearProfileOverride expresses "no override" the way the code sees it: the
// resolvers read os.Getenv, for which an empty value and an absent variable are
// the same thing. Going through t.Setenv also restores whatever the developer's
// shell had.
func clearProfileOverride(t *testing.T) {
	t.Helper()
	t.Setenv(profileDirOverride, "")
}

func TestObservationProfileDirDefaultsToLabProfile(t *testing.T) {
	clearProfileOverride(t)

	dir, overridden, err := observationProfileDir()
	if err != nil {
		t.Fatalf("observationProfileDir: %v", err)
	}
	if overridden {
		t.Error("reported an override with the variable unset")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	want := filepath.Join(cwd, wantLabProfileSuffix)
	if dir != want {
		t.Errorf("default profile = %q, want the lab profile %q", dir, want)
	}
}

func TestObservationProfileDirFollowsOverride(t *testing.T) {
	// An absolute override comes back untouched; a relative one is resolved
	// against the working directory, because Chrome is launched from wherever
	// the test binary happens to run.
	absolute := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	const relative = "some/candidate-profile"

	cases := map[string]struct{ set, want string }{
		"absolute": {set: absolute, want: absolute},
		"relative": {set: relative, want: filepath.Join(cwd, relative)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(profileDirOverride, tc.set)

			dir, overridden, err := observationProfileDir()
			if err != nil {
				t.Fatalf("observationProfileDir: %v", err)
			}
			if !overridden {
				t.Error("did not report an override with the variable set")
			}
			if !filepath.IsAbs(dir) {
				t.Errorf("profile = %q, want an absolute path", dir)
			}
			if dir != tc.want {
				t.Errorf("profile = %q, want %q", dir, tc.want)
			}
		})
	}
}

// The guard. Pairing links an account INTO the profile it opens, so following
// the override would let one run re-pair a profile the caller only meant to
// look at.
func TestPairingProfileDirRefusesOverride(t *testing.T) {
	elsewhere := t.TempDir()
	t.Setenv(profileDirOverride, elsewhere)

	dir, err := pairingProfileDir()
	if err == nil {
		t.Fatalf("pairing accepted the overridden profile %q; it MUTATES what it opens", dir)
	}
	if dir != "" {
		t.Errorf("refused but still returned a profile: %q", dir)
	}
	if !strings.Contains(err.Error(), profileDirOverride) {
		t.Errorf("refusal %q does not name %s, so nobody can tell why it refused",
			err, profileDirOverride)
	}
}

func TestPairingProfileDirUsesLabProfileWithoutOverride(t *testing.T) {
	clearProfileOverride(t)

	dir, err := pairingProfileDir()
	if err != nil {
		t.Fatalf("pairingProfileDir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if want := filepath.Join(cwd, wantLabProfileSuffix); dir != want {
		t.Errorf("pairing profile = %q, want the lab profile %q", dir, want)
	}
}

func TestRequireExistingProfile(t *testing.T) {
	existing := t.TempDir()
	if err := requireExistingProfile(existing); err != nil {
		t.Errorf("rejected an existing profile: %v", err)
	}

	absent := filepath.Join(existing, "never-created")
	if err := requireExistingProfile(absent); err == nil {
		t.Error("accepted an absent profile; it would boot empty and read as unpaired")
	}

	file := filepath.Join(existing, "not-a-directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := requireExistingProfile(file); err == nil {
		t.Error("accepted a file as a profile directory")
	}
}

// anySample reports whether any sample of the run satisfied pick.
//
// It exists for the RECORDED controls, whose question is "was this ever true
// during the boot?" rather than readinessMarks' "when did it turn on?".
func anySample(samples []readinessSample, pick func(readinessSample) bool) bool {
	for _, s := range samples {
		if pick(s) {
			return true
		}
	}
	return false
}

// markString renders an instant, or says the signal never arrived.
func markString(d time.Duration) string {
	if d == 0 {
		return "NEVER"
	}
	return fmt.Sprintf("T+%.2fs", d.Seconds())
}

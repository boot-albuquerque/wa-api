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
			socket_state: socketState
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

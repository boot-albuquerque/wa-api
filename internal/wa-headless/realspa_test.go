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

// openRealSPA launches the lab profile and navigates to the target.
func openRealSPA(t *testing.T, runner *engine.Runner) (*engine.Browser, *engine.Tab) {
	t.Helper()
	requireRealSPA(t)
	binary := findChrome(t)

	profile, err := filepath.Abs(labProfileDir)
	if err != nil {
		t.Fatalf("resolving the lab profile: %v", err)
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatalf("creating the lab profile: %v", err)
	}

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
func openLabProfileHeadful(t *testing.T, runner *engine.Runner) *engine.Tab {
	t.Helper()
	binary := findChrome(t)

	profile, err := filepath.Abs(labProfileDir)
	if err != nil {
		t.Fatalf("resolving the lab profile: %v", err)
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatalf("creating the lab profile: %v", err)
	}

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
	// The readiness candidates, chosen by MEASUREMENT rather than by guess.
	//
	// LOOP B1.4a read the key names of WAWebConnModel on an unpaired login
	// screen. What was there: __x_ref, __x_refExpiry, __x_refTTL (the QR),
	// __x_id, __x_stale, __x_meReadyTriggered. What was NOT there: __x_wid.
	//
	// That disqualifies everything the first version of this probe used —
	// window.require and all eight startup modules resolve at T+0.01s on the
	// LOGIN screen, so an inventory cannot be evidence of readiness. It also
	// explains the first instrument's mistake: it accepted `ref`, which is the
	// QR's own field, and so went true with no session at all.
	//
	// HasOwnerWID is presence only. The value is the account's JID and is never
	// read here.
	HasOwnerWID bool `json:"has_owner_wid"`
	// MeReadyTriggered is the SPA's own flag for "the account finished
	// loading". A boolean of Meta's, not an inference of ours.
	MeReadyTriggered bool `json:"me_ready_triggered"`
	// SocketState is an enum of Meta's (CONNECTED, OPENING…), not user data.
	SocketState string `json:"socket_state"`
}

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
		let hasWid = false, meReady = false, socketState = '';
		if (hasRequire) {
			for (const name of [` + strings.Join(names, ",") + `]) {
				try { if (window.require(name)) resolved++; } catch (e) {}
			}
			try {
				const m = window.require('WAWebConnModel');
				const c = m && (m.Conn || m.default || m);
				// Presence only. The value is the account's JID.
				hasWid = !!(c && c.__x_wid);
				meReady = !!(c && c.__x_meReadyTriggered === true);
			} catch (e) {}
			try {
				const m = window.require('WAWebSocketModel');
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
			has_owner_wid: hasWid,
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
func TestRealSPAReadinessTimeline(t *testing.T) {
	runner := engine.NewRunner()
	_, tab := openRealSPA(t, runner)

	script := readinessScript(spa.RequiredAtStartup)
	want := len(spa.RequiredAtStartup)

	samples, marks := sampleReadiness(t, runner, tab, script, want)
	paneAt, modulesAt, connAt := marks.pane, marks.modules, marks.conn

	logTimeline(t, samples, want)
	// "never" must not print as T+0.00s. A zero mark means the signal never
	// turned on, and rendering that as an instant reads like "immediately" —
	// an instrument that lies in the direction of optimism.
	t.Logf("  #pane-side at        %s", markString(paneAt))
	t.Logf("  module inventory at  %s", markString(modulesAt))
	t.Logf("  owner wid present at %s", markString(connAt))

	if paneAt == 0 {
		t.Fatal("#pane-side never appeared on a paired profile")
	}
	if modulesAt == 0 {
		t.Fatal("the module inventory never resolved; the SPA never finished booting")
	}

	// The verdict this loop exists to produce.
	const tolerance = 250 * time.Millisecond
	switch {
	case connAt == 0:
		t.Error("EARLY_MARKER: #pane-side appeared but the owner identity never did — " +
			"the classifier would report READY for a session that cannot act")
	case connAt > paneAt+tolerance:
		t.Errorf("EARLY_MARKER: #pane-side at T+%.2fs but the owner identity only at "+
			"T+%.2fs — a %.2fs window where the classifier says READY and the engine "+
			"cannot act as anybody", paneAt.Seconds(), connAt.Seconds(),
			(connAt - paneAt).Seconds())
	default:
		t.Logf("SAFE_READY_MARKER: the owner identity was already there when #pane-side "+
			"appeared (wid T+%.2fs <= pane T+%.2fs + tolerance)",
			connAt.Seconds(), paneAt.Seconds())
	}
	// Recorded, not asserted: the inventory is known to be true on the login
	// screen, so it can never be the discriminating signal.
	t.Logf("  (module inventory at T+%.2fs — measured true on the LOGIN screen too, "+
		"so it is not evidence of readiness)", modulesAt.Seconds())
}

// readinessMarks are the instants this loop compares.
type readinessMarks struct{ pane, modules, conn time.Duration }

// record stamps the first time each signal turned on, and reports whether all
// of them have. First-time-only: a signal that flickers must keep its earliest
// instant, because the question is when readiness BECAME true.
func (m *readinessMarks) record(s readinessSample, want int) bool {
	if m.pane == 0 && s.HasPaneSide {
		m.pane = s.At
	}
	if m.modules == 0 && s.ModulesResolved == want {
		m.modules = s.At
	}
	if m.conn == 0 && s.HasOwnerWID {
		m.conn = s.At
	}
	return m.pane > 0 && m.modules > 0 && m.conn > 0
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
			s.HasOwnerWID == prev.HasOwnerWID && s.MeReadyTriggered == prev.MeReadyTriggered &&
			s.SocketState == prev.SocketState {
			continue
		}
		t.Logf("  T+%6.2fs nodes=%-6d pane=%-5v modules=%d/%d wid=%-5v meReady=%-5v socket=%s",
			s.At.Seconds(), s.DOMNodes, s.HasPaneSide,
			s.ModulesResolved, want, s.HasOwnerWID, s.MeReadyTriggered, s.SocketState)
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

// markString renders an instant, or says the signal never arrived.
func markString(d time.Duration) string {
	if d == 0 {
		return "NEVER"
	}
	return fmt.Sprintf("T+%.2fs", d.Seconds())
}

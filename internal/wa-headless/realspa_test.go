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
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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

// launchObservationProfile boots the observation profile with the PRODUCTION
// launcher and returns the browser plus the resolved profile path.
//
// Split out of openRealSPA so that a probe which needs a LIVE browser but no
// page — the SingletonLock observation of LOOP 03.9 — boots through the same
// code path the production parser will face, instead of a hand-rolled exec that
// could differ in exactly the way being measured.
//
// The clean stop is registered here, so no caller can boot without one.
func launchObservationProfile(t *testing.T, runner *engine.Runner) (*engine.Browser, string) {
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
	return browser, profile
}

// openRealSPA launches the observation profile and navigates to the target.
func openRealSPA(t *testing.T, runner *engine.Runner) (*engine.Browser, *engine.Tab) {
	t.Helper()
	browser, _ := launchObservationProfile(t, runner)

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
	// RTT is how long THIS sample's single round trip took.
	//
	// It is the instrument measuring ITSELF, and M7 is why it exists: under CPU
	// contention the renderer answers more slowly, so the real spacing between
	// samples stops being the sleep and becomes sleep+RTT. An arm that reported
	// a 50 ms tick while its round trips cost 400 ms would be claiming a
	// resolution it does not have — which is F-20 all over again, one layer up.
	RTT time.Duration `json:"-"`

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

	samples, marks := sampleReadiness(t, runner, tab, script, want, readinessTick)

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
type readinessMarks struct {
	pane, modules, identity, meReady, connected time.Duration
	// openingFirst is the first instant the socket was seen reporting OPENING.
	//
	// RECORDED, never part of the stop condition, so adding it changed nothing
	// about what M6 measured — the M6 numbers stay comparable.
	//
	// It exists because M6 published its "window in OPENING" as
	// connected - meReady, which is an ANCHOR CHOICE and not the state itself.
	// M7 needs both: the M6-comparable figure, and the direct one, which is
	// this mark to connected. Where the two disagree, the disagreement is the
	// finding rather than a detail — see EVIDENCIA-SPA.md M7.
	openingFirst time.Duration
}

// socketStateOpening is Meta's own name for a socket that is negotiating. It is
// both the boot state and the state M5 measured a lost server falling into,
// which is the whole reason a DURATION is needed to tell them apart.
const socketStateOpening = "OPENING"

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
	if m.openingFirst == 0 && s.SocketState == socketStateOpening {
		m.openingFirst = s.At
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
// The tick is a PARAMETER rather than the package constant because M6 and M7
// need different resolutions from the same instrument: M6's runs are frozen
// evidence taken at 250 ms, and M7 has to resolve a window that 250 ms turns
// into one or two ticks (F-20, EVIDENCIA-SPA.md M6.5). Passing it keeps one
// sampler instead of two, which is what makes the two measurements comparable
// at all.
func sampleReadiness(t *testing.T, runner *engine.Runner, tab *engine.Tab,
	script string, want int, tick time.Duration) ([]readinessSample, readinessMarks) {
	t.Helper()

	start := time.Now()
	var samples []readinessSample
	var marks readinessMarks

	for deadline := start.Add(readinessBudget); time.Now().Before(deadline); {
		before := time.Now()
		s, err := oneReadinessSample(runner, tab, script)
		s.At = time.Since(start)
		s.RTT = time.Since(before)
		if err != nil {
			t.Logf("t+%.2fs probe failed: %v", s.At.Seconds(), err)
			time.Sleep(tick)
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
		time.Sleep(tick)
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
	_, marks := sampleReadiness(t, runner, tab, script, want, readinessTick)
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

// --- LOOP 03.9: the on-disk FORM of SingletonLock, measured -----------------

const (
	// The three names Chromium leaves behind. engine spells them in its own
	// unexported constants and this file spells them again ON PURPOSE: the
	// probe exists to check the engine's assumption from OUTSIDE, and a name
	// imported from the code under test would agree with it by construction.
	singletonLockName   = "SingletonLock"
	singletonCookieName = "SingletonCookie"
	singletonSocketName = "SingletonSocket"
)

// countProfileFiles counts entries under dir, recursively.
//
// The COUNT, never the names: profile hygiene ("the profile did not shrink") is
// answerable with a number, and a number cannot carry an account.
func countProfileFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("counting %s: %v", dir, err)
	}
	return n
}

// replicaOfLock rebuilds, in a temp directory, the profile shape Chromium left
// behind — with the target string read from the REAL lock, byte for byte.
//
// The production parser is exercised against this replica and NEVER against the
// live profile, and the reason is the defect being measured: if the format did
// diverge, ReclaimProfile falls into its "unreadable" branch and DELETES. Doing
// that to a live paired profile is precisely the accident this loop exists to
// rule out. The replica loses nothing that the parser reads — readLockHolder
// reads the symlink target, not the directory around it.
func replicaOfLock(t *testing.T, target string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, singletonLockName)); err != nil {
		t.Fatalf("replicating the measured lock: %v", err)
	}
	for _, name := range []string{singletonCookieName, singletonSocketName} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("replicating %s: %v", name, err)
		}
	}
	return dir
}

// --- Zero PII in the probe's output: a MECHANISM, not a warning -------------
//
// The measured lock target embeds this machine's hostname, which can carry a
// person's name, and zero PII is a BLOCKER here. A comment telling the reader
// to redact before pasting is not a guard: the moment that matters is a
// FAILURE, when someone copies the output into an issue, a CI log or a commit
// message — and nobody re-reads a comment then. So the probe must not be able
// to emit the raw value on any path, the failure path included.
//
// Two pieces, because the value reaches the output from two directions:
//
//   - describeLockTarget, for the string this file measured itself;
//   - hostRedactor, for the strings this file does NOT compose. The engine
//     writes the host INTO its own text — `pid %d on %q is still running`
//     (engine/profile.go:215) — so it comes back inside the error and inside
//     ReclaimResult.Reason, and every reason and every error printed by this
//     probe goes through the redactor before it is formatted.
//
// Redaction must not cost the diagnosis: what survives is the shape (hyphen
// count, dot, .local suffix), the separator and the pid, which is what a reader
// needs to tell WHAT diverged.

// redactedHost is the placeholder the documents already use, so a log line and
// a HOUSEKEEP entry read the same.
const redactedHost = "<HOST>"

// hostRedactor rewrites every known spelling of this machine's name to
// redactedHost.
//
// Built from values that were OBSERVED, never from a pattern: a pattern that
// fails to match leaks silently, and an observed value cannot fail to match
// itself. Over-redaction is the safe direction and is accepted.
type hostRedactor struct{ replacer *strings.Replacer }

func newHostRedactor(secrets ...string) hostRedactor {
	// Longest first. strings.Replacer matches in argument order, so a bare
	// hostname listed ahead of its ".local" spelling would replace the stem and
	// leave the suffix dangling in the output.
	sorted := append([]string(nil), secrets...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	pairs := make([]string, 0, 2*len(sorted))
	for _, s := range sorted {
		if s == "" {
			continue
		}
		pairs = append(pairs, s, redactedHost)
	}
	return hostRedactor{replacer: strings.NewReplacer(pairs...)}
}

// scrub renders v and takes the host out of the rendering. It accepts `any` so
// that an error or a ReclaimResult field is formatted HERE, and no caller ever
// holds the raw text long enough to pass it to a Logf by accident.
func (h hostRedactor) scrub(v any) string {
	return h.replacer.Replace(fmt.Sprint(v))
}

// hostSecretsOf lists every spelling of the machine name that could show up in
// output: what os.Hostname() returns, the host half of the measured target
// (they should be equal — proving that is the point of the probe — and the
// redactor must not depend on it), and each of those without the ".local"
// suffix macOS appends.
func hostSecretsOf(target string) []string {
	secrets := []string{hostnameOrEmpty(), hostHalfOf(target)}
	for _, s := range append([]string(nil), secrets...) {
		if trimmed := strings.TrimSuffix(s, ".local"); trimmed != s && trimmed != "" {
			secrets = append(secrets, trimmed)
		}
	}
	return secrets
}

// hostHalfOf returns what the production rule reads as the host: everything
// before the LAST hyphen. With no hyphen it returns the whole string, which
// over-redacts — the safe direction.
func hostHalfOf(target string) string {
	if i := strings.LastIndex(target, "-"); i > 0 {
		return target[:i]
	}
	return target
}

// describeLockTarget renders the measured target with the host half replaced by
// the placeholder, keeping the separator and the pid exactly as written plus
// the SHAPE of the host: the hyphen count is the property the "pid after the
// LAST hyphen" rule turns on, and the dot and the .local suffix are what tell a
// macOS name apart from a container one.
func describeLockTarget(target string) string {
	// Named apart from the redacted case: "no holder was reported" and "a holder
	// we are not printing" must not read the same on a failure line.
	if target == "" {
		return `"" (empty: nothing was reported as the holder)`
	}
	i := strings.LastIndex(target, "-")
	if i <= 0 {
		return fmt.Sprintf("%q (no usable hyphen: len=%d hyphens=%d dot=%v)",
			redactedHost, len(target), strings.Count(target, "-"),
			strings.Contains(target, "."))
	}
	host, pid := target[:i], target[i+1:]
	return fmt.Sprintf("%q (host: len=%d hyphens=%d dot=%v endsLocal=%v)",
		redactedHost+"-"+pid, len(host), strings.Count(host, "-"),
		strings.Contains(host, "."), strings.HasSuffix(host, ".local"))
}

// TestSingletonLockProbeOutputCannotCarryTheHost is the guard for the mechanism
// above, and it is NOT gated behind WA_HEADLESS_REAL_SPA on purpose: a
// redaction that only runs when someone opts into a browser run is a redaction
// nobody checks. It opens no browser and touches no profile.
//
// The strings it redacts are not written by hand. They come out of the REAL
// ReclaimProfile against a synthetic lock, because the leak this guards is
// precisely the one this file does not author: if engine ever changes how it
// spells the host into its error, a hand-written sample would keep passing
// while the probe started leaking (ARMADILHAS 1).
func TestSingletonLockProbeOutputCannotCarryTheHost(t *testing.T) {
	// A name of the shape that would be PII if it ever escaped.
	const host = "alice-macbook.local"
	const target = host + "-4242"

	held, heldErr := engine.ReclaimProfile(replicaOfLock(t, target), engine.ReclaimOptions{
		Hostname:     host,
		ProcessAlive: func(int) bool { return true },
	})
	if !errors.Is(heldErr, engine.ErrProfileHeldByLiveBrowser) {
		t.Fatalf("the sample did not reach the live-holder path: err=%v", heldErr)
	}
	// The premise of the guard: production really does put the host in its
	// output. If this stops being true the redaction is still correct, but the
	// guard would be testing nothing, so it must be re-read rather than trusted.
	if !strings.Contains(heldErr.Error(), host) {
		t.Fatalf("engine no longer spells the host into its error (%q): this guard "+
			"has stopped exercising the leak it exists for", heldErr)
	}

	red := newHostRedactor(hostSecretsOf(target)...)
	// hostSecretsOf reads os.Hostname() for the real run; here the synthetic
	// name is what must disappear, and it is covered by the target's host half.
	emitted := map[string]string{
		"scrubbed error":       red.scrub(heldErr),
		"scrubbed reason":      red.scrub(held.Reason),
		"scrubbed lock holder": red.scrub(held.LockHolder),
		"described target":     describeLockTarget(target),
		"described split":      describeSplit(splitLockTargetAt(target, strings.LastIndex(target, "-"))),
	}
	for what, got := range emitted {
		assertCarriesNoHost(t, what, got, host, "alice", "macbook")
	}

	// Redaction that eats the diagnosis is its own defect: a reader of a
	// DIVERGENCE message still has to be able to tell what diverged.
	assertKeepsAll(t, "the scrubbed error", red.scrub(heldErr), "4242", redactedHost)
	assertKeepsAll(t, "the described target", describeLockTarget(target),
		"4242", "hyphens=1", "endsLocal=true")
}

// assertCarriesNoHost fails if an emitted string contains any spelling of the
// host, or any fragment of it that would identify a person.
func assertCarriesNoHost(t *testing.T, what, got string, leaks ...string) {
	t.Helper()
	for _, leak := range leaks {
		if strings.Contains(got, leak) {
			t.Errorf("%s leaked %q: %s", what, leak, got)
		}
	}
}

// assertKeepsAll fails if redaction took the diagnosis with the host.
func assertKeepsAll(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("%s lost %q, which a reader needs to tell what diverged: %s",
				what, want, got)
		}
	}
}

// --- Is the rule under test actually EXERCISED by this host's name? ---------
//
// The probe checks that production reads the measured target as "<host>-<pid>",
// splitting on the LAST hyphen (engine/profile.go). Whether that assertion can
// FAIL depends on the machine: with a hostname like "buildbox" the first hyphen
// IS the last one, so a strings.LastIndex -> strings.Index mutation reads the
// target identically and the probe passes while verifying nothing. On a host
// like "wa-headless-pod-7" the same mutation makes the target unparseable and
// the probe fails.
//
// That dependency used to be nowhere: not asserted, not even written down. It
// is now checked against the hostname actually observed, and a host that cannot
// discriminate produces a SKIP naming the reason instead of a silent pass —
// which is the trap of ARMADILHAS 1 inverted, an instrument that reports more
// than it measured.
//
// SKIP and not FAIL, deliberately: a hyphen-less hostname is a legitimate
// machine, not a defect, and failing there would be a false alarm that teaches
// the next reader to delete the check (the same cost H10 names). Nothing is
// lost by skipping, because the RULE is locked elsewhere and ungated —
// engine.TestLockHolderSplitsOnTheLastHyphen (engine/profile_test.go) proves it
// with a controlled double on every `make check`, on any host. What the skip
// protects is this probe's CLAIM about what a given run verified.

// lockSplit is one reading of a lock target: host before a hyphen, pid after it.
type lockSplit struct {
	host string
	pid  int
	ok   bool
}

// splitLockTargetAt mirrors the parse in engine/profile.go (readLockHolder: pid
// after the chosen hyphen, host before it, and the reading only counts if the
// pid is a positive integer).
//
// It exists to compare the LAST-hyphen rule against its FIRST-hyphen mutation
// on ONE measured value, and for nothing else. It never answers "is the format
// right" — the authoritative reading of that value is the one the production
// ReclaimProfile does a few lines up, through the typed error. A double that
// stood in for the parser would be exactly the trap ARMADILHAS 1 describes.
func splitLockTargetAt(target string, i int) lockSplit {
	if i <= 0 {
		return lockSplit{}
	}
	pid, err := strconv.Atoi(target[i+1:])
	if err != nil || pid <= 0 {
		return lockSplit{}
	}
	return lockSplit{host: target[:i], pid: pid, ok: true}
}

// lockTargetDiscriminatesLastHyphen answers the one question the probe cannot
// answer by running production: on THIS measured string, would splitting on the
// first hyphen instead of the last produce a different reading?
func lockTargetDiscriminatesLastHyphen(target string) bool {
	return splitLockTargetAt(target, strings.Index(target, "-")) !=
		splitLockTargetAt(target, strings.LastIndex(target, "-"))
}

// describeSplit renders a reading without the host in it.
func describeSplit(s lockSplit) string {
	if !s.ok {
		return "UNPARSEABLE (the reclaim would call the lock unreadable and DELETE)"
	}
	return fmt.Sprintf("host=%s(hyphens=%d) pid=%d", redactedHost,
		strings.Count(s.host, "-"), s.pid)
}

// TestRealSPASingletonLockFormat measures what Chromium actually writes into
// SingletonLock, and whether the production parser agrees with it.
//
// HOUSEKEEP H4. ReclaimProfile decides between DELETE and REFUSE by reading the
// lock as a SYMLINK whose target is "<hostname>-<pid>", a convention taken from
// chrome/browser/process_singleton_posix.cc. Until this probe, no real profile
// had ever been inspected on the Chromium this project runs. A divergence is
// SILENT: readLockHolder falls into its "unreadable" branch, the reclaim deletes
// without proving a thing, and the guard of invariant "reclaim of Singleton at
// boot is mandatory in a container" (HANDOFF §6, numbered 13) stops guarding.
//
// METHOD, and the obvious one does NOT work: a cleanly stopped profile never
// shows the file — the clean shutdown removes it, measured as M3 in
// EVIDENCIA-SPA.md and re-confirmed by the tail of this test. So the inspection
// happens with the browser ALIVE, which is why this probe boots one.
//
// ZERO PII, BY MECHANISM: the measured target embeds this machine's hostname,
// which can carry a person's name. Nothing here prints it — describeLockTarget
// and hostRedactor above stand between every measured value and every Logf,
// Errorf and Fatalf, so the failure path is covered too. That used to be a
// comment asking the reader to redact by hand.
//
// WHAT THIS PROBE VERIFIES DEPENDS ON THE HOST, and it now says so: the
// "pid after the LAST hyphen" rule is only exercised by a hostname that
// contains a hyphen. The nested "last-hyphen rule" subtest checks that against
// the name it actually observes and SKIPS with a reason when the name cannot
// discriminate. See the section comment above lockSplit.
func TestRealSPASingletonLockFormat(t *testing.T) {
	requireRealSPA(t)

	profile, overridden, err := observationProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	if overridden {
		if err := requireExistingProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	before := 0
	if _, err := os.Stat(profile); err == nil {
		before = countProfileFiles(t, profile)
	}
	t.Logf("BASELINE profile_files=%d singleton_lock_present=%v",
		before, lockPresent(t, profile))

	// The subtest exists for its CLEANUP: the clean stop is registered inside
	// launchObservationProfile, so the only way to observe the profile AFTER
	// the browser is down is to let a nested test end first.
	t.Run("live", func(t *testing.T) {
		observeLiveSingletonLock(t, profile)
	})

	after := countProfileFiles(t, profile)
	// REPORTED, not asserted, and the difference was measured rather than
	// assumed. MEASURED: "the profile never shrinks" is the CAP-05 observable
	// across sleep/wake cycles, and at the granularity of ONE boot it is false
	// — a boot of the lab profile went 457 -> 456. HYPOTHESIS, not measured on
	// that same boot: Chromium ROTATES Default/Sessions/Session_* and Tabs_*,
	// dropping the previous pair and writing a new one, so the count moves by
	// whatever the rotation happens not to balance. The rotation is real, but
	// the listing diff that showed it was taken around a LATER boot whose delta
	// was 0; the -1 boot was never diffed. See HOUSEKEEP H10. Either way, a run
	// that asserted non-decrease here would fail on behaviour already observed
	// to be healthy and teach the next reader to delete the check.
	t.Logf("HYGIENE profile_files before=%d after=%d delta=%+d (Default/Sessions/* rotates; see HOUSEKEEP H10)",
		before, after, after-before)
	// Re-confirms M3, and it is the reason the naive method fails: after a
	// clean stop the lock is not there to be looked at.
	if lockPresent(t, profile) {
		t.Errorf("%s survived the clean stop; the next boot would have to reclaim it", singletonLockName)
	}
	t.Logf("AFTER CLEAN STOP singleton_lock_present=%v", lockPresent(t, profile))
}

func lockPresent(t *testing.T, profile string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(profile, singletonLockName))
	return err == nil
}

// observeLiveSingletonLock is the measurement proper: raw form first, then the
// production parser's reading of that same raw form.
func observeLiveSingletonLock(t *testing.T, profile string) {
	t.Helper()
	runner := engine.NewRunner()
	browser, launched := launchObservationProfile(t, runner)
	if launched != profile {
		t.Fatalf("the probe measured %s but the launcher opened %s", profile, launched)
	}

	lock := filepath.Join(profile, singletonLockName)
	info, err := os.Lstat(lock)
	if err != nil {
		t.Fatalf("%s while the browser is ALIVE: %v — either the browser is not "+
			"holding the profile, or Chromium no longer writes this file at all, "+
			"and the second case makes the whole reclaim guard dead code",
			singletonLockName, err)
	}
	target, err := os.Readlink(lock)
	if err != nil {
		t.Fatalf("%s exists but is NOT a symlink (%v): mode=%v. readLockHolder "+
			"reads it with Readlink and would report it unreadable — DIVERGENCE",
			singletonLockName, err, info.Mode())
	}

	// Everything printed from here on passes through the redactor or through
	// describeLockTarget. Nothing below may format `target`, os.Hostname(), a
	// ReclaimResult field or an engine error directly.
	red := newHostRedactor(hostSecretsOf(target)...)

	t.Logf("MEASURED lstat: mode=%v symlink=%v", info.Mode(), info.Mode()&os.ModeSymlink != 0)
	t.Logf("MEASURED readlink target=%s", describeLockTarget(target))
	t.Logf("MEASURED os.Hostname() matches the lock's host half=%v launched_pid=%d",
		hostnameOrEmpty() == hostHalfOf(target), browser.PID())

	// What readLockHolder makes of that exact string, read through production.
	//
	// The typed error is the assertion, not a sentence: ErrProfileHeldByLiveBrowser
	// is reachable ONLY after the target parsed into a host that equals this
	// one AND a pid that is running. A format that did not parse would instead
	// come back with a nil error and three deleted files.
	live, liveErr := engine.ReclaimProfile(replicaOfLock(t, target), engine.ReclaimOptions{
		ProcessAlive: engine.ProcessAlive,
	})
	t.Logf("PRODUCTION PARSER (real probe): lock_holder=%s reason=%s removed=%v err=%s",
		describeLockTarget(live.LockHolder), red.scrub(live.Reason), live.Removed,
		red.scrub(liveErr))

	if !errors.Is(liveErr, engine.ErrProfileHeldByLiveBrowser) {
		t.Fatalf("DIVERGENCE: readLockHolder did not resolve %s to a live holder on this "+
			"host; got reason=%s removed=%v err=%s. The reclaim would delete without "+
			"proof — HOUSEKEEP H4", describeLockTarget(target), red.scrub(live.Reason),
			live.Removed, red.scrub(liveErr))
	}
	if len(live.Removed) != 0 {
		t.Fatalf("the reclaim refused AND deleted %v; a refusal must touch nothing", live.Removed)
	}
	if live.LockHolder != target {
		t.Errorf("the reclaim reported holder %s for a lock whose target is %s",
			describeLockTarget(live.LockHolder), describeLockTarget(target))
	}

	// Does the name of THIS machine let the assertion above fail? Answered
	// against the observed value, never assumed. The subtest carries the claim
	// so that a host which cannot discriminate produces a named SKIP instead of
	// a silent pass.
	t.Logf("LAST-HYPHEN RULE exercised_by_this_target=%v",
		lockTargetDiscriminatesLastHyphen(target))
	t.Run("last-hyphen rule", func(t *testing.T) {
		first := splitLockTargetAt(target, strings.Index(target, "-"))
		last := splitLockTargetAt(target, strings.LastIndex(target, "-"))
		if first == last {
			t.Skipf("NOT VERIFIED ON THIS HOST, and saying so is the point. The measured "+
				"target is %s: its first and last hyphen are the same hyphen (or it has "+
				"none), so both rules read it identically — a strings.LastIndex -> "+
				"strings.Index mutation in engine/profile.go would pass this probe "+
				"unnoticed here, and a silent pass would overstate what this run "+
				"verified. The RULE is not lost: engine.TestLockHolderSplitsOnTheLastHyphen "+
				"(engine/profile_test.go) locks it with a controlled double, ungated, on "+
				"every `make check` and on any host. To exercise it against a REAL "+
				"target, run this probe on a machine whose hostname contains a hyphen.",
				describeLockTarget(target))
		}
		t.Logf("DISCRIMINATING: first-hyphen split reads this target as %s, last-hyphen "+
			"as %s — the LastIndex -> Index mutation could not survive the assertion above",
			describeSplit(first), describeSplit(last))
	})

	// The second reading exists to print the parse in full: with a probe that
	// says the holder is gone, the reason names the pid readLockHolder pulled
	// out of the measured target, and the delete path is reached — which it can
	// only be after the host matched.
	gone, goneErr := engine.ReclaimProfile(replicaOfLock(t, target), engine.ReclaimOptions{
		ProcessAlive: func(int) bool { return false },
	})
	t.Logf("PRODUCTION PARSER (holder declared dead): reason=%s removed=%v err=%s",
		red.scrub(gone.Reason), gone.Removed, red.scrub(goneErr))
	if goneErr != nil {
		t.Fatalf("a lock whose holder is gone must be reclaimable: %v", goneErr)
	}
	if len(gone.Removed) != 3 {
		t.Fatalf("removed %v, want all three; the reclaim did not reach the delete path",
			gone.Removed)
	}
}

func hostnameOrEmpty() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
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

// --- LOOP 04.3E · M7: the leg that should WORSEN ---------------------------
//
// M6 measured how long a HEALTHY boot keeps the socket negotiating, and got
// 0.27-0.50s across six samples. M6.4 says in writing why that is not yet an
// answer: samples 2-5 land within 20 ms of each other, which is ONE CONDITION
// SAMPLED FIVE TIMES rather than the distribution of the phenomenon. What
// decides a liveness cut is the UPPER TAIL — which slow boot becomes a false
// positive — and a comfortable half of a distribution cannot produce it.
//
// The project rule this closes is the second of "medir antes de projetar":
// measure the scenario where the mechanism COSTS, not only the one where it
// pays. A distribution sampled only under favourable conditions measured the
// hypothesis, not the mechanism.
//
// There is direct evidence the tail exists: #pane-side arrived at 7.40s in one
// M3.3 run and at 15.61s in another, SAME profile. If the negotiating window
// scales with boot slowness the way the panel does, half a second becomes
// seconds under contention.
//
// The two hypotheses, and what would decide between them, WRITTEN BEFORE THE
// RUN (EVIDENCIA-SPA.md M7.1 holds the same three lines):
//
//   - CONFIRMS "the window scales with boot slowness": the stressed arms'
//     maximum rises materially above the unstressed arms' maximum, and rises
//     WITH the level of degradation rather than at random.
//   - WEAKENS it: the stressed maximum rises only to the same order as the
//     unstressed one, or rises without any relation to the level.
//   - FALSIFIES it: the stressed arms' maximum stays at or below the
//     unstressed arms' maximum while the contention is demonstrably applied —
//     that is, with the boot itself measurably slower (pane, meReady) and the
//     dilation and RTT figures showing real starvation. A window that will not
//     move while everything around it moves is a window bounded by something
//     other than this machine, which is the ALTERNATIVE hypothesis: a
//     server-side handshake.
//
// This probe DOES NOT choose the cut. It produces one of the two legs the
// choice needs.

const (
	// stressTick is M7's sampling interval, and it is FOUR TIMES finer than
	// M6's 250 ms on purpose: F-20 says the instrument does not resolve the
	// window it was built to measure, and M6.5 says the 0.27/0.50s figures are
	// one or two ticks of the coarse sampler.
	//
	// It does NOT re-baseline M6. readinessTick is untouched, so the M6 runs
	// stay exactly what they were; the bridge between the two measurements is
	// M7's own unstressed arm, which is interleaved with the stressed ones and
	// samples the same phenomenon M6 sampled.
	//
	// The tick is a FLOOR on the spacing, never a promise about it: each sample
	// costs a round trip, and under CPU contention the round trip is what sets
	// the real spacing. That is why every sample carries its RTT and why the
	// report prints the OBSERVED spacing per arm rather than this constant.
	stressTick = 50 * time.Millisecond
	// stressRounds is how many times the whole condition set is run. Three is
	// the project minimum for "one sample cannot pass for a trend", and every
	// boot of a paired profile costs something (phase 4C), so it is also the
	// maximum this loop is willing to spend.
	stressRounds = 3
	// spinWork is the size of the calibration loop. Fixed, so that the only
	// thing that can change its duration is how much CPU the process got.
	spinWork = 40_000_000
)

// spinSink exists so the compiler cannot delete the calibration loop. A
// benchmark that got optimised away would report a dilation of 1.00 under any
// load whatsoever — the instrument answering by construction.
var spinSink int

// spinDuration times a fixed amount of pure CPU work.
//
// It measures the contention as THIS PROCESS feels it, which is a proxy for how
// the browser feels it and not the same thing: they are different processes on
// the same cores. The in-band figure is the probe RTT, and both are reported.
func spinDuration() time.Duration {
	start := time.Now()
	x := 0
	for i := 0; i < spinWork; i++ {
		x += i % 7
	}
	spinSink = x
	return time.Since(start)
}

// cpuLoad is REAL contention: separate OS processes spinning on the same cores
// the browser runs on.
//
// Real processes rather than goroutines, because goroutines would compete for
// this process's own GOMAXPROCS share first and could starve the sampler while
// leaving the browser comparatively alone — a caricature that does not actually
// starve the thing being measured measures nothing (packet DO#3).
type cpuLoad struct{ procs []*exec.Cmd }

// burnerCommand is a pure busy loop in the system shell. It takes no input of
// any kind, so there is nothing here that could carry anything from the page.
const burnerCommand = "while :; do :; done"

func startCPULoad(t *testing.T, n int) *cpuLoad {
	t.Helper()
	load := &cpuLoad{}
	for i := 0; i < n; i++ {
		cmd := exec.Command("/bin/sh", "-c", burnerCommand)
		if err := cmd.Start(); err != nil {
			load.stop(t)
			t.Fatalf("starting CPU burner %d/%d: %v", i+1, n, err)
		}
		load.procs = append(load.procs, cmd)
	}
	return load
}

// stop kills the burners and REAPS them. Without the wait they linger as
// zombies and the next condition's load figure would include them.
func (l *cpuLoad) stop(t *testing.T) {
	t.Helper()
	for _, cmd := range l.procs {
		if cmd.Process == nil {
			continue
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	l.procs = nil
}

// loadAverage reads the kernel's own 1-minute figure.
//
// REPORTED, never asserted: it is a one-minute exponential average, so at the
// scale of a boot it lags badly and would answer a question about the last
// minute when asked about the last ten seconds. It is here because it is the
// system's own number and costs nothing; the figures that actually quantify
// the contention are the spin dilation and the probe RTT.
func loadAverage() string {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(out))
}

// stressCondition is one arm of the measurement: an amount of CPU contention
// and an amount of network degradation.
//
// The two axes are varied ONE AT A TIME rather than crossed. A full cross would
// be 9 conditions and 27 boots of a paired profile, and phase 4C measured that
// repeated boots are how a paired session degrades; one-at-a-time also
// attributes any movement to the axis that moved.
type stressCondition struct {
	name    string
	burners int
	net     engine.NetworkDegradation
}

func (c stressCondition) degraded() bool { return c.net != engine.NetworkDegradation{} }

// stressConditions is the arm list, sized against the host's real core count.
//
// Three levels per axis (packet DO#5), because two runs of one condition is how
// M6 ended up with five samples of a single state. The network levels are
// chosen to DEGRADE rather than to break: the boot still has to complete for
// there to be a negotiating window to time at all, and the Navigate budget is
// 30s. A level that fails to boot is reported as such rather than dropped.
func stressConditions(cores int) []stressCondition {
	const (
		kb = 1 << 10
		mb = 1 << 20
	)
	return []stressCondition{
		{name: "unstressed"},
		{name: "cpu-0.5x", burners: cores / 2},
		{name: "cpu-1x", burners: cores},
		{name: "cpu-2x", burners: cores * 2},
		{name: "net-mild", net: engine.NetworkDegradation{
			Latency: 150 * time.Millisecond, DownloadBytesPerSecond: 1.5 * mb,
			UploadBytesPerSecond: 512 * kb}},
		{name: "net-moderate", net: engine.NetworkDegradation{
			Latency: 400 * time.Millisecond, DownloadBytesPerSecond: 500 * kb,
			UploadBytesPerSecond: 200 * kb}},
		{name: "net-heavy", net: engine.NetworkDegradation{
			Latency: 900 * time.Millisecond, DownloadBytesPerSecond: 200 * kb,
			UploadBytesPerSecond: 100 * kb}},
	}
}

// bootResult is one boot, reduced to the numbers the report is made of.
type bootResult struct {
	round     int
	condition string
	burners   int
	// openingM6 is connected - meReady, which is EXACTLY what M6 published as
	// its "window in OPENING". Kept because comparability with M6 is the point
	// of reusing this instrument.
	openingM6 time.Duration
	// openingDirect is first-seen-OPENING to CONNECTED — the state itself
	// rather than an anchor chosen around it. The two are reported side by side
	// because M6 never distinguished them.
	openingDirect            time.Duration
	pane, meReady, connected time.Duration
	// spanMax and spanMedian are the OBSERVED spacing between samples: the
	// instrument's real resolution for THIS boot, which under contention is not
	// stressTick.
	spanMax, spanMedian  time.Duration
	rttMax, rttMedian    time.Duration
	spinDilation         float64
	offlineEvents        int
	onlineEvents         int
	navigatorEverOffline bool
	samples              int
	loadAvg              string
	failure              string
	// lockAfterStop is the hygiene observable of invariant 2, read by the
	// PARENT after the boot's subtest ended and the clean stop ran.
	//
	// The profile's FILE COUNT is deliberately not used for hygiene here: H10
	// measured "the profile never shrinks" FALSE at the granularity of one boot
	// (457 -> 456 on a clean stop), so a count would be a false observable.
	lockAfterStop bool
}

// TestRealSPABootUnderStress is M7.
//
// It boots the paired profile repeatedly, INTERLEAVING stressed and unstressed
// conditions inside ONE execution on ONE machine. Comparing separate executions
// would also measure the state of the machine, which is the project rule and
// not a preference.
//
// Read-only throughout: it opens the SPA, samples booleans and enum names, and
// stops through the protocol. Nothing is sent and no message, name or number is
// read.
func TestRealSPABootUnderStress(t *testing.T) {
	requireRealSPA(t)

	cores := runtime.NumCPU()
	conditions := stressConditions(cores)

	// The unstressed reference for the dilation figure, taken before anything
	// is loaded. Without it "the spin took 210 ms" is a number with no meaning.
	reference := spinDuration()
	t.Logf("HOST cores=%d arch=%s go=%s tick=%v rounds=%d reference_spin=%v load=%s",
		cores, runtime.GOARCH, runtime.Version(), stressTick, stressRounds,
		reference.Round(time.Millisecond), loadAverage())

	profile, _, err := observationProfileDir()
	if err != nil {
		t.Fatal(err)
	}

	// Which boot carries the positive control, decided BEFORE the loop from the
	// same rotation the loop uses, so the two cannot drift apart.
	final := rotatedConditions(conditions, stressRounds)
	lastBoot := fmt.Sprintf("r%d/%s", stressRounds, final[len(final)-1].name)

	var results []bootResult
	var order []string
	for round := 1; round <= stressRounds; round++ {
		for _, c := range rotatedConditions(conditions, round) {
			label := fmt.Sprintf("r%d/%s", round, c.name)
			order = append(order, label)
			var got bootResult
			// A subtest per boot, for its CLEANUP: the clean stop is registered
			// inside launchObservationProfile, so the only way to observe the
			// profile with the browser DOWN is to let a nested test end.
			t.Run(label, func(t *testing.T) {
				got = oneStressedBoot(t, c, round, reference, label == lastBoot)
			})
			got.lockAfterStop = lockPresent(t, profile)
			results = append(results, got)
		}
	}

	t.Logf("INTERLEAVING ORDER (%d boots, one execution, one machine): %s",
		len(order), strings.Join(order, " -> "))
	reportStressResults(t, results, conditions)
}

// rotatedConditions keeps the unstressed arm first — it is the bridge to M6 and
// wants the same place in every round — and rotates the rest.
//
// The rotation is what makes the interleaving worth anything: if every round
// ran the arms in the same order, a machine that warms up or drifts over the
// execution would add the same offset to the same arm three times, and that
// offset would be read as a property of the condition.
func rotatedConditions(all []stressCondition, round int) []stressCondition {
	if len(all) < 2 {
		return all
	}
	head, tail := all[:1], all[1:]
	shift := (round - 1) % len(tail)
	out := append([]stressCondition{}, head...)
	out = append(out, tail[shift:]...)
	return append(out, tail[:shift]...)
}

// oneStressedBoot applies the condition, boots, and times the window.
//
// ORDER IS LOAD-BEARING. The contention and the degradation are applied BEFORE
// the navigation, because the question is about a SLOW BOOT and a degradation
// switched on after the bundle has landed would be a degradation of nothing.
func oneStressedBoot(t *testing.T, c stressCondition, round int, reference time.Duration,
	withPositiveControl bool) bootResult {
	t.Helper()

	out := bootResult{round: round, condition: c.name, burners: c.burners}

	if c.burners > 0 {
		load := startCPULoad(t, c.burners)
		defer load.stop(t)
	}
	// Measured with the burners already spinning and the browser not yet up, so
	// it reports the contention this arm ASKED for. The report also carries the
	// probe RTT, which is the same question asked of the renderer while the
	// boot is actually happening.
	out.spinDilation = float64(spinDuration()) / float64(reference)
	out.loadAvg = loadAverage()

	runner := engine.NewRunner()
	browser, _ := launchObservationProfile(t, runner)
	_ = browser

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	t.Cleanup(tab.Close)

	applyDegradation(t, runner, tab, c)

	if err := tab.Navigate(runner, realSPAURL, "stress/navigate"); err != nil {
		// A degradation severe enough to blow the Navigate budget is a REPORTED
		// outcome of that level, not a crash: the honest reading is "this level
		// was not measured", and deleting the arm would quietly narrow the very
		// tail this loop exists to sample.
		out.failure = fmt.Sprintf("navigation did not complete: %v", err)
		t.Logf("NOT MEASURED at this level: %s", out.failure)
		return out
	}

	// As early as the page allows. It cannot cover the milliseconds between the
	// document being created and this call, which is why the STRUCTURAL
	// argument is reported beside the count: overrideNetworkState is never sent
	// on this path, so there is no `offline` event for the browser to fire.
	// navigator.onLine is sampled independently in every readiness sample and
	// is the second, redundant witness.
	installNetEventSentinel(t, runner, tab)

	samples, marks := sampleReadiness(t, runner, tab, readinessScript(spa.RequiredAtStartup),
		len(spa.RequiredAtStartup), stressTick)

	events := readNetEvents(t, runner, tab)
	out.absorb(samples, marks, events)

	out.report(t)
	out.assertConfoundAbsent(t)
	if withPositiveControl {
		provePageCountsOfflineEvents(t, runner, tab, out.offlineEvents)
	}
	return out
}

// provePageCountsOfflineEvents is the POSITIVE side of the confound control,
// and without it every zero above proves nothing: a listener that can never
// count anything reads zero under all conditions, including the conditions it
// was installed to detect.
//
// It runs on the LAST boot only, and only AFTER that boot's window has been
// timed and its counter read, so it cannot contaminate a measurement. Folding
// it into an existing boot rather than giving it its own is deliberate: it
// costs the paired profile nothing, and it is a STRONGER control than a
// separate run because it exercises the same listener, in the same page
// instance, that had just read zero.
//
// SetNetworkOffline is the announced cut — the one that fires the event. That
// is the whole point here, and it is the reason this call must never appear on
// a measuring path.
func provePageCountsOfflineEvents(t *testing.T, runner *engine.Runner, tab *engine.Tab, before int) {
	t.Helper()
	t.Logf("POSITIVE CONTROL: the same listener that just read offline=%d is now "+
		"given a genuine event", before)

	if err := tab.SetNetworkOffline(runner, true, "control/announce"); err != nil {
		t.Fatalf("the positive control could not fire the event: %v", err)
	}
	defer func() {
		if err := tab.SetNetworkOffline(runner, false, "control/restore"); err != nil {
			t.Logf("restoring after the positive control: %v", err)
		}
	}()
	// The event is dispatched by the browser, not by us, so it needs a moment
	// to reach the page's listener. The clock is on the GO side (invariant 6):
	// no wait in this module keeps its clock in the page.
	time.Sleep(2 * time.Second)

	after := readNetEvents(t, runner, tab)
	if after.Offline <= before {
		t.Errorf("the `offline` listener did NOT count a genuine event (%d -> %d). "+
			"Every zero this run reported is therefore unproven: a counter that "+
			"cannot count is not evidence of absence",
			before, after.Offline)
		return
	}
	t.Logf("POSITIVE CONTROL PASSED: offline %d -> %d, online %d. The counter "+
		"counts, so the zeros above are measurements and not silence.",
		before, after.Offline, after.Online)
}

// applyDegradation switches the network condition on BEFORE the navigation,
// and registers its own undo.
//
// Order is load-bearing: the question is how long a SLOW BOOT negotiates, and a
// degradation switched on after the bundle has landed would be a degradation of
// nothing.
func applyDegradation(t *testing.T, runner *engine.Runner, tab *engine.Tab, c stressCondition) {
	t.Helper()
	if !c.degraded() {
		return
	}
	if err := tab.SetNetworkDegraded(runner, c.net, "stress/degrade"); err != nil {
		t.Fatalf("SetNetworkDegraded: %v", err)
	}
	t.Cleanup(func() {
		if err := tab.ClearNetworkConditions(runner, "stress/restore"); err != nil {
			t.Logf("clearing the network conditions: %v", err)
		}
	})
	t.Logf("DEGRADED latency=%v down=%.0fB/s up=%.0fB/s (offline=false, structurally: "+
		"NetworkDegradation cannot express an outage — engine/network_test.go)",
		c.net.Latency, c.net.DownloadBytesPerSecond, c.net.UploadBytesPerSecond)
}

// absorb turns one boot's samples and marks into the numbers the report is made
// of.
//
// BOTH window anchors are computed. openingM6 is connected-meReady, which is
// exactly what M6 published, and openingDirect is first-seen-OPENING to
// connected, which is the state itself. M6 never distinguished the two, so
// where they disagree the disagreement is a finding rather than a detail.
func (r *bootResult) absorb(samples []readinessSample, marks readinessMarks, events netEventCounts) {
	r.offlineEvents, r.onlineEvents = events.Offline, events.Online
	r.navigatorEverOffline = anySample(samples, func(s readinessSample) bool {
		return !s.NavigatorOnline
	})
	r.samples = len(samples)
	r.pane, r.meReady, r.connected = marks.pane, marks.meReady, marks.connected
	if marks.connected > 0 && marks.meReady > 0 {
		r.openingM6 = marks.connected - marks.meReady
	}
	if marks.connected > 0 && marks.openingFirst > 0 {
		r.openingDirect = marks.connected - marks.openingFirst
	}
	r.spanMax, r.spanMedian = spacing(samples)
	r.rttMax, r.rttMedian = roundTrips(samples)
	if marks.connected == 0 {
		r.failure = "the socket never reported " + socketStateConnected +
			" within the budget"
	}
}

// report prints one boot. Split out of oneStressedBoot so the boot function
// stays under the complexity gate; the lines it prints are unchanged.
func (r bootResult) report(t *testing.T) {
	t.Helper()
	t.Logf("BOOT %s round=%d burners=%d dilation=%.2fx load=%q samples=%d",
		r.condition, r.round, r.burners, r.spinDilation, r.loadAvg, r.samples)
	t.Logf("  pane=%s meReady=%s connected=%s | openingM6=%s openingDirect=%s",
		markString(r.pane), markString(r.meReady), markString(r.connected),
		markString(r.openingM6), markString(r.openingDirect))
	t.Logf("  observed spacing median=%v max=%v · probe RTT median=%v max=%v "+
		"(the REAL resolution of this arm, not the %v tick)",
		r.spanMedian.Round(time.Millisecond), r.spanMax.Round(time.Millisecond),
		r.rttMedian.Round(time.Millisecond), r.rttMax.Round(time.Millisecond), stressTick)
	t.Logf("  CONFOUND offline_events=%d online_events=%d navigator_ever_offline=%v",
		r.offlineEvents, r.onlineEvents, r.navigatorEverOffline)
}

// assertConfoundAbsent is the ONLY assertion this probe makes, and it is about
// the CONFOUND rather than about the phenomenon.
//
// A degraded run that fired an `offline` event would be measuring the SPA's
// reaction to an ANNOUNCEMENT (M5.4: ~3s) instead of to a slow network, so its
// number would belong to a different measurement. Nothing about the window
// itself is asserted — locking a number this loop exists to DISCOVER would be
// speculation wearing the costume of a test.
func (r bootResult) assertConfoundAbsent(t *testing.T) {
	t.Helper()
	if r.offlineEvents != 0 {
		t.Errorf("the page received %d `offline` event(s): this run measured the "+
			"SPA reacting to an ANNOUNCEMENT, not to a slow network (M5.4), and its "+
			"window is void", r.offlineEvents)
	}
	if r.navigatorEverOffline {
		t.Error("navigator.onLine went false during the boot: the degradation " +
			"announced itself, which is the 04.3A confound")
	}
}

// spacing is the OBSERVED interval between consecutive samples: median and max.
//
// It is the number that says what this instrument actually resolved on this
// boot. F-20 is the finding that a 250 ms tick cannot resolve a ~500 ms window;
// printing a 50 ms constant while the round trips cost 400 ms would be the same
// finding, one layer up and self-inflicted.
func spacing(samples []readinessSample) (max, median time.Duration) {
	if len(samples) < 2 {
		return 0, 0
	}
	gaps := make([]time.Duration, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		gaps = append(gaps, samples[i].At-samples[i-1].At)
	}
	return maxOf(gaps), medianOf(gaps)
}

func roundTrips(samples []readinessSample) (max, median time.Duration) {
	if len(samples) == 0 {
		return 0, 0
	}
	rtts := make([]time.Duration, 0, len(samples))
	for _, s := range samples {
		rtts = append(rtts, s.RTT)
	}
	return maxOf(rtts), medianOf(rtts)
}

func maxOf(ds []time.Duration) time.Duration {
	var out time.Duration
	for _, d := range ds {
		if d > out {
			out = d
		}
	}
	return out
}

// medianOf sorts a COPY: sorting the caller's slice would reorder a timeline
// whose order is the measurement.
func medianOf(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	cp := append([]time.Duration{}, ds...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

// percentileOf is the NEAREST-RANK percentile, which is the only honest choice
// for the sample sizes here: interpolating between three points invents a value
// that was never observed. With n=3 the p95 IS the maximum, and the report says
// so rather than dressing it up.
func percentileOf(ds []time.Duration, p float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	cp := append([]time.Duration{}, ds...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	rank := int(math.Ceil(p / 100 * float64(len(cp))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(cp) {
		rank = len(cp)
	}
	return cp[rank-1]
}

// reportStressResults prints the distribution, per arm and pooled.
//
// The MAXIMUM comes first, everywhere. The median of a boot-time distribution
// is the number that makes a cut look easy; the tail is the number that decides
// which slow boot becomes a false positive, and it is the only one this loop
// was dispatched to produce.
func reportStressResults(t *testing.T, results []bootResult, conditions []stressCondition) {
	t.Helper()

	t.Log("")
	t.Log("=== M7 · time in OPENING, by arm. MAX first: it is the number that decides. ===")
	t.Log("arm            n  max(M6-anchor)  p95     median  | max(direct)  worst spacing  dilation")

	byArm := map[string][]bootResult{}
	for _, r := range results {
		byArm[r.condition] = append(byArm[r.condition], r)
	}

	var stressedAll, unstressedAll []time.Duration
	for _, c := range conditions {
		row, ok := summariseArm(byArm[c.name])
		if !ok {
			t.Logf("%-14s 0  NOT MEASURED — every boot of this arm failed to complete", c.name)
			continue
		}
		if c.name == conditions[0].name {
			unstressedAll = append(unstressedAll, row.m6...)
		} else {
			stressedAll = append(stressedAll, row.m6...)
		}
		t.Logf("%-14s %d  %-15s %-7s %-7s | %-12s %-14s %.2fx",
			c.name, len(row.m6),
			fmtDur(maxOf(row.m6)), fmtDur(percentileOf(row.m6, 95)), fmtDur(medianOf(row.m6)),
			fmtDur(maxOf(row.direct)), fmtDur(maxOf(row.spans)), row.dilation)
	}

	t.Log("")
	t.Logf("POOLED unstressed n=%d max=%s p95=%s", len(unstressedAll),
		fmtDur(maxOf(unstressedAll)), fmtDur(percentileOf(unstressedAll, 95)))
	t.Logf("POOLED stressed   n=%d max=%s p95=%s", len(stressedAll),
		fmtDur(maxOf(stressedAll)), fmtDur(percentileOf(stressedAll, 95)))

	// The question the packet asks, answered from the numbers rather than from
	// the reader's impression of them. INCONCLUSIVE is an available verdict and
	// is not a failure of the run.
	worst := maxOf(append(append([]time.Duration{}, stressedAll...), unstressedAll...))
	t.Logf("DETECTION FLOOR: the worst window measured here is %s against the %s "+
		"M5 measured for a socket to LEAVE %s with the server lost. Ratio %.1fx "+
		"— and the ratio is CONTEXT, not a bound: the detection latency ADDS to a "+
		"cut C (total = detection + C) instead of capping it, so what this run "+
		"gives about C is the lower bound alone.",
		fmtDur(worst), fmtDur(detectionFloor), socketStateConnected,
		float64(detectionFloor)/float64(worst))

	reportHygieneAndConfound(t, results)
}

// reportHygieneAndConfound prints the two per-run observables that are not
// about the window: whether every boot let go of the profile, and whether any
// boot saw the confound.
//
// Split out of reportStressResults for the complexity gate; the lines are
// unchanged.
func reportHygieneAndConfound(t *testing.T, results []bootResult) {
	t.Helper()

	var dirty []string
	attempted := 0
	for _, r := range results {
		if r.lockAfterStop {
			dirty = append(dirty, fmt.Sprintf("r%d/%s", r.round, r.condition))
		}
		// A subtest the -run filter skipped leaves a zero entry. Counting those
		// as boots would report hygiene for runs that never happened.
		if r.samples > 0 || r.failure != "" {
			attempted++
		}
	}
	if len(dirty) > 0 {
		t.Errorf("HYGIENE %s survived the clean stop on: %s", singletonLockName,
			strings.Join(dirty, ", "))
	} else {
		t.Logf("HYGIENE %d/%d boots left no %s behind after the clean stop; "+
			"stopped_via is logged per boot above",
			attempted, attempted, singletonLockName)
	}

	totalOffline := 0
	for _, r := range results {
		totalOffline += r.offlineEvents
	}
	t.Logf("CONFOUND across every boot: offline events = %d (structurally "+
		"impossible on this path — overrideNetworkState is never sent — AND "+
		"counted zero; the counter's positive control ran in THIS run, on the "+
		"last boot, against the same counter on the same page instance that had "+
		"just reported zero, and asserts after > before — see provePageCounts"+
		"OfflineEvents above and EVIDENCIA-SPA.md M7.7)", totalOffline)
}

// armSummary is one arm reduced to the vectors the report prints.
type armSummary struct {
	m6, direct, spans []time.Duration
	dilation          float64
}

// summariseArm drops the boots that produced no window, and reports whether any
// survived. A boot that failed to complete is NOT a zero-length window: folding
// it in as one would pull every statistic of that arm downwards and make a
// broken level look like a fast one.
func summariseArm(runs []bootResult) (armSummary, bool) {
	var out armSummary
	var dilation float64
	for _, r := range runs {
		if r.failure != "" || r.openingM6 == 0 {
			continue
		}
		out.m6 = append(out.m6, r.openingM6)
		out.direct = append(out.direct, r.openingDirect)
		out.spans = append(out.spans, r.spanMax)
		dilation += r.spinDilation
	}
	if len(out.m6) == 0 {
		return armSummary{}, false
	}
	out.dilation = dilation / float64(len(out.m6))
	return out, true
}

// detectionFloor is what M5 measured for the socket to leave CONNECTED with the
// server lost: 33.2-34.2s across three runs, of which the FASTEST is the one
// quoted here.
//
// It is DETECTION LATENCY — the instant of LEAVING CONNECTED — and it therefore
// ADDS to a cut instead of bounding it. For a cut C over time-in-OPENING the
// total time to declare a session dead would be 33.2s + C. What this run
// delivers about C is the LOWER bound and only it: C has to sit above the
// slowest healthy boot. The UPPER bound is how long the socket STAYS in OPENING
// under a cut, which is N2b and is not measured here.
//
// So the ratio printed against this constant is budgetary context — how much
// room there is to choose C next to a detection cost that is already paid — and
// is NOT an upper bound on the cut. The two quantities sit on different axes.
const detectionFloor = 33200 * time.Millisecond

// fmtDur prints a duration in seconds, or says the value is absent. Zero must
// never print as "0.00s": a window that was never measured is not a window of
// no length.
func fmtDur(d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

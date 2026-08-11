package engine

// The launch flag set (ADR-0006 D2).
//
// The flag list IS the memory budget. The spike measured a 790 MB -> 592 MB
// swing from three flags alone, which means a flag inherited from a library
// default is a memory decision nobody made. So every flag is written here, with
// its reason, and no other package may add one.
//
// Study origin: scripts/chromium-study/main.go, CanonicalBrowserProfileV1 —
// the exact list every phase of the study measured against. Changing an entry
// invalidates the measurement that justified it.

import (
	"fmt"
	"strings"
)

// Flag names, as constants: a literal repeated in two places is the same
// decision waiting to diverge (ADR-0004).
const (
	flagHeadless       = "--headless=new"
	flagDisableFeature = "--disable-features="
	flagWindowSize     = "--window-size="
	flagUserAgent      = "--user-agent="
	flagUserDataDir    = "--user-data-dir="
	flagRemotePort     = "--remote-debugging-port="
	flagRemoteAddress  = "--remote-debugging-address="
)

// DefaultWindowSize is what every phase of the study measured against.
//
// It is NOT known to be the right size for the target. Phase 5 measured the
// WhatsApp sidebar rendering at (-165,-42) with this window, entirely outside
// the visible area, and widening to 1600x1057 moved it to (-229,-122) rather
// than fixing it. Phase 6 later found a candidate cause that has nothing to do
// with geometry — a renderer that had stopped executing JavaScript — so the
// question is open, not settled. Carrying the measured default and saying so is
// more honest than shipping a number that looks chosen.
const DefaultWindowSize = "1280,800"

// DefaultRemoteAddress binds the DevTools endpoint to loopback only.
//
// DELIBERATE DIVERGENCE from the study, which used 0.0.0.0 because it ran
// inside a throwaway container. An open CDP port is arbitrary code execution in
// the browser that holds a paired WhatsApp session: anyone who reaches it can
// read every message and send as the owner. The study's value is its memory and
// timing measurements, and the bind address changes neither.
const DefaultRemoteAddress = "127.0.0.1"

// canonicalProfileV1 is the launch policy, in the order the study ran it.
//
// Rationale by group:
//   - headless=new: the supported headless implementation. Old headless is a
//     different browser with different memory behaviour, not a compatibility
//     mode.
//   - no-sandbox: required to run as a non-root-capable process in a plain
//     container. It is a SECURITY REDUCTION, stated here rather than inherited
//     from a library that detects containers and flips it silently.
//   - disable-dev-shm-usage: containers default to a 64 MB /dev/shm, and
//     without this Chromium crashes under load.
//   - the site isolation pair: the single largest memory lever the study found.
//     Set explicitly so it is never accidentally dropped — see AppendFlags for
//     the way it CAN be dropped by accident.
var canonicalProfileV1 = []string{
	flagHeadless,
	"--no-sandbox",
	"--disable-dev-shm-usage",
	"--disable-gpu",
	"--no-first-run",
	"--no-default-browser-check",
	"--disable-background-networking",
	"--disable-background-timer-throttling",
	"--disable-backgrounding-occluded-windows",
	"--disable-renderer-backgrounding",
	"--disable-breakpad",
	"--disable-client-side-phishing-detection",
	"--disable-default-apps",
	"--disable-extensions",
	"--disable-component-extensions-with-background-pages",
	"--disable-hang-monitor",
	"--disable-ipc-flooding-protection",
	"--disable-popup-blocking",
	"--disable-prompt-on-repost",
	"--disable-sync",
	"--metrics-recording-only",
	flagDisableFeature + "site-per-process,Translate,TranslateUI,BlinkGenPropertyTrees," +
		"AcceptCHFrame,MediaRouter,OptimizationHints",
	"--disable-site-isolation-trials",
	"--force-color-profile=srgb",
	flagWindowSize + DefaultWindowSize,
	"--lang=en-US",
}

// LaunchConfig is everything that varies between one browser and the next.
type LaunchConfig struct {
	// ProfileDir is the persistent user data directory. It holds the paired
	// session, so it is never a temp dir in production.
	ProfileDir string
	// DebuggingPort is the DevTools port. Zero lets the caller decide later.
	DebuggingPort int
	// UserAgent overrides the browser's identity.
	//
	// Required against the real target, and the reason is compatibility, not
	// evasion: WhatsApp refuses the HeadlessChrome/151 token outright and
	// accepts the identity matching the installed version. Phase 5 lost a run
	// to launching without it. Nothing else about the fingerprint is touched.
	UserAgent string
	// WindowSize overrides DefaultWindowSize. Empty keeps the measured default.
	WindowSize string
	// RemoteAddress overrides DefaultRemoteAddress. Empty keeps loopback.
	RemoteAddress string
	// Headful shows the browser window instead of running headless.
	//
	// It exists for ONE operation: pairing. A QR code is a credential that
	// lives for seconds, and the safest way to get it in front of a person is
	// to put it on their screen — never to screenshot it, which would write
	// that credential to disk, and never to log it.
	//
	// Every measurement in the study was headless, so a headful run is NOT
	// comparable to them: `--headless=new` is a different browser with
	// different memory behaviour, not a display toggle. Nothing that produces
	// a number should set this.
	Headful bool
}

// BuildFlags renders the full argument list for one browser.
func BuildFlags(cfg LaunchConfig) ([]string, error) {
	if cfg.ProfileDir == "" {
		return nil, fmt.Errorf("launch: ProfileDir is required; a browser without a "+
			"persistent profile loses the paired session on every restart (%s)", flagUserDataDir)
	}
	if cfg.DebuggingPort <= 0 {
		return nil, fmt.Errorf("launch: DebuggingPort is required; without it there is "+
			"no CDP endpoint and no way to stop the browser cleanly (%s)", flagRemotePort)
	}

	out := make([]string, 0, len(canonicalProfileV1)+6)
	for _, f := range canonicalProfileV1 {
		// The headless flag has no "off" switch: Chromium honours the last
		// occurrence of most flags, but there is no --headless=false to append.
		// It has to be left out.
		if cfg.Headful && f == flagHeadless {
			continue
		}
		out = append(out, f)
	}

	address := cfg.RemoteAddress
	if address == "" {
		address = DefaultRemoteAddress
	}
	out = append(out,
		flagUserDataDir+cfg.ProfileDir,
		flagRemotePort+fmt.Sprint(cfg.DebuggingPort),
		flagRemoteAddress+address,
	)
	if cfg.UserAgent != "" {
		out = append(out, flagUserAgent+cfg.UserAgent)
	}
	if cfg.WindowSize != "" {
		out = append(out, flagWindowSize+cfg.WindowSize)
	}
	return out, nil
}

// AppendFlags adds extra flags to a rendered set, refusing the one addition
// that silently changes what was measured.
//
// Chromium honours only the LAST occurrence of --disable-features. Appending a
// second one therefore ERASES the canonical list — including site-per-process,
// the largest memory lever the study found — while looking like an addition.
// Phase 6 nearly lost an experiment to exactly this: five candidate flags were
// being compared, and passing each as a bare --disable-features would have
// changed the memory under measurement instead of adding to it.
//
// The safe way to extend the disabled set is DisableFeatures, which merges.
func AppendFlags(base []string, extra ...string) ([]string, error) {
	for _, f := range extra {
		if strings.HasPrefix(f, flagDisableFeature) {
			return nil, fmt.Errorf("launch: %q would be a second %s, and Chromium honours "+
				"only the last one — the canonical list, site-per-process included, would be "+
				"silently erased. Use DisableFeatures to merge", f, flagDisableFeature)
		}
	}
	out := make([]string, 0, len(base)+len(extra))
	out = append(out, base...)
	return append(out, extra...), nil
}

// DisableFeatures merges names into the single --disable-features entry,
// preserving what is already there.
func DisableFeatures(base []string, names ...string) ([]string, error) {
	if len(names) == 0 {
		return base, nil
	}
	out := make([]string, len(base))
	copy(out, base)

	for i, f := range out {
		if !strings.HasPrefix(f, flagDisableFeature) {
			continue
		}
		current := strings.TrimPrefix(f, flagDisableFeature)
		out[i] = flagDisableFeature + current + "," + strings.Join(names, ",")
		return out, nil
	}
	return nil, fmt.Errorf("launch: no %s in the flag set to merge into; the canonical "+
		"profile always carries one, so this set was not built by BuildFlags", flagDisableFeature)
}

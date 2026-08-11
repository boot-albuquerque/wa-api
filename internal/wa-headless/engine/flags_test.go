package engine

import (
	"strings"
	"testing"
)

func flagWithPrefix(flags []string, prefix string) (string, int) {
	var found string
	n := 0
	for _, f := range flags {
		if strings.HasPrefix(f, prefix) {
			if n == 0 {
				found = f
			}
			n++
		}
	}
	return found, n
}

func mustBuild(t *testing.T, cfg LaunchConfig) []string {
	t.Helper()
	flags, err := BuildFlags(cfg)
	if err != nil {
		t.Fatalf("BuildFlags: %v", err)
	}
	return flags
}

var baseConfig = LaunchConfig{ProfileDir: "/session/acct", DebuggingPort: 9222}

// The three flags the study identified as the memory budget. Losing one is not
// a style regression: the spike measured a 790 MB -> 592 MB swing from this
// group, so a silent drop is 200 MB per session that nobody decided to spend.
func TestCanonicalProfileKeepsTheMeasuredMemoryLevers(t *testing.T) {
	flags := mustBuild(t, baseConfig)
	joined := strings.Join(flags, " ")

	for _, required := range []string{
		"site-per-process",
		"--disable-site-isolation-trials",
		"--disable-dev-shm-usage",
		flagHeadless,
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("the flag set lost %q — this is the memory budget, not a preference", required)
		}
	}
}

// Chromium honours only the LAST --disable-features. Two of them is not a
// longer list, it is a REPLACED list.
func TestExactlyOneDisableFeaturesEntry(t *testing.T) {
	flags := mustBuild(t, baseConfig)
	if _, n := flagWithPrefix(flags, flagDisableFeature); n != 1 {
		t.Fatalf("found %d %s entries, want exactly 1; Chromium honours only the last, "+
			"so a second one erases the canonical list", n, flagDisableFeature)
	}
}

func TestAppendFlagsRefusesASecondDisableFeatures(t *testing.T) {
	flags := mustBuild(t, baseConfig)

	_, err := AppendFlags(flags, flagDisableFeature+"SomethingNew")
	if err == nil {
		t.Fatal("a second --disable-features was accepted; it would have silently erased " +
			"site-per-process, the largest memory lever measured")
	}
	if !strings.Contains(err.Error(), "DisableFeatures") {
		t.Errorf("the refusal must point at the merging alternative: %v", err)
	}
}

func TestAppendFlagsAcceptsOrdinaryFlags(t *testing.T) {
	flags := mustBuild(t, baseConfig)

	out, err := AppendFlags(flags, "--mute-audio")
	if err != nil {
		t.Fatalf("AppendFlags: %v", err)
	}
	if len(out) != len(flags)+1 || out[len(out)-1] != "--mute-audio" {
		t.Fatal("an ordinary flag was not appended")
	}
	// The guard must not corrupt what it guards.
	if _, n := flagWithPrefix(out, flagDisableFeature); n != 1 {
		t.Fatal("appending changed the --disable-features entry")
	}
}

func TestDisableFeaturesMergesInsteadOfReplacing(t *testing.T) {
	flags := mustBuild(t, baseConfig)

	out, err := DisableFeatures(flags, "SpareRendererForSitePerProcess")
	if err != nil {
		t.Fatalf("DisableFeatures: %v", err)
	}
	entry, n := flagWithPrefix(out, flagDisableFeature)
	if n != 1 {
		t.Fatalf("found %d %s entries after merging, want 1", n, flagDisableFeature)
	}
	if !strings.Contains(entry, "site-per-process") {
		t.Fatal("merging dropped site-per-process — that is a replacement, not a merge")
	}
	if !strings.Contains(entry, "SpareRendererForSitePerProcess") {
		t.Fatal("merging did not add the requested feature")
	}
	// The caller's slice must not be rewritten under it.
	original, _ := flagWithPrefix(flags, flagDisableFeature)
	if strings.Contains(original, "SpareRendererForSitePerProcess") {
		t.Fatal("DisableFeatures mutated the input slice")
	}
}

// A browser with no persistent profile loses the paired session on every
// restart, and a QR scan is a human action that cannot be retried silently.
func TestBuildFlagsRequiresAProfileDir(t *testing.T) {
	if _, err := BuildFlags(LaunchConfig{DebuggingPort: 9222}); err == nil {
		t.Fatal("a browser with no profile directory was accepted")
	}
}

// Without a debugging port there is no CDP endpoint, and with no CDP endpoint
// the clean shutdown of CAP-02 is unreachable — every stop would be a signal.
func TestBuildFlagsRequiresADebuggingPort(t *testing.T) {
	if _, err := BuildFlags(LaunchConfig{ProfileDir: "/session/acct"}); err == nil {
		t.Fatal("a browser with no debugging port was accepted")
	}
}

// The DevTools endpoint is arbitrary code execution in a browser holding a
// paired session. It binds to loopback unless someone says otherwise out loud.
func TestDebuggingEndpointBindsToLoopbackByDefault(t *testing.T) {
	flags := mustBuild(t, baseConfig)

	entry, n := flagWithPrefix(flags, flagRemoteAddress)
	if n != 1 {
		t.Fatalf("found %d %s entries, want 1", n, flagRemoteAddress)
	}
	if entry != flagRemoteAddress+DefaultRemoteAddress {
		t.Fatalf("got %q, want loopback: an open CDP port lets anyone read every message "+
			"and send as the owner", entry)
	}
	if strings.Contains(entry, "0.0.0.0") {
		t.Fatal("the DevTools endpoint is bound to every interface by default")
	}
}

// The user agent is required against the real target — WhatsApp refuses the
// HeadlessChrome token — so it must actually reach the command line.
func TestUserAgentAndWindowSizeOverridesReachTheCommandLine(t *testing.T) {
	cfg := baseConfig
	cfg.UserAgent = "Mozilla/5.0 (X11; Linux x86_64) Chrome/151.0.0.0"
	cfg.WindowSize = "1600,1200"

	flags := mustBuild(t, cfg)

	if entry, n := flagWithPrefix(flags, flagUserAgent); n != 1 || !strings.Contains(entry, "Chrome/151") {
		t.Errorf("user agent did not reach the command line: %q (n=%d)", entry, n)
	}
	// Chromium honours the LAST --window-size, so the override must come after
	// the canonical one; asserting only "it is present" would pass with the
	// override buried before the default and silently ignored.
	last := ""
	for _, f := range flags {
		if strings.HasPrefix(f, flagWindowSize) {
			last = f
		}
	}
	if last != flagWindowSize+"1600,1200" {
		t.Errorf("the effective window size is %q, want the override — Chromium honours "+
			"the last occurrence, so order decides", last)
	}
}

func TestDefaultsAreUsedWhenNothingIsOverridden(t *testing.T) {
	flags := mustBuild(t, baseConfig)

	if _, n := flagWithPrefix(flags, flagUserAgent); n != 0 {
		t.Error("a user agent was injected without being asked for")
	}
	entry, n := flagWithPrefix(flags, flagWindowSize)
	if n != 1 || entry != flagWindowSize+DefaultWindowSize {
		t.Errorf("got %q (n=%d), want the measured default %q", entry, n, DefaultWindowSize)
	}
}

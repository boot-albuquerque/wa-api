package core

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// harnessBootBudget is what the TESTS give a browser boot, and it is
// deliberately not what PRODUCTION gives one.
//
// engine.DefaultDeadlines.Boot is 30s, chosen from a measurement: a usable SPA
// pre-login took 6.1-8.5s, and 30s was headroom for a cold cache. That number
// is a product decision and stays where it is.
//
// The tests are a different situation. They run dozens of browsers, under
// -race, on a machine that is also running the rest of the gate — and F100
// recorded SEVEN failures where a test timed out at 30s and then passed in
// 4-16s when run alone. Three of those were on one day. What that measures is
// the machine, not the code, and a red gate that means "the laptop was busy"
// costs more than it protects: three make check runs were lost to it in a
// single session.
//
// SO 90s IS A HARNESS BUDGET, NOT AN SLA. It says "if this has not booted in a
// minute and a half, something is actually wrong", which is the only claim the
// tests need. TestAHungBootStillTerminatesBounded keeps it from becoming a
// licence to hang.
// RAISED FROM 90s TO 150s ON 2026-08-21, and the reason is arithmetic rather
// than superstition: this package gained more real browser boots with the
// lifecycle facts (H88), and `make check` runs it under -race alongside every
// other package. The ninth F100 failure waited the full ninety seconds and
// still had not primed a tab.
//
// Raising a ceiling is the move this repository distrusts most, so it comes
// with the two things that make it honest: the budget is still strictly greater
// than the PRODUCT's (TestTheHarnessBudgetIsNotTheProductBudget) and a hung boot
// still terminates within it (TestAHungBootStillTerminatesBounded). A raised
// ceiling that is still a ceiling is a different thing from no ceiling.
const harnessBootBudget = 150 * time.Second

// harnessRunner builds a Runner with the harness boot budget and production
// values everywhere else.
func harnessRunner() *engine.Runner {
	r := engine.NewRunner()
	r.Policy.Boot = harnessBootBudget
	return r
}

// TestTheHarnessBudgetIsNotTheProductBudget locks the distinction rather than
// leaving it in a comment. Raising DefaultDeadlines.Boot to make the gate
// quieter would change what the product promises, and this test fails if
// somebody does that.
func TestTheHarnessBudgetIsNotTheProductBudget(t *testing.T) {
	if engine.DefaultDeadlines.Boot != 30*time.Second {
		t.Fatalf("the PRODUCT boot deadline is %s; it was 30s and moving it is a "+
			"product decision that needs its own measurement, not a side effect "+
			"of making the test suite quieter", engine.DefaultDeadlines.Boot)
	}
	if harnessBootBudget <= engine.DefaultDeadlines.Boot {
		t.Fatalf("the harness budget (%s) is not above the product one (%s), so it "+
			"does nothing about the load sensitivity it exists for",
			harnessBootBudget, engine.DefaultDeadlines.Boot)
	}
	if harnessRunner().Policy.Navigate != engine.DefaultDeadlines.Navigate {
		t.Fatal("the harness runner changed a deadline other than Boot; only the " +
			"boot budget was measured as load-sensitive")
	}
}

// TestAHungBootStillTerminatesBounded is the control the raised ceiling needs.
//
// A budget that is only ever raised eventually becomes no budget at all. This
// proves the mechanism still gives up: given a tiny boot budget and a browser
// that will never become ready, StartSession returns an error, and it returns
// it WITHIN the budget rather than after it.
//
// It uses a small budget on purpose — the property under test is "the boot
// deadline bounds the wait", and proving that with 90 seconds would cost 90
// seconds to learn the same thing.
func TestAHungBootStillTerminatesBounded(t *testing.T) {
	const tiny = 2 * time.Second

	// A page that loads but never becomes a usable SPA: boot has something to
	// talk to and nothing to conclude, which is the shape of a hang.
	cfg := baseConfig(t, pageServer(t, "/blank", `<html><body>nothing here</body></html>`))
	r := engine.NewRunner()
	r.Policy.Boot = tiny
	cfg.Runner = r
	cfg.SettleBudget = tiny

	start := time.Now()
	sess, err := StartSession(context.Background(), cfg)
	elapsed := time.Since(start)
	if err == nil {
		via := sess.Stop(context.Background())
		t.Fatalf("a page that never becomes ready reached READY (stopped_via=%s)", via)
	}
	var boot *BootFailure
	if !errors.As(err, &boot) {
		t.Fatalf("want *BootFailure, got %T: %v", err, err)
	}
	// The ceiling is generous against launch and navigate, which happen before
	// the settle loop and are not what this bounds.
	if elapsed >= harnessBootBudget {
		t.Fatalf("a boot given %s took %s — the budget is not bounding the wait, "+
			"which is exactly what raising it to %s would otherwise hide",
			tiny, elapsed, harnessBootBudget)
	}
}

// TestTheInjectedBudgetGovernsTheTabPrimingToo closes the hole that made F100
// look like eight unrelated flakes.
//
// A caller that raises Runner.Policy.Boot is saying "this boot may take longer";
// every step honoured that except OpenTab, which hard-coded the default. The
// step it skipped is the one that actually runs long under contention, so the
// raised budget helped exactly where it was not needed.
//
// The assertion is on the SOURCE rather than on a timing, because timing is what
// made these failures unreadable in the first place: a test that waits for a
// slow boot is a test that fails on a fast machine or passes on a broken one.
func TestTheInjectedBudgetGovernsTheTabPrimingToo(t *testing.T) {
	body, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatalf("read session.go: %v", err)
	}
	src := string(body)
	if strings.Contains(src, "engine.OpenTab(sessionCtx") {
		t.Fatal("the boot opens the tab with engine.OpenTab, which hard-codes " +
			"DefaultDeadlines.For(OpBoot) and ignores the runner it was given")
	}
	if !strings.Contains(src, "engine.OpenTabWithin(sessionCtx, browser, runner.Policy.For(engine.OpBoot))") {
		t.Fatal("the boot does not pass the runner's own boot budget to the tab priming")
	}
}

// TestEveryBootInTheseTestsCarriesTheHarnessBudget stops F100 from coming back
// through the door it kept using.
//
// The budget was introduced once, in baseConfig, and two tests that build a
// StartConfig by hand never got it. They kept the PRODUCT's thirty seconds
// while the rest of the package had a hundred and fifty, and under contention
// they were the ones that failed — which read as flakiness rather than as a
// config that was missed.
//
// A rule stated in a comment would have been missed the same way. This is the
// same rule as a gate: every StartConfig literal in this package's tests names
// a Runner.
func TestEveryBootInTheseTestsCarriesTheHarnessBudget(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var offenders []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		lines := strings.Split(string(body), "\n")
		for i, raw := range lines {
			// STRIP THE STRING LITERALS BEFORE MATCHING, and the first run of
			// this gate is why: it flagged its OWN source, because the line that
			// searches for the marker contains the marker. Comments were already
			// being skipped; a quoted string is the same trap wearing different
			// punctuation, and this repository has now met it in three forms.
			line := withoutStringLiterals(raw)
			if !strings.Contains(line, "StartConfig{") ||
				strings.HasPrefix(strings.TrimSpace(raw), "//") {
				continue
			}
			// An explicit exception, with a reason, for a config that never
			// boots — the same discipline the page-clock gate's allowlist uses.
			exempt := strings.Contains(raw, "no-runner:")
			for k := i - 1; k >= 0 && k >= i-3 && !exempt; k-- {
				if !strings.HasPrefix(strings.TrimSpace(lines[k]), "//") {
					break
				}
				exempt = strings.Contains(lines[k], "no-runner:")
			}
			if exempt {
				continue
			}
			// Read forward to the literal's closing brace, shallowly: these are
			// all flat literals, and a brace counter would be more machinery
			// than the thing it checks.
			found := false
			for j := i; j < len(lines) && j < i+16; j++ {
				if strings.Contains(lines[j], "Runner:") {
					found = true
					break
				}
				if j > i && strings.TrimSpace(lines[j]) == "}" {
					break
				}
			}
			if !found {
				offenders = append(offenders, e.Name()+":"+strconv.Itoa(i+1))
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("StartConfig literal(s) with no Runner at %s — they would boot on the "+
			"PRODUCT's %s instead of the harness's %s, and fail under contention as F100 "+
			"did eight times. Use harnessRunner().",
			strings.Join(offenders, ", "), engine.DefaultDeadlines.Boot, harnessBootBudget)
	}
}

// withoutStringLiterals blanks the contents of double-quoted strings, so a gate
// that looks for a token in CODE is not satisfied by the same token appearing
// inside a string — including the gate's own source.
//
// Naive by design: it does not handle backtick strings or escaped quotes. It is
// used on one package's test files, all of which are ordinary Go, and a helper
// that pretended to lex Go would be a worse thing to trust than a stated limit.
func withoutStringLiterals(line string) string {
	var b strings.Builder
	in := false
	for _, r := range line {
		if r == '"' {
			in = !in
			b.WriteRune(r)
			continue
		}
		if in {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

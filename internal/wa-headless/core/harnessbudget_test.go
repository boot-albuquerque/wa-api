package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
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
const harnessBootBudget = 90 * time.Second

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

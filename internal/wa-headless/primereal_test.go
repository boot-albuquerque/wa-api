package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAPrimeDoesNotDamageTheRoster proves the postcondition against the
// live account, and it is deliberately NOT a test that the refresh improves
// anything.
//
// Measured 2026-08-20: doFullContactSync took 41.9s and moved one number —
// verified business names, 52 to 56. Total unchanged at 944, getName unchanged
// at 1. Asserting improvement would encode an outcome the mechanism does not
// promise, and the test would fail on any account that is already current.
//
// What IS asserted is the thing a caller cannot check for themselves: the
// refresh MUTATES, and a roster that comes back smaller has lost people.
func TestRealSPAPrimeDoesNotDamageTheRoster(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_PRIME_TEST") == "" {
		t.Skip("set WA_HEADLESS_PRIME_TEST=1; this triggers a real ~42s contact sync")
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
	// Minutes, not the boot deadline: the operation itself takes ~42s, and a
	// boot-scoped parent would expire mid-refresh and blame the page (H42).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	lister := contacts.New(runner, eval)

	got, err := lister.Prime(ctx, "real/prime")
	if err != nil {
		t.Fatalf("Prime: %v", err)
	}
	t.Logf("%s", got)

	// THE POSTCONDITION, and it is the one that makes this test able to fail.
	// The sync returns undefined, so command success proves nothing; the page's
	// own refresh mark moving is the evidence that it executed.
	//
	// Negative control EXECUTED against this very test: with production changed
	// so it does not call doFullContactSync, this line fires and the failure
	// even carries the tell — "returned after 5ms" against the real 42s.
	if !got.Ran() {
		t.Fatal("the refresh did not run: the page's sync mark did not move, so the " +
			"assertions below would be describing a roster nobody touched")
	}
	if got.Before.Total == 0 {
		t.Fatal("the roster was empty BEFORE the refresh; this account has contacts, " +
			"so a zero baseline means the snapshot is broken and the postcondition " +
			"below would be comparing nothing to nothing")
	}
	if got.After.Total < got.Before.Total {
		t.Fatalf("the roster shrank: %d -> %d", got.Before.Total, got.After.Total)
	}
	if got.Waited <= 0 {
		t.Fatal("the measured cost came back as zero, so nothing was actually awaited")
	}
	// The cost is the number a caller budgets around, so it is reported rather
	// than asserted against a threshold that would just encode today's weather.
	t.Logf("refresh cost %s (measured 41.9s on 2026-08-20)", got.Waited.Round(time.Second))

	// AND THE ROSTER MUST STILL BE READABLE AFTERWARDS. A refresh that leaves
	// the collection in a state List cannot walk would pass every check above
	// and break the capability that matters.
	roster, err := lister.List(ctx, "real/prime/after")
	if err != nil {
		t.Fatalf("List after Prime: %v", err)
	}
	t.Logf("roster after the refresh: %s", roster)
	if len(roster.Contacts) == 0 {
		t.Fatal("no contact survived the refresh")
	}
}

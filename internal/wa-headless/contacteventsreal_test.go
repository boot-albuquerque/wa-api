package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/avatar"
	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAContactChangesAreObserved proves onContact against the live page,
// and it PROVOKES the change rather than waiting for one.
//
// Waiting would work — the roster produced 7 change events in 90 idle seconds —
// but a test that depends on someone else's account being busy is a test that
// fails on a quiet afternoon and passes on a loud one. The measurement showed
// that 24 of the 46 field-level events came from profilePicThumb, and
// fetchContactAvatar is exactly what writes those fields. So the avatar
// capability becomes the stimulus.
//
// That also makes this the second integration proof of a pair: one capability's
// output is another's input, which is what caught H41.
func TestRealSPAContactChangesAreObserved(t *testing.T) {
	requireRealSPA(t)
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Skip("WA_SEND_FROM_PROFILE is required for the live roster")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	// Ten minutes, not the boot deadline: this test sleeps, and a boot-scoped
	// parent would expire mid-measurement and report the page as unresponsive
	// (HOUSEKEEP H42).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	roster, err := contacts.New(runner, eval).List(ctx, "real/ev/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(roster.Contacts) < 6 {
		t.Skipf("the roster has %d contacts; this proof needs at least 6", len(roster.Contacts))
	}

	sub := contacts.Subscribe(runner, eval, 0)
	if err := sub.Install(ctx, "real/ev/install"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Prime: whatever was already buffered is not evidence of anything this
	// test caused.
	primed, err := sub.Drain(ctx, "real/ev/prime")
	if err != nil {
		t.Fatalf("priming drain: %v", err)
	}
	t.Logf("primed away: %s", primed)

	// THE STIMULUS.
	f := avatar.New(runner, eval)
	asked := 0
	for i := 0; i < 6; i++ {
		if _, err := f.Fetch(ctx, roster.Contacts[i].Identity(), "real/ev/avatar"); err != nil {
			t.Fatalf("avatar fetch %d: %v", i+1, err)
		}
		asked++
	}
	t.Logf("provoked with %d avatar fetches", asked)

	deadline := time.Now().Add(60 * time.Second)
	var total, changes, adds int
	for time.Now().Before(deadline) {
		d, err := sub.Drain(ctx, "real/ev/drain")
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if d.Reinstalled {
			t.Fatal("the subscription was lost mid-test; the events between the loss " +
				"and the reinstall are uncountable, so this run cannot answer the question")
		}
		for _, e := range d.Events {
			total++
			switch e.Name {
			case contacts.EventChange:
				changes++
			case contacts.EventAdd:
				adds++
			}
			if e.Contact.Identity() == "" {
				t.Fatal("an event arrived with no identity at all")
			}
		}
		if changes > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("observed %d event(s): change=%d add=%d", total, changes, adds)

	if changes == 0 {
		t.Fatal("six avatar fetches produced no 'change' event. The measurement saw " +
			"24 profilePicThumb field events in 90 idle seconds, and this test writes " +
			"exactly those fields — so a zero here means the subscription is bound to " +
			"a word this build does not emit, which is the failure mode that has NO " +
			"symptom: it installs cleanly and delivers nothing")
	}
}

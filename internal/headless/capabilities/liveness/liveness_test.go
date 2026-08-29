package liveness

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// pageDouble stands in for the browser.
//
// THE DOUBLE IMITATES THE REAL RULE, not a friendlier one (ARMADILHAS §1): the
// production liveness script answers `JSON.stringify(!!document.querySelector(
// '#pane-side'))`, so a mounted application is the four bytes "true" and an
// unmounted one is "false". A double that returned Go booleans, or an empty
// string, would let a decoding bug through — spa.Monitor unmarshals this text.
type pageDouble struct {
	answers []string // one per call; the last is reused once exhausted
	errs    []error
	calls   int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	//
	// engine.Tab.Evaluate derives from the caller's context, so a cancelled or
	// expired ctx fails there. A double that ignored ctx would be better
	// behaved than the world in exactly the axis ARMADILHAS §1 warns about —
	// and it would let a capability fabricate a successful answer for a caller
	// that had already given up.
	if err := ctx.Err(); err != nil {
		return err
	}
	i := p.calls
	p.calls++
	if i < len(p.errs) && p.errs[i] != nil {
		return p.errs[i]
	}
	switch {
	case len(p.answers) == 0:
		*out = "false"
	case i < len(p.answers):
		*out = p.answers[i]
	default:
		*out = p.answers[len(p.answers)-1]
	}
	return nil
}

func newChecker(processAlive bool, page *pageDouble) *Checker {
	return New(func() bool { return processAlive }, engine.NewRunner(), page.eval)
}

func TestAliveWhenProcessRunsAndAppIsMounted(t *testing.T) {
	page := &pageDouble{answers: []string{"true"}}
	got := newChecker(true, page).Check(context.Background(), "test/alive")

	if !got.Alive || got.Signal != SignalAlive {
		t.Fatalf("alive=%v signal=%s, want true/%s", got.Alive, got.Signal, SignalAlive)
	}
	if !got.PageProbed {
		t.Fatal("PageProbed=false on a check that reached the page")
	}
	if got.Class != spa.ClassAppReady {
		t.Fatalf("class=%s, want %s", got.Class, spa.ClassAppReady)
	}
}

// TestDeadProcessIsNamedAsSuchAndSkipsThePageProbe is the capability's reason to
// exist: the two signals must not be fused.
//
// It asserts BOTH halves. The signal must name the process, not the page — a
// checker that reported PAGE_UNRESPONSIVE here would be naming the consequence
// and sending its caller to debug a renderer that was never the problem. And
// the page must not be probed at all, because on a dead process that probe can
// only spend a full StateProbe budget to rediscover what is already known.
func TestDeadProcessIsNamedAsSuchAndSkipsThePageProbe(t *testing.T) {
	page := &pageDouble{answers: []string{"true"}}
	got := newChecker(false, page).Check(context.Background(), "test/dead")

	if got.Alive {
		t.Fatal("reported alive with the browser process gone")
	}
	if got.Signal != SignalProcessGone {
		t.Fatalf("signal=%s, want %s: naming the page here would send the caller to "+
			"debug a renderer that was never the problem", got.Signal, SignalProcessGone)
	}
	if page.calls != 0 {
		t.Fatalf("the page was probed %d times on a dead process; the probe cannot "+
			"succeed and would spend a full budget to learn what is already known",
			page.calls)
	}
	if got.PageProbed {
		t.Fatal("PageProbed=true when no probe ran — a zero Class and Latency would " +
			"then read as 'the page answered instantly'")
	}
}

// TestAppAbsentIsNotUnresponsive separates the third signal from the second. A
// page that ANSWERS "the app is not mounted" is executing: it is a QR screen or
// a redirect, not a dead renderer, and recycling it would be the wrong repair.
func TestAppAbsentIsNotUnresponsive(t *testing.T) {
	page := &pageDouble{answers: []string{"false"}}
	got := newChecker(true, page).Check(context.Background(), "test/absent")

	if got.Alive {
		t.Fatal("reported alive with the application not mounted")
	}
	if got.Signal != SignalAppAbsent {
		t.Fatalf("signal=%s, want %s", got.Signal, SignalAppAbsent)
	}
	if !got.PageProbed || page.calls != 1 {
		t.Fatalf("PageProbed=%v calls=%d, want true/1", got.PageProbed, page.calls)
	}
}

// TestOneFailureIsSlowAndTheStreakIsUnresponsive locks the threshold boundary
// the monitor owns, seen through the capability. One blown deadline is a loaded
// host; the streak is a verdict. Reporting the first as UNRESPONSIVE would
// recycle healthy sessions off a busy machine.
func TestOneFailureIsSlowAndTheStreakIsUnresponsive(t *testing.T) {
	boom := errors.New("probe exploded")
	page := &pageDouble{errs: []error{boom, boom, boom}}
	c := newChecker(true, page)

	first := c.Check(context.Background(), "test/f1")
	if first.Signal != SignalPageSlow {
		t.Fatalf("first failure signal=%s, want %s", first.Signal, SignalPageSlow)
	}
	if first.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive=%d after one failure, want 1", first.ConsecutiveFailures)
	}

	_ = c.Check(context.Background(), "test/f2")
	third := c.Check(context.Background(), "test/f3")
	if third.Signal != SignalPageUnresponsive {
		t.Fatalf("third consecutive failure signal=%s, want %s (spa.DefaultUnresponsiveAfter=%d)",
			third.Signal, SignalPageUnresponsive, spa.DefaultUnresponsiveAfter)
	}
	if third.Class != spa.ClassUnresponsive {
		t.Fatalf("class=%s, want %s", third.Class, spa.ClassUnresponsive)
	}
}

// TestRecoveryClearsTheStreak proves the capability does not latch. A session
// that answers again is alive again; a checker that stayed UNRESPONSIVE would
// make one bad minute permanent.
func TestRecoveryClearsTheStreak(t *testing.T) {
	boom := errors.New("probe exploded")
	page := &pageDouble{answers: []string{"", "true"}, errs: []error{boom}}
	c := newChecker(true, page)

	if got := c.Check(context.Background(), "test/fail"); got.Alive {
		t.Fatal("reported alive on a failed probe")
	}
	got := c.Check(context.Background(), "test/recover")
	if !got.Alive || got.Signal != SignalAlive {
		t.Fatalf("after recovery alive=%v signal=%s, want true/%s",
			got.Alive, got.Signal, SignalAlive)
	}
	if got.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive=%d after a good probe, want 0", got.ConsecutiveFailures)
	}
}

// TestEverySignalIsSet guards the one property a caller cannot check for
// itself: no path may leave Signal empty. An empty Signal would compare unequal
// to every constant and silently take the default branch of any switch.
func TestEverySignalIsSet(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]Report{
		"alive":   newChecker(true, &pageDouble{answers: []string{"true"}}).Check(context.Background(), "s/1"),
		"absent":  newChecker(true, &pageDouble{answers: []string{"false"}}).Check(context.Background(), "s/2"),
		"dead":    newChecker(false, &pageDouble{}).Check(context.Background(), "s/3"),
		"failed":  newChecker(true, &pageDouble{errs: []error{boom}}).Check(context.Background(), "s/4"),
		"garbled": newChecker(true, &pageDouble{answers: []string{"not json"}}).Check(context.Background(), "s/5"),
	}
	for name, got := range cases {
		if got.Signal == "" {
			t.Errorf("%s: Signal is empty; it would fall through every switch a caller writes", name)
		}
	}
}

// TestCancelledContextIsNotAFabricatedSuccess exercises the axis the double
// used to ignore, and liveness is where it matters most: this is the capability
// a hot path calls, so it is the likeliest to be handed a context whose caller
// has already given up.
//
// It returns a Report rather than an error, so the assertion is different from
// the other capabilities: the session must NOT be reported alive on the word of
// a probe that never ran.
func TestCancelledContextIsNotAFabricatedSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := newChecker(true, &pageDouble{answers: []string{"true"}}).Check(ctx, "t/cancel")
	if got.Alive {
		t.Fatal("reported ALIVE from a probe that could not run: the page was never asked, " +
			"and answering for it is how a dead session looks healthy")
	}
	if got.Err == nil {
		t.Fatal("the cancellation was not carried in the report")
	}
	if got.Signal == SignalAlive {
		t.Fatalf("signal=%s on a cancelled probe", got.Signal)
	}
}

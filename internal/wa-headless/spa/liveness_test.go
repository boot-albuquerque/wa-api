package spa

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// scriptedPage answers the liveness probe from a queue of outcomes, and records
// what it was asked. The recording matters: the probe must never send anything,
// and "it did not send" cannot be checked by looking at the result.
type scriptedPage struct {
	mu       sync.Mutex
	answers  []any // bool, or error
	i        int
	asked    []string
	sendSeen bool
}

func (p *scriptedPage) eval(ctx context.Context, expression string, out *string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, expression)
	// The product contract rejects synthetic-send explicitly. Anything that
	// looks like composing or dispatching is a contract violation, and the
	// double is where that gets caught.
	for _, banned := range []string{"sendMessage", "sendText", "Composer", "submit("} {
		if strings.Contains(expression, banned) {
			p.sendSeen = true
		}
	}

	if p.i >= len(p.answers) {
		return errors.New("scriptedPage: ran out of answers")
	}
	a := p.answers[p.i]
	p.i++
	switch v := a.(type) {
	case error:
		return v
	case bool:
		if v {
			*out = "true"
		} else {
			*out = "false"
		}
		return nil
	case string:
		*out = v
		return nil
	}
	return errors.New("scriptedPage: bad answer")
}

func monitorOn(p *scriptedPage, threshold int) *Monitor {
	pol := engine.DefaultDeadlines
	pol.StateProbe = 100 * time.Millisecond
	return &Monitor{
		Runner:            &engine.Runner{Policy: pol},
		Eval:              p.eval,
		UnresponsiveAfter: threshold,
	}
}

func TestCheckReportsALiveSession(t *testing.T) {
	page := &scriptedPage{answers: []any{true}}
	m := monitorOn(page, 3)

	res := m.Check(context.Background(), "probe")

	if !res.Alive || res.Class != ClassAppReady {
		t.Fatalf("alive=%v class=%q, want true/%q", res.Alive, res.Class, ClassAppReady)
	}
	if res.Unresponsive() {
		t.Error("a session that answered was reported unresponsive")
	}
}

// The whole point of the capability: a probe that answered `1` would prove the
// renderer executes JavaScript and nothing more. The script has to ask about
// the application.
func TestProbeAsksAboutTheApplicationNotAConstant(t *testing.T) {
	page := &scriptedPage{answers: []any{true}}
	m := monitorOn(page, 3)

	m.Check(context.Background(), "probe")

	if len(page.asked) != 1 {
		t.Fatalf("asked %d expressions, want 1", len(page.asked))
	}
	if !strings.Contains(page.asked[0], "#pane-side") {
		t.Fatalf("the probe asked %q; a constant expression proves the renderer runs, "+
			"not that the application is there", page.asked[0])
	}
}

// The product contract rejects synthetic-send at Level 3, explicitly. A
// liveness probe that sends a message to prove the session works has destroyed
// the thing it was measuring.
func TestProbeNeverSendsAnything(t *testing.T) {
	page := &scriptedPage{answers: []any{true}}
	m := monitorOn(page, 3)

	m.Check(context.Background(), "probe")

	if page.sendSeen {
		t.Fatal("the liveness probe tried to send: read-only is the contract, and a " +
			"synthetic send is a real message to somebody")
	}
}

// One blown deadline is not a verdict. Recycling on it would kill healthy
// sessions off a loaded host.
func TestOneFailureIsNotYetUnresponsive(t *testing.T) {
	page := &scriptedPage{answers: []any{context.DeadlineExceeded}}
	m := monitorOn(page, 3)

	res := m.Check(context.Background(), "probe")

	if res.Alive {
		t.Error("a failed probe was reported alive")
	}
	if res.Unresponsive() {
		t.Fatal("a single failure was enough to declare the session lost; a loaded " +
			"host would have its healthy sessions recycled")
	}
	if res.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", res.ConsecutiveFailures)
	}
}

// The measured state lasted over four minutes. After the threshold the session
// is LOST, not idle — that distinction is what stops a fleet counting a dead
// session as capacity.
func TestConsecutiveFailuresReachUnresponsive(t *testing.T) {
	page := &scriptedPage{answers: []any{
		context.DeadlineExceeded, context.DeadlineExceeded, context.DeadlineExceeded,
	}}
	m := monitorOn(page, 3)

	var res LivenessResult
	for i := 0; i < 3; i++ {
		res = m.Check(context.Background(), "probe")
	}

	if !res.Unresponsive() {
		t.Fatalf("class = %q after 3 consecutive failures, want %q", res.Class, ClassUnresponsive)
	}
	if res.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", res.ConsecutiveFailures)
	}
}

// A streak is a STREAK. One answer in the middle means the page is executing,
// and phase 6's own trace had exactly that shape — one answer at t=1s and then
// silence. Counting cumulative failures instead would declare sessions dead on
// scattered slow probes.
func TestASuccessResetsTheStreak(t *testing.T) {
	page := &scriptedPage{answers: []any{
		context.DeadlineExceeded, context.DeadlineExceeded, true, context.DeadlineExceeded,
	}}
	m := monitorOn(page, 3)

	m.Check(context.Background(), "probe")
	m.Check(context.Background(), "probe")
	if res := m.Check(context.Background(), "probe"); res.ConsecutiveFailures != 0 {
		t.Fatalf("a successful probe left %d consecutive failures", res.ConsecutiveFailures)
	}
	res := m.Check(context.Background(), "probe")
	if res.ConsecutiveFailures != 1 {
		t.Fatalf("ConsecutiveFailures = %d after the reset, want 1", res.ConsecutiveFailures)
	}
	if res.Unresponsive() {
		t.Fatal("the streak was not reset by a successful probe")
	}
}

// The second half of the phase 6 rule: degradation shows as a probe that takes
// three seconds long before it shows as a probe that times out. A monitor that
// only reports the boolean learns nothing until the session is already gone.
func TestLatencyIsRecordedOnSuccessAndOnFailure(t *testing.T) {
	clock := time.Unix(0, 0)
	page := &scriptedPage{answers: []any{true, context.DeadlineExceeded}}
	m := monitorOn(page, 3)
	m.Now = func() time.Time {
		clock = clock.Add(40 * time.Millisecond)
		return clock
	}

	ok := m.Check(context.Background(), "probe")
	if ok.Latency <= 0 {
		t.Error("a successful probe recorded no latency; degradation would be invisible")
	}
	bad := m.Check(context.Background(), "probe")
	if bad.Latency <= 0 {
		t.Error("a failed probe recorded no latency")
	}

	probes, failures, last, _ := m.Stats()
	if probes != 2 || failures != 1 {
		t.Errorf("Stats() = %d probes, %d failures; want 2, 1", probes, failures)
	}
	if last <= 0 {
		t.Error("Stats() reports no last latency")
	}
}

// A page that is executing but has no application mounted is neither alive nor
// unresponsive. Folding it into either would hide a QR screen behind the wrong
// word — and the recycler acts on that word.
func TestAnExecutingPageWithNoApplicationIsNotUnresponsive(t *testing.T) {
	page := &scriptedPage{answers: []any{false, false, false, false}}
	m := monitorOn(page, 3)

	var res LivenessResult
	for i := 0; i < 4; i++ {
		res = m.Check(context.Background(), "probe")
	}

	if res.Alive {
		t.Error("a page with no application was reported alive")
	}
	if res.Unresponsive() {
		t.Fatal("a page that ANSWERED four times was declared unresponsive; the " +
			"session may just be showing a QR code, and recycling it wastes a scan")
	}
}

// The probe must terminate on its budget, or it is the phase 6 hang with extra
// steps.
func TestCheckIsBoundedByTheStateProbeBudget(t *testing.T) {
	slow := func(ctx context.Context, expression string, out *string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	pol := engine.DefaultDeadlines
	pol.StateProbe = 120 * time.Millisecond
	m := &Monitor{Runner: &engine.Runner{Policy: pol}, Eval: slow, UnresponsiveAfter: 1}

	start := time.Now()
	res := m.Check(context.Background(), "probe")
	elapsed := time.Since(start)

	if !res.Unresponsive() {
		t.Fatalf("class = %q, want %q", res.Class, ClassUnresponsive)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Check took %v on a 120ms budget", elapsed)
	}
}

// A probe timer and a command path both ask. Run under -race.
func TestCheckIsSafeUnderConcurrency(t *testing.T) {
	page := &scriptedPage{}
	for i := 0; i < 64; i++ {
		page.answers = append(page.answers, true)
	}
	m := monitorOn(page, 3)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				m.Check(context.Background(), "probe")
				m.Stats()
			}
		}()
	}
	wg.Wait()

	if probes, _, _, _ := m.Stats(); probes != 64 {
		t.Fatalf("recorded %d probes, want 64 — results were lost", probes)
	}
}

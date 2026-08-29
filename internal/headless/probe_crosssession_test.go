package headless

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeWhyCrossSessionExists asks the question the event bus was the missing
// instrument for.
//
// Three capabilities are CROSS_SESSION (H82): the change reaches the server and
// the session that made it never sees it. Two explanations fit that observation
// and they need different repairs:
//
//	the session RECEIVES the server's notification and does not apply it
//	   -> the model is stale for a reason inside the page, and there may be a
//	      way to make it refresh
//
//	the notification never arrives at this session at all
//	   -> nothing inside the page can help, and cross-session is the ceiling
//
// Until the bus existed there was no way to tell them apart. Now there is:
// subscribe to everything, make a CROSS_SESSION change, and see whether ANY
// event arrives that mentions the thing that changed.
//
// The change is a group policy flipped to its opposite and flipped back, which
// is the reversible one of the three.
func TestProbeWhyCrossSessionExists(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CROSSSESSION") == "" {
		t.Skip("set WA_PROBE_CROSSSESSION=1; this flips a lab group policy and flips it back")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	oldPoll := events.PollInterval
	events.PollInterval = 200 * time.Millisecond
	defer func() { events.PollInterval = oldPoll }()

	hub := events.NewHub()
	pump := events.NewPump(runner, sess.Tab().Evaluate, hub)
	pctx, stop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = pump.Run(pctx) }()
	defer func() { stop(); wg.Wait(); _ = pump.Uninstall(context.Background()) }()

	type note struct {
		typ    events.Type
		chat   string
		replay bool
		at     time.Time
	}
	var mu sync.Mutex
	var notes []note
	hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		notes = append(notes, note{e.Type, e.ChatJID, e.Replay, time.Now()})
	})

	// Let the replay settle so the count that follows is about the change.
	time.Sleep(3 * time.Second)
	mu.Lock()
	base := len(notes)
	mu.Unlock()
	t.Logf("MEASURED: %d event(s) before the change (replay and idle traffic)", base)

	m := group.New(runner, sess.Tab().Evaluate)
	const p = group.PolicyMessagesAdminsOnly
	was, err := m.PolicyOf(ctx, gjid, p, "xs/read")
	if err != nil {
		t.Fatalf("PolicyOf: %v", err)
	}
	defer func() {
		if _, err := m.SetPolicy(context.Background(), gjid, p, was, "xs/restore"); err != nil {
			t.Errorf("RESTORE FAILED — the lab group keeps a flipped policy: %v", err)
		}
	}()

	changeAt := time.Now()
	if _, err := m.SetPolicy(ctx, gjid, p, !was, "xs/flip"); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	// HOW LONG until the model reflects it, measured rather than assumed. The
	// classifier answered "not-here" for this write and it was WRONG, because
	// the control wrote the value the group already had — and a no-op cannot
	// move a reader. This is the measurement that control should have been.
	flipSeen := time.Duration(-1)
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); {
		cur, err := m.PolicyOf(ctx, gjid, p, "xs/poll")
		if err == nil && cur != was {
			flipSeen = time.Since(changeAt)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if flipSeen >= 0 {
		t.Logf("MEASURED: the policy became visible IN THIS SESSION after %s", flipSeen.Round(100*time.Millisecond))
	} else {
		t.Log("MEASURED: the policy never became visible in this session within 60s")
	}
	time.Sleep(5 * time.Second)

	mu.Lock()
	var after []note
	for _, n := range notes {
		if n.at.After(changeAt) {
			after = append(after, n)
		}
	}
	mu.Unlock()

	var aboutTheGroup int
	byType := map[events.Type]int{}
	for _, n := range after {
		byType[n.typ]++
		if n.chat == gjid {
			aboutTheGroup++
		}
	}
	t.Logf("MEASURED: %d event(s) after the change; by type %v", len(after), byType)
	t.Logf("MEASURED: %d of them name the group that changed", aboutTheGroup)

	// THE ANSWER, stated as the two explanations rather than as a pass/fail:
	// this probe exists to distinguish them, and either outcome is information.
	if aboutTheGroup > 0 {
		t.Logf("ANSWER: the session DOES receive notifications about this group after the " +
			"change, so the model being stale is something the page decides — there may " +
			"be a way to make it refresh, and CROSS_SESSION is not necessarily a ceiling")
	} else {
		t.Logf("ANSWER: NO event about this group arrived in 15s. Either the notification " +
			"does not reach this session, or it arrives on a channel this bus does not " +
			"listen to — the bus covers messages and chats, not group metadata, and that " +
			"distinction is the next thing to measure")
	}
	// Also report what the policy reads as HERE, to confirm the staleness the
	// investigation is about is still happening.
	now, err := m.PolicyOf(ctx, gjid, p, "xs/read-after")
	if err != nil {
		t.Fatalf("PolicyOf (after): %v", err)
	}
	t.Logf("MEASURED: in-session the policy reads %t; it was %t before the flip", now, was)
}

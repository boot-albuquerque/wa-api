package waheadless

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeParticipantsAreReallyCrossSession re-asks a question that was already
// answered, because the answer next to it turned out to be wrong.
//
// H58 measured a participant change staying invisible for ninety seconds and
// concluded "group metadata is stale in the session that changed it". H85 then
// showed that GROUP POLICIES — which live on the same metadata object — are
// visible in about a second, and that the classification saying otherwise came
// from a control that could not fail.
//
// H58's own measurement was valid: a real change, polled, confirmed across
// sessions. But it is now the only thing holding up a story that was too broad
// once, so it is worth re-running with the bus watching — which H58 did not
// have.
//
// The peer is removed and put back, each in its own session, exactly as the
// delivered capability does it.
func TestProbeParticipantsAreReallyCrossSession(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PARTS_XS") == "" {
		t.Skip("set WA_PROBE_PARTS_XS=1; this removes and re-adds the peer in the lab group")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	session := func(t *testing.T, what string, step func(context.Context, *engine.Runner, *core.Session, *group.Manager, string)) {
		t.Helper()
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		defer h.Stop(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		sess, err := h.Session(ctx)
		if err != nil {
			t.Fatalf("%s: boot: %v", what, err)
		}
		gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
		if gjid == "" {
			t.Skipf("%s: lab group not found", what)
		}
		step(ctx, runner, sess, group.New(runner, sess.Tab().Evaluate), gjid)
	}

	// The restore runs last, in its own session, whatever happens above.
	defer session(t, "restore", func(ctx context.Context, _ *engine.Runner, _ *core.Session, m *group.Manager, gjid string) {
		n, err := m.Count(ctx, gjid, "xs/count-restore")
		if err != nil {
			t.Errorf("Count: %v", err)
			return
		}
		if n >= 2 {
			t.Logf("restore: the group already has %d participants", n)
			return
		}
		back, err := m.AddParticipant(ctx, gjid, peer, "xs/restore")
		if err != nil {
			t.Errorf("RESTORE FAILED — the lab group is left with one member: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	})

	session(t, "remove", func(ctx context.Context, runner *engine.Runner, sess *core.Session, m *group.Manager, gjid string) {
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

		var mu sync.Mutex
		byType := map[events.Type]int{}
		aboutGroup := 0
		hub.Subscribe(func(e events.Event) {
			mu.Lock()
			defer mu.Unlock()
			byType[e.Type]++
			if e.ChatJID == gjid {
				aboutGroup++
			}
		})
		time.Sleep(3 * time.Second) // let the replay settle
		mu.Lock()
		byType = map[events.Type]int{}
		aboutGroup = 0
		mu.Unlock()

		before, err := m.Count(ctx, gjid, "xs/count-before")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if before != 2 {
			t.Skipf("the lab group has %d participants, not the 2 this probe needs", before)
		}

		start := time.Now()
		got, err := m.RemoveParticipant(ctx, gjid, peer, "xs/remove")
		if err != nil {
			t.Fatalf("RemoveParticipant: %v", err)
		}
		t.Logf("removed: %s", got)

		// THE SAME POLL THE POLICY GOT. If participants behave like policies,
		// this sees 1 within a second or two.
		seen := time.Duration(-1)
		for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
			n, err := m.Count(ctx, gjid, "xs/poll")
			if err == nil && n != before {
				seen = time.Since(start)
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		mu.Lock()
		t.Logf("MEASURED: %d event(s) about the group after the change; by type %v", aboutGroup, byType)
		mu.Unlock()
		if seen >= 0 {
			t.Logf("ANSWER: participants ARE visible in this session, after %s — H58's "+
				"conclusion needs the same correction H85 made for policies", seen.Round(100*time.Millisecond))
		} else {
			t.Log("ANSWER: participants stayed invisible for 90s in the session that " +
				"changed them. H58 stands, and the difference from policies — same " +
				"metadata object, different behaviour — is itself the finding")
		}
	})

	session(t, "confirm", func(ctx context.Context, _ *engine.Runner, _ *core.Session, m *group.Manager, gjid string) {
		n, err := m.Count(ctx, gjid, "xs/count-after")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		t.Logf("CROSS-SESSION: the group now has %d participant(s)", n)
	})
}

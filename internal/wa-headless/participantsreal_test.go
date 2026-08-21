package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPARemovesAndAddsTheLabPeer proves both directions on the LAB group,
// where the only member besides this account is the peer lab account.
//
// REMOVING SOMEBODY IS VISIBLE TO THEM, and adding them back does not erase the
// system notice the group keeps. So this is only ever run against the account
// that is ours, and the test puts the membership back before it finishes.
func TestRealSPARemovesAndAddsTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this removes and re-adds the peer lab account")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	m := group.New(runner, sess.Tab().Evaluate)
	g, err := m.Ensure(ctx, labGroupSubject, []string{peer}, "parts/ensure")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Logf("lab group: %s", g)

	// PUT THE PEER BACK NO MATTER WHAT HAPPENS BELOW. A failed run must not
	// leave the lab group with one member.
	defer func() {
		back, err := m.AddParticipant(context.Background(), g.JID, peer, "parts/restore")
		if err != nil {
			t.Errorf("restoring the peer: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	}()

	removed, err := m.RemoveParticipant(ctx, g.JID, peer, "parts/remove")
	if err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	t.Logf("removed: %s", removed)
	if !removed.Changed() {
		t.Fatalf("the membership did not change: %s", removed)
	}
	if removed.After != removed.Before-1 {
		t.Fatalf("removing one member took the count from %d to %d", removed.Before, removed.After)
	}

	added, err := m.AddParticipant(ctx, g.JID, peer, "parts/add")
	if err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}
	t.Logf("added: %s", added)
	if added.After != added.Before+1 {
		t.Fatalf("adding one member took the count from %d to %d", added.Before, added.After)
	}

	// ASKING AGAIN MUST BE A NO-OP, not a second addition and not a refusal —
	// the same shape as an already-archived chat (H55).
	again, err := m.AddParticipant(ctx, g.JID, peer, "parts/add-again")
	if err != nil {
		t.Fatalf("AddParticipant (already a member): %v", err)
	}
	if again.Changed() {
		t.Fatalf("adding an existing member changed the group: %s", again)
	}
}

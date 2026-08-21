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

// TestRealSPAPromotesAndDemotesTheLabPeer proves both directions, and the proof
// is the SECOND CALL'S NoOp FLAG.
//
// Session two reads fresh metadata. If the promotion in session one worked, the
// peer is an admin there, so the demotion is a real change and NoOp is false. If
// the promotion did nothing, the peer is still an ordinary member, the demotion
// finds the role it wants already in place, and NoOp is true. One boolean, and
// it cannot be satisfied by a call that quietly failed — which is exactly what
// H58 needed and did not have.
//
// LEAVING A GROUP IS NOT TESTED HERE AND WILL NOT BE. An account that leaves a
// group it created cannot rejoin without an invite from somebody still inside,
// and the lab has two accounts. That capability is unit tested only, and the
// omission is written down rather than quietly skipped.
func TestRealSPAPromotesAndDemotesTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this promotes and demotes the peer lab account")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	session := func(t *testing.T, what string, step func(context.Context, *group.Manager, string)) {
		t.Helper()
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		defer h.Stop(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sess, err := h.Session(ctx)
		if err != nil {
			t.Fatalf("%s: boot: %v", what, err)
		}
		gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
		if gjid == "" {
			t.Skipf("%s: lab group not found by subject", what)
		}
		step(ctx, group.New(runner, sess.Tab().Evaluate), gjid)
	}

	// Registered first, so it runs last: the peer must not be left an admin.
	defer session(t, "demote", func(ctx context.Context, m *group.Manager, gjid string) {
		back, err := m.Demote(ctx, gjid, peer, "admin/demote")
		if err != nil {
			t.Errorf("DEMOTE FAILED — the peer is left an admin of the lab group: %v", err)
			return
		}
		t.Logf("demoted: %s", back)
		if back.NoOp {
			t.Error("the demotion was a no-op, which means the promotion never took effect " +
				"— this is the cross-session check, and it says the promote did nothing")
		}
	})

	session(t, "promote", func(ctx context.Context, m *group.Manager, gjid string) {
		got, err := m.Promote(ctx, gjid, peer, "admin/promote")
		if err != nil {
			t.Fatalf("Promote: %v", err)
		}
		t.Logf("promoted: %s", got)
		if got.NoOp {
			t.Fatal("the peer was already an admin, so this run proves nothing; demote and retry")
		}
		if got.Verified {
			t.Fatal("a real admin change claims to be verified; this build cannot confirm one in-session")
		}
	})
}

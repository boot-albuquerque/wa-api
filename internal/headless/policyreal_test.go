package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAFlipsAGroupPolicyAcrossSessions proves one policy in both
// directions, across sessions, because this build does not refresh group
// metadata in the session that changed it (H58).
//
// It flips `announcement` — only admins may send — on the LAB group, whose only
// other member is the peer lab account. The restore runs from a defer in its own
// session: a lab group left open to everyone is a change to a real group's
// rules, even if the group is ours.
func TestRealSPAFlipsAGroupPolicyAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this changes the lab group's policy")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}

	session := func(t *testing.T, what string, step func(context.Context, *group.Manager, string)) {
		t.Helper()
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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

	const p = group.PolicyMessagesAdminsOnly
	var was bool

	// Registered first so it runs last, in its own session.
	defer session(t, "restore", func(ctx context.Context, m *group.Manager, gjid string) {
		back, err := m.SetPolicy(ctx, gjid, p, was, "policy/restore")
		if err != nil {
			t.Errorf("RESTORE FAILED — the lab group keeps a policy this test set: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	})

	session(t, "flip", func(ctx context.Context, m *group.Manager, gjid string) {
		cur, err := m.PolicyOf(ctx, gjid, p, "policy/read-before")
		if err != nil {
			t.Fatalf("PolicyOf: %v", err)
		}
		was = cur
		t.Logf("MEASURED: the lab group's %s is %t", p, cur)

		got, err := m.SetPolicy(ctx, gjid, p, !cur, "policy/flip")
		if err != nil {
			t.Fatalf("SetPolicy: %v", err)
		}
		t.Logf("flipped: %s", got)
		if got.NoOp {
			t.Fatal("flipping to the opposite of what was read was reported as a no-op")
		}
		// VERIFIED IS TRUE NOW (H85). This assertion used to demand the
		// opposite, on a belief built from a control that could not fail.
		if !got.Verified {
			t.Fatal("a real policy change is not reported as verified; it is visible in ~1s")
		}
	})

	session(t, "confirm", func(ctx context.Context, m *group.Manager, gjid string) {
		now, err := m.PolicyOf(ctx, gjid, p, "policy/read-after")
		if err != nil {
			t.Fatalf("PolicyOf: %v", err)
		}
		t.Logf("CROSS-SESSION: %s was %t before the flip and reads %t after", p, was, now)
		if now == was {
			t.Fatalf("the policy did not change: it reads %t, the same as before", now)
		}
	})
}

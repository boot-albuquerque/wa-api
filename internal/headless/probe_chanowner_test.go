package headless

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/channel"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeChannelOwnership creates a channel, edits it, and DELETES it.
//
// AUTHORISATION: creating a channel is an outward-facing act on a real account.
// It was authorised explicitly for the lab account, with the condition that the
// channel be removed at the end — so the delete is not a test step that may be
// skipped, it is the condition under which this runs at all.
//
// The removal is registered with defer and NOT t.Cleanup, because t.Cleanup runs
// after every defer, including the one that stops the session; a cleanup against
// a dead tab fails with "context canceled" and would leave a real channel
// standing on the account.
func TestProbeChannelOwnership(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANOWNER") == "" {
		t.Skip("set WA_PROBE_CHANOWNER=1 (this CREATES and DELETES a channel)")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	m := channel.NewManager(runner, eval)
	r := channel.New(runner, eval)

	// The name says what it is, in case a human ever sees it before the delete.
	name := fmt.Sprintf("wa-headless probe %d", time.Now().Unix())
	made, err := m.Create(ctx, name, "created by an automated parity probe", "probe/chanowner")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("created: %s", made)

	// THE DELETE IS THE CONDITION OF THE RUN, so it is registered immediately
	// after the create and before anything that can fail.
	defer func() {
		if err := m.Delete(ctx, made.JID, made.InviteCode, "probe/chanowner-delete"); err != nil {
			t.Errorf("DELETE FAILED: %v — a real channel is left standing on the "+
				"account and must be removed by hand", err)
			return
		}
		// And the delete's own postcondition, checked here too: the reader must
		// no longer find it.
		if _, err := r.ByInviteCode(ctx, made.InviteCode, "probe/chanowner-gone"); err == nil {
			t.Error("the channel is still readable after a delete that reported success")
		} else {
			t.Logf("deleted and confirmed gone (%v)", err)
		}
	}()

	if made.JID == "" || made.InviteCode == "" {
		t.Fatal("the create reported no identity, so nothing else can be exercised")
	}
	if made.CreatedAt.IsZero() {
		t.Error("the create reported no creation time")
	}

	// AND THE FOLLOW LIST MUST NOW HOLD IT. `Followed` is this module's
	// getChannels, and it answered 0 for the whole of Phase 1 because the account
	// follows nothing — a reader never seen returning anything is the H93 trap.
	// A channel this account OWNS is in the same collection, so creating one is
	// the honest way to prove the reader non-empty without subscribing to a
	// stranger's channel (which measured unreachable, H123).
	listed, err := m.Followed(ctx, "probe/chanowner")
	if err != nil {
		t.Errorf("Followed: %v", err)
	} else {
		t.Logf("Followed returned %d", len(listed))
		if len(listed) == 0 {
			t.Error("the channel collection is empty right after creating a channel")
		}
		for _, e := range listed {
			t.Logf("  %s", e)
			if e.Membership == "" {
				t.Error("an entry carries no membership")
			}
		}
	}

	// REACTION POLICY, on a channel this account owns. The setter goes through
	// the SAME edit action as the description, which H113 measured accepting and
	// never storing — so this is also a test of whether that failure is specific
	// to descriptions or general to the action.
	for _, want := range []channel.ReactionPolicy{
		channel.ReactionsNone, channel.ReactionsBasic, channel.ReactionsAll,
	} {
		wire, err := m.SetReactionPolicy(ctx, made.JID, made.InviteCode, want, "probe/chanowner")
		switch {
		case errors.Is(err, channel.ErrUnverifiable):
			// MEASURED, not asserted away: a fresh channel's metadata carries no
			// reaction mixin, so there is nothing to verify against. The write may
			// well have landed; nobody can say.
			t.Logf("reaction policy %d: unverifiable on a fresh channel (%v)", want, err)
		case err != nil:
			t.Errorf("SetReactionPolicy(%d): %v", want, err)
		default:
			t.Logf("reaction policy %d -> server wire %d", want, wire)
		}
	}

	// RENAME, verified against the server rather than against the echo.
	renamed := name + " (renamed)"
	if err := m.SetName(ctx, made.JID, made.InviteCode, renamed, "probe/chanowner"); err != nil {
		t.Errorf("SetName: %v", err)
	} else {
		t.Log("rename took and was read back from the server")
	}

	// DESCRIBE. The first run measured this FAILING while the rename succeeded,
	// so the question is whether the description simply propagates slower — and
	// that is measured here rather than fixed by adding a retry to production.
	const wantDesc = "second description"
	errDesc := m.SetDescription(ctx, made.JID, made.InviteCode, wantDesc, "probe/chanowner")
	t.Logf("SetDescription immediate verdict: %v", errDesc)
	settled := false
	for i := 0; i < 10; i++ {
		got, err := r.ByInviteCode(ctx, made.InviteCode, "probe/chanowner-desc")
		if err == nil && got.Description == wantDesc {
			t.Logf("the description appeared after %s", time.Duration(i+1)*2*time.Second)
			settled = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !settled {
		t.Logf("MEASURED: the description never appeared within 20s, while the rename " +
			"was readable back immediately. This is not latency.")
	}

	// AND THE EMPTY CASE PASSED FOR THE WRONG REASON on the first run: the server
	// description was ALREADY empty, so writing "" matched trivially. It is only
	// meaningful after a description actually exists.
	if settled {
		if err := m.SetDescription(ctx, made.JID, made.InviteCode, "", "probe/chanowner"); err != nil {
			t.Errorf("clearing a real description failed: %v", err)
		} else {
			t.Log("a real description was cleared and read back")
		}
	} else {
		t.Log("skipping the clear: with no description set, writing \"\" would pass " +
			"without proving anything")
	}

	// And the reader agrees about the final state.
	got, err := r.ByInviteCode(ctx, made.InviteCode, "probe/chanowner")
	if err != nil {
		t.Errorf("read back: %v", err)
	} else {
		t.Logf("final: %s", got)
		if got.Name != renamed {
			t.Error("the server does not report the renamed channel")
		}
		t.Logf("final description present = %t", got.Description != "")
	}

	// A rename of a channel that does not exist must not read as success.
	if err := m.SetName(ctx, "0000@newsletter", made.InviteCode, "x", "probe/chanowner"); err == nil {
		t.Error("renaming a channel that does not exist reported success")
	} else if errors.Is(err, context.Canceled) {
		t.Errorf("unexpected: %v", err)
	}
}

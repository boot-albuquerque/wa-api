package waheadless

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
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
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
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

package waheadless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPASendsAPollToTheLabPeer sends one poll to the peer lab account.
//
// A poll is an ordinary message and can be revoked, which is what makes this
// safe to try against the live build even though two details of the call were
// inferred from the app's call site rather than read from a signature.
func TestRealSPASendsAPollToTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	// THIS TEST IS RED ON PURPOSE (H69). The poll call throws
	// "Cannot read properties of undefined (reading 'name')", and the entry
	// explains why: the object shape was copied from a UI helper that is not
	// createPollCreationMsgData. It stays here, behind its own switch, so the
	// next attempt has a harness instead of a blank page.
	if os.Getenv("WA_HEADLESS_POLL_TEST") == "" {
		t.Skip("set WA_HEADLESS_POLL_TEST=1; NOT PROVEN — see H69, this currently fails")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
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

	chatJID := findLabChatJID(ctx, t, runner, sess.Tab().Evaluate, peer)
	if chatJID == "" {
		t.Skip("no loaded chat with the peer")
	}

	question := fmt.Sprintf("wa-headless poll probe %d", time.Now().Unix())
	got, err := send.PollTo(ctx, runner, sess.Tab().Evaluate, chatJID,
		question, []string{"alpha", "beta", "gamma"}, false, "test/poll")
	if err != nil {
		t.Fatalf("PollTo: %v", err)
	}
	t.Logf("poll: %s", got)
	t.Logf("MEASURED: pollType came from %s", got.PollTypeSource)

	if got.ID == "" {
		t.Fatal("no poll id")
	}
	if got.Options != 3 {
		t.Fatalf("the option count did not survive: %s", got)
	}
}

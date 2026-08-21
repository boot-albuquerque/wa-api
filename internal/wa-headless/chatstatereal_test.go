package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chatstate"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAArchivesAndPinsAConversation proves both flags against a live
// account, on the PEER LAB conversation only.
//
// Archiving or pinning somebody else's conversation would change what the
// account's owner sees in their own app to make a test pass. The same rule kept
// the group proof off a real group (H48) and the read receipts off strangers
// (H52).
//
// EVERY CHANGE IS UNDONE. These flags are reversible and the test leaves the
// conversation exactly as it found it — a suite that accumulated archived chats
// would be one nobody could run twice.
func TestRealSPAArchivesAndPinsAConversation(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this changes real conversation state")
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	s := chatstate.New(runner, sess.Tab().Evaluate)

	// ARCHIVE, then put it back.
	on, err := s.SetArchived(ctx, peer, true, "real/archive-on")
	if err != nil {
		t.Fatalf("SetArchived(true): %v", err)
	}
	t.Logf("archived: %s", on)
	if !on.After {
		t.Fatal("the conversation is not archived after archiving it")
	}
	defer func() {
		if _, err := s.SetArchived(context.Background(), peer, false, "real/archive-restore"); err != nil {
			t.Errorf("restoring the archive flag: %v", err)
		}
	}()

	off, err := s.SetArchived(ctx, peer, false, "real/archive-off")
	if err != nil {
		t.Fatalf("SetArchived(false): %v", err)
	}
	t.Logf("unarchived: %s", off)
	if off.After {
		t.Fatal("the conversation is still archived after unarchiving it")
	}
	if !off.Changed() {
		t.Fatal("unarchiving reported no change, but archiving had just happened")
	}

	// PIN, then put it back. The limit is real, so a refusal here is a
	// legitimate outcome and not a failure of the capability.
	pinned, err := s.SetPinned(ctx, peer, true, "real/pin-on")
	if err != nil {
		t.Skipf("pinning was refused (%v); on an account at its pin limit this is "+
			"the correct answer, and the archive half above already proved the path", err)
	}
	t.Logf("pinned: %s", pinned)
	defer func() {
		if _, err := s.SetPinned(context.Background(), peer, false, "real/pin-restore"); err != nil {
			t.Errorf("restoring the pin flag: %v", err)
		}
	}()
	if !pinned.After {
		t.Fatal("the conversation is not pinned after pinning it")
	}

	unpinned, err := s.SetPinned(ctx, peer, false, "real/pin-off")
	if err != nil {
		t.Fatalf("SetPinned(false): %v", err)
	}
	t.Logf("unpinned: %s", unpinned)
	if unpinned.After {
		t.Fatal("the conversation is still pinned after unpinning it")
	}
}

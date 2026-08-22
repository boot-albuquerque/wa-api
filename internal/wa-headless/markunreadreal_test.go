package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAMarksTheLabChatUnreadAcrossSessions proves the deliberate unread
// mark the only honest way this build allows.
//
// THE FIRST VERSION OF THIS TEST ASSERTED IN-SESSION AND WAS WRONG. It reported
// the lab chat at 22 unread before and after; a fresh session read the same chat
// as 0. The count a session sees does not move when that session changes it —
// the same wall H58 hit with group metadata, found here in a second place.
//
// So each step gets its own browser session, and the restore gets one too.
func TestRealSPAMarksTheLabChatUnreadAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	// RED ON PURPOSE (H78), behind its own switch so the read-test suite stays
	// green. Two primitives were measured and neither marks the chat; the entry
	// explains what each one did and why the next attempt needs a fifth idea.
	if os.Getenv("WA_HEADLESS_MARKUNREAD_TEST") == "" {
		t.Skip("set WA_HEADLESS_MARKUNREAD_TEST=1; NOT PROVEN — see H78, this currently fails")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	session := func(t *testing.T, what string, step func(context.Context, *chats.Lister, string)) {
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
		jid := findLabChatJID(ctx, t, runner, sess.Tab().Evaluate, peer)
		if jid == "" {
			t.Skipf("%s: no loaded chat with the peer", what)
		}
		step(ctx, chats.New(runner, sess.Tab().Evaluate), jid)
	}

	// Registered first so it runs last, in its own session: a chat left with a
	// deliberate blue dot is something a person has to clear by hand.
	defer session(t, "restore", func(ctx context.Context, l *chats.Lister, jid string) {
		back, err := l.MarkRead(ctx, jid, "test/unread-restore")
		if err != nil {
			t.Errorf("RESTORE FAILED — the lab chat is left marked unread: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	})

	var before int
	session(t, "mark", func(ctx context.Context, l *chats.Lister, jid string) {
		n, err := l.UnreadCount(ctx, jid, "test/count-before")
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		before = n
		if n < 0 {
			t.Fatalf("the lab chat is already marked unread (count %d); this run would prove nothing", n)
		}
		got, err := l.MarkUnread(ctx, jid, "test/unread")
		if err != nil {
			t.Fatalf("MarkUnread: %v", err)
		}
		t.Logf("marked: %s", got)
		if got.Verified {
			t.Fatal("a real unread mark claims to be verified; this build cannot confirm one in-session")
		}
	})

	session(t, "confirm", func(ctx context.Context, l *chats.Lister, jid string) {
		n, err := l.UnreadCount(ctx, jid, "test/count-after")
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		t.Logf("CROSS-SESSION: unread was %d before the mark and reads %d after", before, n)
		t.Logf("MEASURED: the deliberate mark reads as unreadCount=%d on this build", n)
		if n >= 0 {
			t.Fatalf("unreadDelta=-1 did not produce a deliberate mark: the count reads %d", n)
		}
	})
}

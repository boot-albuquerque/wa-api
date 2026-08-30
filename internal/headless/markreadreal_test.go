package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/chats"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAMarksAConversationRead proves the acknowledgement, and it is
// deliberately narrow about WHICH conversation.
//
// MARKING READ IS AN OUTWARD EFFECT ON SOMEBODY ELSE: it sends a read receipt,
// and the other person's client shows it. The announcing account here has 122
// unread conversations with real people, and acknowledging any of them would
// change what a stranger sees to make a test pass. So the target is the peer
// LAB account and nothing else — the same rule that kept the group proof from
// messaging a real group (H48).
func TestRealSPAMarksAConversationRead(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set HEADLESS_SEND_TEST=1; this sends a real read receipt to the peer lab account")
	}
	profile := os.Getenv("WA_MARKREAD_PROFILE")
	peer := os.Getenv("WA_MARKREAD_PEER")
	if profile == "" || peer == "" {
		t.Skip("WA_MARKREAD_PROFILE and WA_MARKREAD_PEER are required, and both must " +
			"be lab accounts: this acknowledges messages and the sender is told")
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

	l := chats.New(runner, sess.Tab().Evaluate)

	// CREATE SOMETHING TO ACKNOWLEDGE, because the first version of this test
	// passed with before=0 — the conversation happened to be already read, so
	// the acknowledgement path was never exercised and only the "nothing to do"
	// branch was proven. A green run that touches neither the call nor the
	// postcondition is the false positive this module keeps meeting.
	//
	// The peer lab account sends one message here, so the unread this test
	// clears is one the test itself caused.
	sender := os.Getenv("WA_MARKREAD_SENDER_PROFILE")
	self := os.Getenv("WA_MARKREAD_SELF_JID")
	if sender == "" || self == "" {
		t.Skip("WA_MARKREAD_SENDER_PROFILE and WA_MARKREAD_SELF_JID are required so " +
			"the test can create the unread it acknowledges")
	}
	sendRunner := engine.NewRunner()
	hs := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: sender, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: sendRunner,
	})
	defer hs.Stop(context.Background())
	sctx, scancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	ssess, err := hs.Session(sctx)
	scancel()
	if err != nil {
		t.Fatalf("boot sender: %v", err)
	}
	sendCtx, cancelSend := context.WithTimeout(context.Background(), 5*time.Minute)
	if _, err := send.Text(sendCtx, sendRunner, ssess.Tab().Evaluate, self,
		"headless: mensagem de teste para marcar como lida", "real/markread/send"); err != nil {
		cancelSend()
		t.Fatalf("seeding the unread: %v", err)
	}
	cancelSend()

	// Wait for it to land as UNREAD on this side. Polling the capability's own
	// listing, so the wait is on the fact the test needs and not on a clock.
	unreadDeadline := time.Now().Add(90 * time.Second)
	seeded := false
	for time.Now().Before(unreadDeadline) {
		list, err := l.List(ctx, 500, "real/markread/wait")
		if err != nil {
			t.Fatalf("List while waiting: %v", err)
		}
		for _, c := range list.Chats {
			if c.JID == peer && c.Unread > 0 {
				seeded = true
			}
		}
		if seeded {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if !seeded {
		t.Skip("the seeded message never arrived as unread within 90s; without an " +
			"unread conversation this test would only exercise the do-nothing branch")
	}

	got, err := l.MarkRead(ctx, peer, "real/markread")
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	t.Logf("%s", got)
	// THE PATH THAT MATTERS. before=0 would mean the acknowledgement never ran.
	if got.Before == 0 {
		t.Fatal("the conversation had nothing unread, so the acknowledgement was " +
			"never exercised — this run proves only the do-nothing branch")
	}
	if !got.Changed() {
		t.Fatalf("nothing changed: %s", got)
	}

	// The postcondition, restated here because the capability could in
	// principle report success without it being true.
	if got.After != 0 {
		t.Fatalf("the conversation still has %d unread after being marked", got.After)
	}

	// AND IT MUST STICK. A count that returns to zero and then climbs back
	// would mean the acknowledgement was local only — the sort of thing that
	// looks correct in the same breath and is wrong a second later.
	again, err := l.MarkRead(ctx, peer, "real/markread-again")
	if err != nil {
		t.Fatalf("MarkRead (second call): %v", err)
	}
	if again.Before != 0 {
		t.Fatalf("the conversation was unread again on the second call (before=%d): "+
			"the acknowledgement did not stick", again.Before)
	}
	if again.Changed() {
		t.Fatalf("the second call changed something: %s", again)
	}
	t.Logf("second call confirmed it stuck: %s", again)

	// The listing must agree with the capability: two sources for one fact is
	// how the contact roster's double-counting hid (H39).
	list, err := l.List(ctx, 500, "real/markread/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, c := range list.Chats {
		if c.JID == peer && c.Unread != 0 {
			t.Fatalf("the listing still reports %d unread for the acknowledged "+
				"conversation", c.Unread)
		}
	}
	t.Logf("listing agrees: %s", list)
}

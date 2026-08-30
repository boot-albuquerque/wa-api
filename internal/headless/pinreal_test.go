package headless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/pin"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAPinsAndUnpinsAMessageAcrossSessions proves pinning a MESSAGE in
// both directions, across sessions.
//
// A pinned message is visible to everyone in the conversation, so this pins one
// this account just sent to the peer lab account, and takes it down from a defer
// in its own session.
//
// CROSS-SESSION, for the reason H58 measured twice: the collection that would
// confirm the pin does not reflect a change the same session made.
func TestRealSPAPinsAndUnpinsAMessageAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	// RED ON PURPOSE (H81), behind its own switch. The call is accepted and
	// nothing is pinned; the entry carries both enums, the duration and the
	// caller's shape so the next attempt does not re-measure them.
	if os.Getenv("WA_HEADLESS_PIN_TEST") == "" {
		t.Skip("set HEADLESS_PIN_TEST=1; NOT PROVEN — see H81, this currently fails")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	session := func(t *testing.T, what string, step func(context.Context, *engine.Runner, func(context.Context, string, *string) error, *pin.Pinner, string)) {
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
		step(ctx, runner, sess.Tab().Evaluate, pin.New(runner, sess.Tab().Evaluate), jid)
	}

	var msgID string
	var before int

	// Registered first so it runs last, in its own session: a message left
	// pinned sits at the top of the peer's conversation for a week.
	defer session(t, "unpin", func(ctx context.Context, _ *engine.Runner, _ func(context.Context, string, *string) error, p *pin.Pinner, jid string) {
		if msgID == "" {
			return
		}
		back, err := p.Unpin(ctx, msgID, "test/unpin")
		if err != nil {
			t.Errorf("UNPIN FAILED — a message is left pinned in the lab chat: %v", err)
			return
		}
		t.Logf("unpinned: %s", back)
	})

	session(t, "pin", func(ctx context.Context, runner *engine.Runner, eval func(context.Context, string, *string) error, p *pin.Pinner, jid string) {
		list, err := p.PinnedIn(ctx, jid, "test/pinned-before")
		if err != nil {
			t.Fatalf("PinnedIn: %v", err)
		}
		before = len(list)
		t.Logf("MEASURED: the lab chat starts with %d pinned message(s)", before)

		sent, err := send.Text(ctx, runner, eval, peer,
			fmt.Sprintf("headless pin probe %d", time.Now().UnixNano()), "test/pin-send")
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		msgID = sent.ID.ID

		got, err := p.Message(ctx, msgID, "test/pin")
		if err != nil {
			t.Fatalf("Message: %v", err)
		}
		t.Logf("pinned: %s", got)
		t.Logf("MEASURED: this build's default pin lasts %d seconds", got.Seconds)
		if got.Verified {
			t.Fatal("a real pin claims to be verified; this build cannot confirm one in-session")
		}
		if got.Seconds <= 0 {
			t.Fatalf("the pin was sent with no duration: %s", got)
		}
	})

	session(t, "confirm", func(ctx context.Context, _ *engine.Runner, _ func(context.Context, string, *string) error, p *pin.Pinner, jid string) {
		list, err := p.PinnedIn(ctx, jid, "test/pinned-after")
		if err != nil {
			t.Fatalf("PinnedIn: %v", err)
		}
		t.Logf("CROSS-SESSION: %d pinned before, %d after", before, len(list))
		found := false
		for _, id := range list {
			if id == msgID {
				found = true
			}
		}
		if !found {
			t.Fatalf("the pinned message is not in the chat's pinned list (%d there)", len(list))
		}
	})
}

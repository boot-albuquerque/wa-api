package headless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/capabilities/star"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAStarsAndUnstarsItsOwnMessage proves both directions live.
//
// Starring is LOCAL — the peer never sees the flag — so this is the least
// intrusive live proof in the module. It still sends its own message rather
// than borrowing one, so the run leaves the account exactly as it found it: a
// message that was starred and unstarred is indistinguishable from one that was
// never touched.
func TestRealSPAStarsAndUnstarsItsOwnMessage(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_STAR_TEST") == "" {
		t.Skip("set WA_HEADLESS_STAR_TEST=1; this sends a message to the peer lab account")
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

	sent, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("wa-headless star probe %d", time.Now().UnixNano()), "test/star-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	s := star.New(runner, sess.Tab().Evaluate)

	// Registered before the star, so a failed assertion still leaves the flag
	// where it was found.
	defer func() {
		back, err := s.Unstar(context.Background(), sent.ID.ID, "test/unstar")
		if err != nil {
			t.Errorf("UNSTAR FAILED — the message is left starred: %v", err)
			return
		}
		t.Logf("restored: %s", back)
		if back.After {
			t.Errorf("the flag is still set after unstarring: %s", back)
		}
	}()

	got, err := s.Star(ctx, sent.ID.ID, "test/star")
	if err != nil {
		t.Fatalf("Star: %v", err)
	}
	t.Logf("starred: %s", got)
	if got.Before || !got.After || !got.Changed() {
		t.Fatalf("the flag did not move from false to true: %s", got)
	}

	again, err := s.Star(ctx, sent.ID.ID, "test/star-again")
	if err != nil {
		t.Fatalf("starring an already-starred message returned an error: %v", err)
	}
	if !again.AlreadyInState || again.Changed() {
		t.Fatalf("a redundant star was not reported as a no-op: %s", again)
	}
}

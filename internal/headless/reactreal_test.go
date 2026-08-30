package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/react"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAReactsToItsOwnMessage establishes the postcondition that could not
// be copied.
//
// Measured before this existed: of 395 messages on the lab account, ZERO
// carried a reaction and the ReactionsCollection was empty. There was no case
// to learn the shape from, so this test CREATES one — it sends a message to the
// peer lab account and reacts to it — and the first run is what decides whether
// hasReaction is the field that moves.
//
// It reacts to a message this account just sent, so nothing is done to anybody
// else's message.
func TestRealSPAReactsToItsOwnMessage(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set HEADLESS_SEND_TEST=1; this sends a real message and reacts to it")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	toJID := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || toJID == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	sent, err := send.Text(ctx, runner, eval, toJID,
		"headless: mensagem de teste para reagir", "react/seed")
	if err != nil {
		t.Fatalf("seeding a message to react to: %v", err)
	}
	t.Logf("seeded: %s", sent)

	r := react.New(runner, eval)

	got, err := r.Add(ctx, sent.ID.ID, "\U0001F44D", "react/add")
	if err != nil {
		t.Fatalf("Add: %v — if this is ErrUnreacted, hasReaction is not the field "+
			"that moves and the postcondition needs remeasuring, not relaxing", err)
	}
	t.Logf("reacted: %s", got)
	if got.Had {
		t.Fatal("the message already carried a reaction before this test touched it; " +
			"the run cannot attribute what it sees")
	}
	if !got.Has {
		t.Fatal("Add reported success with no reaction on the message")
	}

	// AND IT MUST COME OFF. A capability that can only add would leave every
	// test run marking a real conversation.
	cleared, err := r.Remove(ctx, sent.ID.ID, "react/remove")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	t.Logf("removed: %s", cleared)
	if !cleared.Had {
		t.Fatal("Remove says the message carried no reaction, but Add had just put one on")
	}
	// REMOVAL IS NOT VERIFIED HERE, and the Result says so rather than this
	// test pretending otherwise. Measured three times from FRESH sessions: after
	// a removal the account shows hasReaction false everywhere and ZERO rows in
	// the ReactionsCollection — the removal works. What does not work is
	// checking it from the session that did it, where hasReaction stays true
	// for at least 30 seconds.
	//
	// So the assertion is on the CONTRACT: a removal must never claim to have
	// been verified.
	if cleared.Verified {
		t.Fatal("Remove reported Verified=true; this build does not reflect a " +
			"removal in the session that performed it, so nothing was checked")
	}
	if !got.Verified {
		t.Fatal("Add reported Verified=false; adding IS observable and must be checked")
	}
}

package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/block"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPABlocksAndUnblocksTheLabPeer proves both directions against the
// live build, on the peer LAB account and nobody else.
//
// BLOCKING IS VISIBLE TO THE OTHER SIDE — they stop being able to reach this
// account — so the unblock runs from a defer and runs even when the block half
// fails. The measured baseline is a blocklist of size 0 (probe_block_test.go),
// which is what makes 0 -> 1 -> 0 a proof rather than a coincidence.
func TestRealSPABlocksAndUnblocksTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_BLOCK_TEST") == "" {
		t.Skip("set WA_HEADLESS_BLOCK_TEST=1; this blocks and unblocks the peer lab account")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	b := block.New(runner, sess.Tab().Evaluate)

	// The restore is registered BEFORE the block, so an assertion that fails in
	// between still leaves the peer able to reach this account.
	defer func() {
		back, err := b.Unblock(context.Background(), peer, "test/unblock")
		if err != nil {
			t.Errorf("UNBLOCK FAILED — the peer is still blocked and needs a human: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	}()

	got, err := b.Block(ctx, peer, "test/block")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	t.Logf("blocked: %s", got)
	if got.AlreadyInState {
		t.Fatal("the peer was already blocked, so this run proved nothing; unblock and retry")
	}
	if got.After != got.Before+1 {
		t.Fatalf("the blocklist did not grow by exactly one: %s", got)
	}

	// Asking twice is the no-op path, and it has to be a SUCCESS.
	again, err := b.Block(ctx, peer, "test/block-again")
	if err != nil {
		t.Fatalf("blocking an already-blocked contact returned an error: %v", err)
	}
	if !again.AlreadyInState || again.Changed() {
		t.Fatalf("a redundant block was not reported as a no-op: %s", again)
	}
}

// TestRealSPARefusesToBlockAGroup proves the refusal costs no page call.
func TestRealSPARefusesToBlockAGroup(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_BLOCK_TEST") == "" {
		t.Skip("set WA_HEADLESS_BLOCK_TEST=1")
	}
	b := block.New(engine.NewRunner(), func(context.Context, string, *string) error {
		t.Fatal("the page was asked to block a group")
		return nil
	})
	if _, err := b.Block(context.Background(), "120363000000000000@g.us", "t"); !errors.Is(err, block.ErrGroup) {
		t.Fatalf("got %v, want ErrGroup", err)
	}
	_ = time.Second
}

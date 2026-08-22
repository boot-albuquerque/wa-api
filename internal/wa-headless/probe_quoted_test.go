package waheadless

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeQuotedOf proves message.QuotedOf by PRODUCING the thing it reads.
//
// Measured first: 336 loaded messages, ZERO carrying a quoted id. Shipping a
// reader never seen returning anything is the H93 trap, and the way out is the
// one that closed GROUP_UPDATE (H119) and VOTE_UPDATE (H121) — make the fact
// happen, then read it.
//
// It SENDS a reply to the lab peer, which is an outward effect confined to the
// authorised pair and the same thing send.Reply's own proof does.
func TestProbeQuotedOf(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_QUOTEDOF") == "" {
		t.Skip("set WA_PROBE_QUOTEDOF=1 (this SENDS a reply)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := strings.TrimSpace(os.Getenv("WA_SEND_TO_JID"))
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
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
	r := message.New(runner, eval)

	// FIRST: an ordinary message must report NO quote. Without this leg, a reader
	// that reported "quotes" for everything would pass the second leg.
	first, err := send.Text(ctx, runner, eval, peer,
		fmt.Sprintf("wa-headless quoted probe target %d", time.Now().Unix()), "probe/quotedof")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	plain, err := r.QuotedOf(ctx, first.ID.ID, "probe/quotedof")
	if err != nil {
		t.Fatalf("QuotedOf(plain): %v", err)
	}
	t.Logf("plain message -> %s", plain)
	if plain.Quotes {
		t.Error("an ordinary message reported that it quotes something")
	}

	// THEN: a reply must report the quote, and point at the right message.
	reply, err := send.Reply(ctx, runner, eval, peer, first.ID.ID,
		"wa-headless quoted probe reply", "probe/quotedof")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	got, err := r.QuotedOf(ctx, reply.ID.ID, "probe/quotedof")
	if err != nil {
		t.Fatalf("QuotedOf(reply): %v", err)
	}
	t.Logf("reply -> %s", got)
	if !got.Quotes {
		t.Fatal("a reply reported that it quotes nothing")
	}
	if got.MessageID != first.ID.ID {
		t.Error("the reply points at a different message than the one it replied to")
	}
	if !got.Loaded {
		t.Error("the quoted message is in this session and was reported unloaded")
	}

	// AND THE ID IS USABLE: the whole point of returning an id is that the other
	// readers take it.
	origin, err := r.OriginOf(ctx, got.MessageID, "probe/quotedof")
	if err != nil {
		t.Errorf("the quoted id is not usable by OriginOf: %v", err)
	} else {
		t.Logf("quoted message resolved: %s", origin)
	}
}

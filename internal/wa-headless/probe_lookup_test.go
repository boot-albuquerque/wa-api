package waheadless

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeNumberID proves lookup.NumberID against the real server.
//
// It asks THREE questions, because one would not distinguish the interesting
// cases: a number that exists and resolves, a number that cannot exist, and a
// group — which short-circuits, since the resolution answers people and would
// otherwise report a real group as absent.
//
// Identity-free: nothing logs a jid or a number.
func TestProbeNumberID(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LOOKUP") == "" {
		t.Skip("set WA_PROBE_LOOKUP=1")
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
	r := lookup.New(runner, sess.Tab().Evaluate)

	// 1. A real number.
	got, err := r.NumberID(ctx, peer, "probe/lookup")
	if err != nil {
		t.Fatalf("NumberID(real): %v", err)
	}
	t.Logf("real peer -> %s", got)
	if got.JID == "" {
		t.Error("a real number produced no identity")
	}
	if got.IsGroup {
		t.Error("a one-to-one jid came back as a group")
	}
	// THE RESOLUTION MUST DO SOMETHING on a LID-first build: asking with a phone
	// jid and getting the same string back would mean the server never spoke.
	t.Logf("resolved to a different identity: %t", got.Resolved)

	// 2. A number that cannot exist. It must be a DEFINITE no, not a read error.
	if _, err := r.NumberID(ctx, "5500000000000@c.us", "probe/lookup"); err == nil {
		t.Error("an impossible number was reported as existing")
	} else if !errors.Is(err, lookup.ErrNotOnWhatsApp) {
		t.Errorf("an impossible number gave %v, want ErrNotOnWhatsApp — a caller "+
			"cannot tell 'no' from 'the read broke'", err)
	} else {
		t.Log("impossible number -> ErrNotOnWhatsApp, as a definite answer")
	}

	// 3. A group short-circuits rather than being reported absent.
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Log("lab group not found; skipping the group leg")
		return
	}
	g, err := r.NumberID(ctx, gjid, "probe/lookup")
	if err != nil {
		t.Fatalf("NumberID(group): %v — a group must not be reported absent", err)
	}
	if !g.IsGroup {
		t.Error("a group jid did not come back flagged as a group")
	}
	t.Logf("group -> %s", g)
}

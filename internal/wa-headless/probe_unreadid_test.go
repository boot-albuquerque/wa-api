package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeUnreadIdentity checks whether two readers in the SAME package answer
// differently about the same conversation depending on which identity they are
// handed.
//
// MarkUnread refused the phone jid with "no such conversation" while accepting
// the resolved one (observed in H150's run). ByJID was built to match either
// identity. If both are true, the package contradicts itself, and the error a
// caller sees says the conversation does not exist when it does.
func TestProbeUnreadIdentity(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_UNREADID") == "" {
		t.Skip("set WA_PROBE_UNREADID=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/unreadid")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	l := chats.New(runner, eval)
	for _, probe := range []struct{ what, jid string }{
		{"asked-for", peer}, {"resolved", ident.JID},
	} {
		c, err := l.ByJID(ctx, probe.jid, "probe/unreadid/byjid-"+probe.what)
		t.Logf("ByJID(%s):      found=%t err=%v", probe.what, err == nil, err)
		_ = c
		m, err := l.MarkUnread(ctx, probe.jid, "probe/unreadid/mark-"+probe.what)
		t.Logf("MarkUnread(%s): %s err=%v", probe.what, m, err)
	}
}

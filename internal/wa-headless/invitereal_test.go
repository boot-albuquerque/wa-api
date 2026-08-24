package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAReadsAndRevokesTheLabGroupInvite proves the invite code against
// the LAB group only.
//
// An invite code is a credential: anyone holding it can join. Reading one from
// a real group would put a joinable link for other people's conversation into
// this process, and revoking one would lock out whoever is relying on it. The
// lab group has two members and both are ours.
//
// NO CODE IS EVER LOGGED — only whether one came back and how long it is.
func TestRealSPAReadsAndRevokesTheLabGroupInvite(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this reads and REVOKES the lab group's invite")
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
	eval := sess.Tab().Evaluate

	m := group.New(runner, eval)
	g, err := m.Ensure(ctx, labGroupSubject, []string{peer}, "invite/ensure")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Logf("lab group: %s", g)

	first, err := m.InviteCode(ctx, g.JID, "invite/read")
	if err != nil {
		t.Fatalf("InviteCode: %v — if this is the iAmAdmin error, the metadata "+
			"query before it did not take effect", err)
	}
	t.Logf("read: %s", first)
	if first.Code == "" {
		t.Fatal("no code came back")
	}
	if first.Revoked {
		t.Fatal("reading a code reported itself as a revocation")
	}
	if !strings.HasPrefix(first.Link(), "https://chat.whatsapp.com/") {
		t.Fatal("the link is not a joinable whatsapp url")
	}

	// READING TWICE MUST GIVE THE SAME CODE. A read that quietly rotated the
	// code would lock people out every time somebody asked what the link is.
	again, err := m.InviteCode(ctx, g.JID, "invite/read-again")
	if err != nil {
		t.Fatalf("InviteCode (second): %v", err)
	}
	if again.Code != first.Code {
		t.Fatal("reading the invite twice produced different codes; a read must not " +
			"rotate the credential")
	}

	// REVOKING MUST CHANGE IT. That is the whole point, and a revocation that
	// returned the same code would be a lock that did not turn.
	rotated, err := m.RevokeInvite(ctx, g.JID, "invite/revoke")
	if err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	t.Logf("revoked: %s", rotated)
	if !rotated.Revoked {
		t.Fatal("the revocation did not report itself as one")
	}
	if rotated.Code == "" {
		t.Fatal("revoking left the group with no code at all")
	}
	if rotated.Code == first.Code {
		t.Fatal("the code did not change after revoking; anyone holding the old " +
			"link would still be able to join")
	}
}

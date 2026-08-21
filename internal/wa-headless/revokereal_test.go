package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/revoke"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPARevokesItsOwnMessage proves deleting for everyone, and it deletes
// ONLY a message this test just sent.
//
// This is the first destructive capability in the module: there is no undo, and
// the effect lands on somebody else's phone. So the target is a message created
// seconds earlier by this account, to the peer LAB account — never anything
// that existed before the test ran.
//
// The postcondition field was not known in advance and is checked by the
// capability against three candidates (isRevokedMsg, type === 'revoked',
// revokeSender); the first run is what says which one this build uses.
func TestRealSPARevokesItsOwnMessage(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this sends a real message and deletes it for everyone")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	toJID := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || toJID == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
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
		"wa-headless: mensagem que sera apagada pelo teste", "revoke/seed")
	if err != nil {
		t.Fatalf("seeding a message to revoke: %v", err)
	}
	t.Logf("seeded: %s", sent)

	got, err := revoke.New(runner, eval).ForEveryone(ctx, sent.ID.ID, false, "revoke/do")
	if err != nil {
		t.Fatalf("ForEveryone: %v — if this is ErrStillThere, the deletion may have "+
			"worked while none of the three candidate fields reflects it, and the "+
			"postcondition needs remeasuring rather than relaxing", err)
	}
	t.Logf("revoked: %s", got)

	// The account's OWN message, seconds old, must be revocable as the sender.
	// Coming back as admin would mean the entitlement check picked the wrong
	// path for a message this account plainly wrote.
	if got.As != "sender" {
		t.Fatalf("revoked as %q; a message this account sent seconds ago is the "+
			"sender's to delete", got.As)
	}
}

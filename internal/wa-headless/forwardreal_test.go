package waheadless

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/forward"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAForwardsItsOwnMessage proves the forward against the live build.
//
// IT FORWARDS A MESSAGE IT JUST SENT, back into the SAME lab chat. That is
// deliberate: forwarding somebody else's content to a third party is the one
// version of this act with a real privacy cost, and the lab has exactly two
// accounts. The peer receives one extra copy of a string this test wrote.
//
// There is no undo for a forward and none is attempted; the copy is an ordinary
// message, and revoke is the capability that removes one.
func TestRealSPAForwardsItsOwnMessage(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_FORWARD_TEST") == "" {
		t.Skip("set WA_HEADLESS_FORWARD_TEST=1; this sends two messages to the peer lab account")
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

	body := fmt.Sprintf("wa-headless forward probe %d", time.Now().UnixNano())
	sent, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer, body, "test/fwd-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent: %s", sent)

	chatJID := findLabChatJID(ctx, t, runner, sess.Tab().Evaluate, peer)
	if chatJID == "" {
		t.Skip("no loaded chat with the peer")
	}

	f := forward.New(runner, sess.Tab().Evaluate)
	got, err := f.To(ctx, sent.ID.ID, chatJID, true, "test/fwd")
	if err != nil {
		t.Fatalf("To: %v", err)
	}
	t.Logf("forwarded: %s", got)

	// THE COPY IS A DIFFERENT MESSAGE. If the postcondition ever returns the
	// source id, it has proved nothing at all — that is the failure mode a
	// "some message appeared" check has.
	if got.NewID == sent.ID.ID {
		t.Fatalf("the forward reported the SOURCE message as the copy: %s", got)
	}
	if got.NewID == "" {
		t.Fatalf("no copy id: %s", got)
	}
	if got.BodyLen != utf16Len(body) {
		t.Errorf("the copy's body length is %d and the original's is %d", got.BodyLen, utf16Len(body))
	}

	// A chat that is not loaded must be refused rather than created.
	if _, err := f.To(ctx, sent.ID.ID, "15550009999@c.us", true, "test/fwd-nochat"); err == nil {
		t.Fatal("forwarding to an unknown chat succeeded; it should have been refused")
	}
}

// findLabChatJID resolves the peer's loaded chat SYNCHRONOUSLY, because
// engine.Tab.Evaluate does not await promises.
func findLabChatJID(ctx context.Context, t *testing.T, runner *engine.Runner,
	eval func(context.Context, string, *string) error, peer string) string {
	t.Helper()
	user := peer
	if i := strings.IndexByte(peer, '@'); i >= 0 {
		user = peer[:i]
	}
	var jid string
	script := `(() => {
		const want = ` + strconv.Quote(user) + `;
		const CC = window.require('WAWebChatCollection').ChatCollection;
		for (const c of CC.getModelsArray()) {
			try {
				if (!c || !c.id || c.id.server === 'g.us') { continue; }
				const ids = [c.id, c.contact && c.contact.id, c.contact && c.contact.phoneNumber];
				for (const i of ids) {
					if (i && i.user === want && c.id._serialized) { return c.id._serialized; }
				}
			} catch (e) {}
		}
		return '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/find-chat", func(c context.Context) error {
		return eval(c, script, &jid)
	}); err != nil {
		t.Fatalf("finding the lab chat: %v", err)
	}
	return jid
}

package headless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/mute"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPAMutesAndUnmutesTheLabChat proves both directions on the chat with
// the peer lab account.
//
// Muting is a NOTIFICATION setting on this account's own device list — the peer
// never learns of it. The restore runs from a defer registered before the mute,
// so a failed assertion cannot leave the lab chat silenced.
func TestRealSPAMutesAndUnmutesTheLabChat(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_MUTE_TEST") == "" {
		t.Skip("set WA_HEADLESS_MUTE_TEST=1; this mutes and unmutes the lab chat")
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

	// The chat's own jid on this build, resolved the way every other capability
	// resolves one — the lab chat may be keyed by lid rather than the phone jid.
	var chatJID string
	// SYNCHRONOUS on purpose. engine.Tab.Evaluate does not await promises, so
	// the async identity resolution the other capabilities use cannot be called
	// from here without the store-and-poll dance. Matching the peer's user part
	// against the loaded chats answers the same question without a promise.
	find := `(() => {
		const want = ` + quoteForTest(peerUser(peer)) + `;
		const CC = window.require('WAWebChatCollection').ChatCollection;
		for (const c of CC.getModelsArray()) {
			try {
				if (!c || !c.id || c.id.server === 'g.us') { continue; }
				const ids = [c.id, c.contact && c.contact.id,
					c.contact && c.contact.phoneNumber];
				for (const i of ids) {
					if (i && i.user === want && c.id._serialized) { return c.id._serialized; }
				}
			} catch (e) {}
		}
		return '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/mute-find", func(c context.Context) error {
		return sess.Tab().Evaluate(c, find, &chatJID)
	}); err != nil {
		t.Fatalf("resolving the lab chat: %v", err)
	}
	if chatJID == "" {
		t.Skip("no loaded chat with the peer; nothing to mute")
	}

	m := mute.New(runner, sess.Tab().Evaluate)

	defer func() {
		back, err := m.Off(context.Background(), chatJID, "test/unmute")
		if err != nil {
			t.Errorf("UNMUTE FAILED — the lab chat is left silenced: %v", err)
			return
		}
		t.Logf("restored: %s", back)
		if back.Muted() {
			t.Errorf("the chat is still muted after unmuting: %s", back)
		}
	}()

	got, err := m.For(ctx, chatJID, 8, "test/mute")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	t.Logf("muted: %s", got)
	if !got.Muted() || !got.Changed() {
		t.Fatalf("the expiration did not move to a muted value: %s", got)
	}
	if got.Before != 0 {
		t.Fatalf("the chat was already muted before this run, so it proved nothing: %s", got)
	}

	again, err := m.For(ctx, chatJID, 8, "test/mute-again")
	if err != nil {
		t.Fatalf("muting for the same duration returned an error: %v", err)
	}
	// The second call recomputes the expiry from the current clock, so it is a
	// genuine change rather than a no-op unless the two land in the same
	// second. Both outcomes are correct; what must not happen is an error.
	t.Logf("second mute: %s", again)
}

// quoteForTest keeps script literals quoted the same way production quotes them.
func quoteForTest(s string) string { return strconv.Quote(s) }

// peerUser is the local part of a jid. The live tests carry a phone jid in the
// environment and the loaded chat may be keyed by lid, so the USER part is what
// the two have in common on this build.
func peerUser(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

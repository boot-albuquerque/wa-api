package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPARepliesToAMessage is LOAD-BEARING rather than confirmatory.
//
// Every other call shape in the send package was read from the app's own code.
// This one rests on an inference: createTextMsgData takes an options object,
// quotedMsg is the field a reply carries, and sendTextMsgToChat forwards its
// third argument — but the bundles never showed the app calling
// sendTextMsgToChat WITH a quote, because its reply UI goes through a composer.
//
// So a failure here does not mean "flaky"; it means the inference was wrong and
// the composer path has to be read instead. The test says that in its failure
// message so the next person does not go looking in the wrong place.
func TestRealSPARepliesToAMessage(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this sends real messages to the peer lab account")
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

	// Something to reply TO, sent by this account so nobody else's message is
	// quoted in a test.
	original, err := send.Text(ctx, runner, eval, toJID,
		"wa-headless: mensagem original", "reply/seed")
	if err != nil {
		t.Fatalf("seeding the original: %v", err)
	}
	t.Logf("original: %s", original)

	res, err := send.Reply(ctx, runner, eval, toJID, original.ID.ID,
		"wa-headless: esta e uma resposta", "reply/send")
	if err != nil {
		t.Fatalf("Reply: %v\n\nIf this is ErrNotAReply, the message WENT OUT but "+
			"without a quote — meaning passing {quotedMsg} through "+
			"sendTextMsgToChat's options does not work on this build, and the "+
			"app's composer path must be read instead. That is a measurement "+
			"result, not a flake.", err)
	}
	t.Logf("reply: %s", res)
	if res.ID.ID == original.ID.ID {
		t.Fatal("the reply and the original are the same message")
	}
}

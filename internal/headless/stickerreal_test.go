package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/ack"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestRealSPASendsASticker sends one sticker to the peer lab account.
//
// THE FIXTURE IS CHECKED IN, not generated at test time. A 512x512 WebP is 554
// bytes, and shelling out to cwebp or ffmpeg to rebuild it would make this test
// fail on a machine that has neither — for a reason having nothing to do with
// the code under test.
//
// The route is the MEDIA path with prepRawMedia's asSticker branch, not
// WAWebSendStickerAction: the argument instrument measured that action wanting
// (chat, {mediaData}) — a sticker MODEL already in the account's collection. It
// re-sends a sticker somebody has; it cannot carry bytes.
func TestRealSPASendsASticker(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_MEDIA_TEST") == "" {
		t.Skip("set WA_HEADLESS_MEDIA_TEST=1; this sends a sticker to the peer lab account")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	body, err := os.ReadFile("testdata/lab-sticker.webp")
	if err != nil {
		t.Fatalf("reading the sticker fixture: %v", err)
	}
	if len(body) < 32 || string(body[:4]) != "RIFF" || string(body[8:12]) != "WEBP" {
		t.Fatalf("the fixture is not a WebP (%d bytes)", len(body))
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

	sent, err := send.SendMedia(ctx, runner, sess.Tab().Evaluate, peer, send.Media{
		Filename:  "lab-sticker.webp",
		MimeType:  "image/webp",
		Data:      body,
		AsSticker: true,
	}, "test/sticker")
	if err != nil {
		t.Fatalf("SendMedia(asSticker): %v", err)
	}
	t.Logf("sticker: %s", sent)

	// IT HAS TO BE A STICKER, not merely a message that went out. The send
	// verifier proves a media message left; only the type proves which kind.
	var kind string
	script := `(() => {
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		for (const m of MC.getModelsArray()) {
			try { if (m.id && m.id.id === "` + sent.ID.ID + `") { return String(m.type || ''); } } catch (e) {}
		}
		return '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/sticker-kind", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &kind)
	}); err != nil {
		t.Fatalf("reading the message kind: %v", err)
	}
	t.Logf("MEASURED: the message this build produced is type=%q", kind)
	if kind != "sticker" {
		t.Fatalf("asSticker produced a %q message; the flag reached the page and did not change the kind", kind)
	}

	// And it left: the ack says at least "sent".
	st, err := ack.New(runner, sess.Tab().Evaluate).Of(ctx, sent.ID.ID, "test/sticker-ack")
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	if st.State < ack.Sent {
		t.Fatalf("the sticker never left: %s", st)
	}
}

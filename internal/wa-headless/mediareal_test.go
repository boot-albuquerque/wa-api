package waheadless

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/messagemeta"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// onePixelPNG is a complete, valid 1x1 PNG.
//
// It is real image bytes rather than a placeholder because the page CLASSIFIES
// the payload: WhatsApp decides what kind of message this is from the content
// and the declared type, and something that only claims to be a PNG would be
// testing a different path than the one users take.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// TestRealSPASendsMediaBetweenAccounts is the closed loop for attachments: one
// paired account sends an image, the OTHER receives it, and the match is by
// MESSAGE ID — not by freshness, which H35 measured accepting a stranger's
// message 23 seconds before the send.
//
// It also proves the kind: an inbound TEXT carrying the right id would be a
// contradiction, and accepting it would hide a page that silently downgraded an
// attachment to a caption.
func TestRealSPASendsMediaBetweenAccounts(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this SENDS a real image between the lab accounts")
	}
	from := os.Getenv("WA_SEND_FROM_PROFILE")
	to := os.Getenv("WA_SEND_TO_PROFILE")
	toJID := os.Getenv("WA_SEND_TO_JID")
	if from == "" || to == "" || toJID == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	boot := func(profile string) (*waruntime.Holder, *core.Session, *engine.Runner) {
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
		defer cancel()
		sess, err := h.Session(ctx)
		if err != nil {
			t.Fatalf("boot %s: %v", profile, err)
		}
		return h, sess, runner
	}

	// The RECEIVER first, subscribing BEFORE anything is sent.
	hRx, rx, rxRunner := boot(to)
	defer hRx.Stop(context.Background())
	sub := messagemeta.New(rxRunner, rx.Tab().Evaluate, 0)
	if err := sub.Install(context.Background(), "media/install"); err != nil {
		t.Fatalf("install on receiver: %v", err)
	}
	if _, err := sub.Drain(context.Background(), "media/prime"); err != nil {
		t.Fatalf("priming drain: %v", err)
	}

	hTx, tx, txRunner := boot(from)
	defer hTx.Stop(context.Background())

	data, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	media := send.Media{
		Filename: "wa-headless-probe.png",
		MimeType: "image/png",
		Data:     data,
	}
	t.Logf("sending %s", media)

	// Minutes, not the boot deadline: an upload is involved (H42).
	sendCtx, cancelSend := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelSend()

	res, err := send.SendMedia(sendCtx, txRunner, tx.Tab().Evaluate, toJID, media, "media/send")
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	t.Logf("SENT and VERIFIED on the sender: %s", res)

	deadline := time.Now().Add(120 * time.Second)
	replayed, other := 0, 0
	for time.Now().Before(deadline) {
		d, err := sub.Drain(context.Background(), "media/drain")
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if d.Reinstalled {
			t.Fatal("the receiver's subscription was lost mid-test; the arrival could not " +
				"have been observed and this run cannot answer the question")
		}
		for _, m := range d.Events {
			if m.Timestamp.IsZero() || m.Timestamp.Before(time.Now().Add(-10*time.Minute)) {
				replayed++
				continue
			}
			if m.Direction != messagemeta.DirectionIn {
				continue
			}
			if m.ID.ID != res.ID.ID {
				other++
				continue
			}
			t.Logf("RECEIVED on the other account: %s", m)
			t.Logf("(matched the sender's id %s)", res.ID.ID)
			t.Logf("(ignored %d replayed, %d unrelated fresh inbound)", replayed, other)
			// THE KIND IS PART OF THE CLAIM. An attachment that arrived as a
			// text message would satisfy an id-only check while meaning the
			// send did something else entirely.
			if m.Type == "chat" {
				t.Fatalf("the image arrived as a TEXT message (type=%q): the id matches, "+
					"so this is the right message — and it is not an attachment", m.Type)
			}
			if m.Type == "" {
				t.Fatal("the received event has no type, so the kind cannot be checked")
			}
			t.Logf("CLOSED LOOP: one account sent an image (type=%s), the other received it", m.Type)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("the image was SENT and verified on the sender (id=%s), but no inbound event "+
		"with that id arrived within 120s (%d replayed, %d unrelated fresh inbound)",
		res.ID.ID, replayed, other)
}

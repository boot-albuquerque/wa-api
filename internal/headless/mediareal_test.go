package headless

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/messagemeta"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
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
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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
	// DROPS ARE PART OF THE ANSWER, not noise. A bounded page buffer plus a
	// freshly booted session is exactly the shape H93 measured — ~1200 messages
	// of hydration in the first seconds — and a subscription that overflows
	// throws away real arrivals along with the history. "It never arrived" and
	// "it arrived and was dropped" are different failures and this test used to
	// report them identically.
	dropped, seen := 0, 0
	for time.Now().Before(deadline) {
		d, err := sub.Drain(context.Background(), "media/drain")
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		dropped += d.Dropped
		seen += d.Seen
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
		"with that id arrived within 120s (%d replayed, %d unrelated fresh inbound, "+
		"%d DROPPED by the page of %d seen)",
		res.ID.ID, replayed, other, dropped, seen)
}

// TestRealSPASendsADocumentAndTheKindIsObservable proves the AsDocument flag,
// which the Media type advertises and nothing exercised.
//
// The SAME BYTES are sent twice — once as an image, once as a document — so the
// assertion is about the flag and not about the payload. Anything else would
// leave "asDocument did nothing" indistinguishable from "PNGs are documents".
//
// WHAT THIS TEST DELIBERATELY DOES NOT CHECK: the caption. Invariant 12 makes
// this module metadata-only, and a caption is message CONTENT — so its delivery
// is unverifiable here without breaking the invariant the module exists to
// hold. The capability accepts a caption and passes it to the page; that it
// arrives is not something this suite can claim. Stated rather than quietly
// implied by a passing test (HOUSEKEEP H47).
func TestRealSPASendsADocumentAndTheKindIsObservable(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this SENDS real messages between the lab accounts")
	}
	from := os.Getenv("WA_SEND_FROM_PROFILE")
	toJID := os.Getenv("WA_SEND_TO_JID")
	if from == "" || toJID == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: from, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancelBoot()
	sess, err := h.Session(bootCtx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	data, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	send1 := func(asDocument bool, label string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		m := send.Media{
			Filename:   "wa-headless-probe.png",
			MimeType:   "image/png",
			Data:       data,
			Caption:    "wa-headless",
			AsDocument: asDocument,
		}
		res, err := send.SendMedia(ctx, runner, eval, toJID, m, label)
		if err != nil {
			t.Fatalf("SendMedia(asDocument=%t): %v", asDocument, err)
		}
		return res.ID.ID
	}

	inlineID := send1(false, "media/inline")
	documentID := send1(true, "media/document")
	if inlineID == documentID {
		t.Fatal("both sends returned the same message id, so only one message exists")
	}

	// Read the two messages back and compare their TYPES. Types are metadata,
	// which is what this module is allowed to see.
	kinds := readOwnMessageKinds(t, runner, eval, []string{inlineID, documentID})
	t.Logf("inline kind=%q  document kind=%q", kinds[inlineID], kinds[documentID])

	if kinds[inlineID] == "" || kinds[documentID] == "" {
		t.Fatalf("one of the two messages was not found in the collection: %v", kinds)
	}
	if kinds[inlineID] == kinds[documentID] {
		t.Fatalf("the SAME bytes produced the same kind %q with and without "+
			"AsDocument, so the flag did nothing", kinds[inlineID])
	}
	if kinds[documentID] != "document" {
		t.Fatalf("AsDocument produced kind %q, want \"document\"", kinds[documentID])
	}
	if kinds[inlineID] != "image" {
		t.Fatalf("without AsDocument a PNG produced kind %q, want \"image\"", kinds[inlineID])
	}
}

// readOwnMessageKinds returns the TYPE of each message id, reading only
// metadata: no body, no caption, no identity.
func readOwnMessageKinds(t *testing.T, runner *engine.Runner, eval func(context.Context, string, *string) error, ids []string) map[string]string {
	t.Helper()
	want, _ := json.Marshal(ids)
	script := `JSON.stringify((() => {
		const coll = window.require('WAWebMsgCollection').MsgCollection;
		const want = new Set(` + string(want) + `);
		const out = {};
		for (const m of coll.getModelsArray()) {
			try {
				const id = m.id && m.id.id;
				if (id && want.has(id)) { out[id] = m.type || ''; }
			} catch (e) {}
		}
		return out;
	})())`

	deadline := time.Now().Add(60 * time.Second)
	for {
		var raw string
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		err := runner.Do(ctx, engine.OpStateProbe, "media/kinds", func(c context.Context) error {
			return eval(c, script, &raw)
		})
		cancel()
		if err != nil {
			t.Fatalf("reading kinds: %v", err)
		}
		got := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("decoding kinds: %v", err)
		}
		if len(got) == len(ids) || !time.Now().Before(deadline) {
			return got
		}
		time.Sleep(2 * time.Second)
	}
}

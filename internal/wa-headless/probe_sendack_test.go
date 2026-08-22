package waheadless

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeSendAckLatency measures whether an ack postcondition is affordable
// for the ordinary send path.
//
// H98 found that a poll is created locally and never leaves, at ack 0, and that
// the capability called it sent — because verification looks for the message
// APPEARING in this session, not for it having gone anywhere. That method is the
// same for text, media, sticker and document.
//
// Text demonstrably does arrive: the peer receives it in every cross-account
// test in this suite. So the question is not whether text works, it is whether
// the postcondition can be tightened to say so — and that depends on one number
// nobody has measured: how long an ack takes.
func TestProbeSendAckLatency(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEND_ACK") == "" {
		t.Skip("set WA_PROBE_SEND_ACK=1; sends one message to the lab peer")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	sent := time.Now()
	res, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("wa-headless ack probe %d", sent.Unix()), "probe/ack")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	t.Logf("verified as sent after %s (by local appearance)", res.Waited.Round(time.Millisecond))

	// AND NOW THE NUMBER THAT DECIDES. Ack 0 is pending, 1 server, 2 device,
	// 3 read. If 1 arrives in a fraction of the verification budget, the
	// postcondition can be tightened; if it takes many seconds, tightening it
	// would make every send slower for a guarantee most callers do not need
	// synchronously.
	script := `JSON.stringify((() => {
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		const all = typeof MC.getModelsArray === 'function' ? MC.getModelsArray() : [];
		for (const m of all) {
			try { if (m.id && m.id.id === ` + strconv.Quote(res.ID.ID) + `) {
				return { ack: typeof m.ack === 'number' ? m.ack : -1 };
			} } catch (e) {}
		}
		return { ack: -2 };
	})())`
	var firstServer, firstDevice time.Duration
	for i := 0; i < 60; i++ {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/ack-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, script, &raw)
		}); err != nil {
			t.Fatalf("reading the ack: %v", err)
		}
		elapsed := time.Since(sent)
		if firstServer == 0 && (strings.Contains(raw, `"ack":1`) || strings.Contains(raw, `"ack":2`) || strings.Contains(raw, `"ack":3`)) {
			firstServer = elapsed
			t.Logf("ack >= 1 (reached the server) after %s", elapsed.Round(time.Millisecond))
		}
		if firstDevice == 0 && (strings.Contains(raw, `"ack":2`) || strings.Contains(raw, `"ack":3`)) {
			firstDevice = elapsed
			t.Logf("ack >= 2 (reached the device) after %s", elapsed.Round(time.Millisecond))
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if firstServer == 0 {
		t.Errorf("the message never reached the server in 30s; a text send has the same " +
			"defect the poll send had, and this is much worse news than the poll")
	}
	t.Logf("VERDICT: local verification took %s, ack>=1 took %s, ack>=2 took %s",
		res.Waited.Round(time.Millisecond), firstServer.Round(time.Millisecond),
		firstDevice.Round(time.Millisecond))
}

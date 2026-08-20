package waheadless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/messagemeta"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPASendAndReceiveBetweenAccounts is the closed loop: one paired
// account sends, the OTHER receives, and both halves are verified against the
// real service.
//
// It is the first test in this module that proves the RECEIVING direction
// without a human — until now `dir=in` had only ever been seen in replayed
// history, because the only account available was the one doing the sending.
//
// It sends only between the two lab accounts, never to a third party, and the
// text is a marker with no meaning outside this test.
func TestRealSPASendAndReceiveBetweenAccounts(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this SENDS a real message between the lab accounts")
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

	// The RECEIVER goes up first and subscribes BEFORE anything is sent.
	// Subscribing after would leave the arrival unobservable and force the test
	// to fall back on history — which is how the delivery test produced a false
	// positive before the freshness rule existed.
	hRx, rx, rxRunner := boot(to)
	defer hRx.Stop(context.Background())
	sub := messagemeta.New(rxRunner, rx.Tab().Evaluate, 0)
	if err := sub.Install(context.Background(), "loop/install"); err != nil {
		t.Fatalf("install on receiver: %v", err)
	}
	if _, err := sub.Drain(context.Background(), "loop/prime"); err != nil {
		t.Fatalf("priming drain: %v", err)
	}

	hTx, tx, txRunner := boot(from)
	defer hTx.Stop(context.Background())

	marker := fmt.Sprintf("wa-headless-loop-%d", time.Now().UnixNano())
	sentAt := time.Now().Add(-2 * time.Minute)

	res, err := send.Text(context.Background(), txRunner, tx.Tab().Evaluate, toJID, marker, "loop/send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("SENT and VERIFIED on the sender: %s", res)

	// The receiving half. Fresh only: the collection replays history, and an
	// old inbound message would confirm nothing about this send.
	deadline := time.Now().Add(90 * time.Second)
	replayed := 0
	for time.Now().Before(deadline) {
		d, err := sub.Drain(context.Background(), "loop/drain")
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if d.Reinstalled {
			t.Fatal("the receiver's subscription was lost mid-test; the arrival could not " +
				"have been observed and this run cannot answer the question")
		}
		for _, m := range d.Events {
			if m.Timestamp.Before(sentAt) || m.Timestamp.IsZero() {
				replayed++
				continue
			}
			if m.Direction != messagemeta.DirectionIn {
				continue
			}
			t.Logf("RECEIVED on the other account: %s", m)
			t.Logf("(ignored %d replayed history event(s))", replayed)
			if !m.ID.Present() {
				t.Error("the received event has no message id")
			}
			if m.Type == "" {
				t.Error("the received event has no type")
			}
			t.Log("CLOSED LOOP: one account sent, the other received, both verified")
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("the message was SENT and verified on the sender, but never arrived as a FRESH "+
		"inbound event on the receiver within 90s (%d replayed events seen). Either delivery "+
		"is slower than this budget, or the receiving subscription does not see this class "+
		"of message", replayed)
}

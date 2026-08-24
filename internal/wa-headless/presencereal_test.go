package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/owner"
	"wa-api/internal/wa-headless/capabilities/presence"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAPresenceCrossesBetweenAccounts is the only honest proof presence
// can have.
//
// ANNOUNCING HAS NO LOCAL POSTCONDITION: the page accepts markComposing and
// nothing on this side changes, so a capability that "succeeded" would be
// indistinguishable from one that sent nothing at all. The evidence has to come
// from the OTHER account, which is why this needs both.
//
// The observing side subscribes first. Measured: zero of 384 presence models on
// the lab account were subscribed, so a read without one reports a silence that
// looks exactly like somebody idle.
func TestRealSPAPresenceCrossesBetweenAccounts(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_SEND_TEST") == "" {
		t.Skip("set WA_HEADLESS_SEND_TEST=1; this announces real presence between the lab accounts")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	toJID := os.Getenv("WA_SEND_TO_JID")
	if fromProfile == "" || toProfile == "" || toJID == "" {
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

	hTx, tx, txRunner := boot(fromProfile)
	defer hTx.Stop(context.Background())
	hRx, rx, rxRunner := boot(toProfile)
	defer hRx.Stop(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	// WHO is announcing, asked of the page rather than hardcoded: a jid written
	// into a test is a phone number in a repository.
	me, err := owner.Refresh(ctx, txRunner, tx.Tab().Evaluate, "presence/who")
	if err != nil {
		t.Fatalf("owner.Refresh on the announcing account: %v", err)
	}
	// THE LID, NOT THE PHONE. The first version of this test preferred the PN
	// and the observing account answered NO_CHAT: this build files chats under
	// the identity the SERVER assigns, and 397 of 399 messages plus 489 of 944
	// contacts live under "@lid" (H34, H39). Observing by the number is
	// observing something the collection does not hold.
	announcer := me.LID.Serialized
	if announcer == "" {
		announcer = me.PN.Serialized
	}
	if announcer == "" {
		t.Fatal("the announcing account has no identity to be observed by")
	}
	t.Logf("announcing account identified: %s", me)

	// THE BUDGET IS RAISABLE FROM THE ENVIRONMENT, for one specific question.
	//
	// H50 saw isSubscribed flip true ONCE and false on every run after. H91
	// synced the accounts into each other's address books and it flipped true
	// again — once. Two readings of that are possible and they call for opposite
	// work: either the subscription genuinely does not take, or it takes longer
	// than the twenty seconds nobody measured. Raising the budget is how the
	// two are told apart, and it is a knob rather than a new default because a
	// number changed to make a test pass is not a measurement.
	if raw := os.Getenv("WA_PRESENCE_SUBSCRIBE_BUDGET"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("WA_PRESENCE_SUBSCRIBE_BUDGET=%q: %v", raw, err)
		}
		presence.SetSubscribeBudget(d)
		t.Logf("subscription budget raised to %s for this run", d)
	}

	rxSide := presence.New(rxRunner, rx.Tab().Evaluate)
	txSide := presence.New(txRunner, tx.Tab().Evaluate)

	// Subscribe FIRST and record the baseline. Without it, a chatstate that was
	// already "composing" would be read as proof of something this test caused.
	before, err := rxSide.Observe(ctx, announcer, "presence/observe-before")
	if err != nil {
		t.Skipf("the observing account cannot see this presence yet (%v); the two "+
			"accounts must have an existing chat for presence to be observable", err)
	}
	t.Logf("baseline on the observer: %s", before)
	if before.Typing() {
		t.Skip("the announcing account was ALREADY typing before this test did " +
			"anything; the run cannot attribute what it sees")
	}

	// AVAILABLE FIRST. The baseline read the announcing account as online=false
	// while its browser was plainly open, which is the tell: a session that has
	// not announced availability is not one whose typing anybody is told about.
	// The page's own client does this on connect; a headless driver has to say
	// it out loud.
	if err := txSide.SetOnline(ctx, true, "presence/available"); err != nil {
		t.Fatalf("SetOnline(true): %v", err)
	}
	t.Log("announced available")

	if err := txSide.Set(ctx, toJID, presence.StateComposing, "presence/compose"); err != nil {
		t.Fatalf("Set(composing): %v", err)
	}
	t.Log("announced composing")

	deadline := time.Now().Add(45 * time.Second)
	seen := false
	var last presence.Snapshot
	for time.Now().Before(deadline) {
		got, err := rxSide.Observe(ctx, announcer, "presence/observe")
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		last = got
		if got.Typing() {
			seen = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("observer after the announcement: %s", last)
	if !seen {
		t.Fatalf("the observing account never saw the typing state. Announcing has "+
			"no local evidence, so this is the ONLY thing that distinguishes a "+
			"working call from one that sent nothing (last: %s)", last)
	}

	// AND IT MUST STOP. A client that only ever announced composing would leave
	// the other side showing a typing indicator forever, which is worse than
	// never announcing at all.
	// RECORDING was implemented and never exercised against the page — the
	// ledger audit found it, and a mapping table asserted in a unit test proves
	// the NAME is right and not that the page accepts the call. Announcing it
	// costs one round trip and turns a PARTIAL into something measured.
	if err := txSide.Set(ctx, toJID, presence.StateRecording, "presence/record"); err != nil {
		t.Fatalf("announcing recording: %v", err)
	}
	t.Log("recording announced; the page accepted markRecording")

	if err := txSide.Set(ctx, toJID, presence.StatePaused, "presence/pause"); err != nil {
		t.Fatalf("Set(paused): %v", err)
	}
	stopDeadline := time.Now().Add(45 * time.Second)
	stopped := false
	for time.Now().Before(stopDeadline) {
		got, err := rxSide.Observe(ctx, announcer, "presence/observe-stop")
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		last = got
		if !got.Typing() {
			stopped = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !stopped {
		t.Fatalf("the typing indicator never cleared after StatePaused (last: %s)", last)
	}
	t.Logf("CLOSED LOOP: one account announced typing, the other saw it, and it stopped")
}

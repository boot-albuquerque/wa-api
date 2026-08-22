package waheadless

import (
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/send"
)

// TestProbeSeenAcrossSessions proves sendSeen by the fact it exists to produce,
// instead of by the counter that H82 correctly refused to trust.
//
// H82 demoted this row for a good reason: MarkRead's postcondition asserts that
// chat.unreadCount moved IN THE SAME SESSION, and H78 measured that counter as
// cross-session. H52 proved it against one chat where it did move; whether that
// generalises stayed open, and asserting a counter that may not move is how a
// capability reports success it cannot see.
//
// THE COUNTER IS THE WRONG WITNESS ANYWAY. What sendSeen does that anybody can
// observe is tell the SENDER their message was read — ack 3 on their copy. That
// is a fact about the other session, which is exactly what no single session
// could check, and exactly what the dual harness (H135) is for.
//
// conta-B sends, conta-A marks read, and conta-B's own copy is watched. The
// baseline matters: a message already at ack 3 before marking would prove
// nothing, so this refuses to start from there.
func TestProbeSeenAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEEN") == "" {
		t.Skip("set WA_PROBE_SEEN=1 (one message between the lab accounts)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	selfA := os.Getenv("WA_SELF_A_JID")
	if pa == "" || pb == "" || selfA == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B and WA_SELF_A_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 10*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	// conta-B manda para conta-A.
	sent, err := send.Text(ctx, d.RunnerB, evalB, selfA, "wa-headless seen probe", "probe/seen")
	if err != nil {
		t.Fatalf("B->A: %v", err)
	}
	t.Logf("conta-B sent: %s", sent)

	ackOnB := func(what string) int {
		t.Helper()
		var raw string
		script := `(() => {
			try {
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					if (m.id && m.id.id === ` + strconv.Quote(sent.ID.ID) + `) {
						return String(typeof m.ack === "number" ? m.ack : -1);
					}
				}
				return "-2";
			} catch (e) { return "-3"; }
		})()`
		if err := evalB(ctx, script, &raw); err != nil {
			t.Fatalf("ack read (%s): %v", what, err)
		}
		n, _ := strconv.Atoi(raw)
		return n
	}

	time.Sleep(5 * time.Second)
	before := ackOnB("before")
	t.Logf("ack on conta-B BEFORE conta-A reads: %d", before)
	if before < 0 {
		t.Fatalf("conta-B cannot see its own message (ack %d); nothing below means anything", before)
	}
	if before >= 3 {
		t.Skip("the message was already read before conta-A acted; a baseline of 3 " +
			"cannot show a transition, and waiting for one would be waiting for nothing")
	}

	// conta-A marca lida. A IDENTIDADE E' A RESOLVIDA — a lição da H148, e o
	// MarkUnread deste mesmo pacote recusa o jid de telefone.
	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, os.Getenv("WA_PEER_B_JID"), "probe/seen/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B from conta-A: %v", err)
	}
	res, err := chats.New(d.RunnerA, evalA).MarkRead(ctx, ident.JID, "probe/seen/markread")
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	t.Logf("conta-A MarkRead: %s", res)

	deadline := time.Now().Add(60 * time.Second)
	for {
		got := ackOnB("after")
		if got >= 3 {
			t.Logf("PROVEN: conta-B's message reached ack %d after conta-A read it "+
				"(was %d)", got, before)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("conta-A marked the chat read and conta-B's copy stayed at ack %d "+
				"(was %d); either the read receipt did not leave, or this account has "+
				"read receipts disabled — both are answers the row needs", got, before)
		}
		time.Sleep(2 * time.Second)
	}
}

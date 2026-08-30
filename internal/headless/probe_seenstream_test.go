package headless

import (
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/send"
)

// TestProbeSeenWithStreamAvailable tests the one difference between our
// mark-read and the reference's, found by reading the reference AFTER the
// measurement said the read receipt does not leave.
//
// H159 measured, with the dual session, that marking a chat read moves nothing:
// unreadCount stays put and the SENDER's copy never reaches ack 3 — in BOTH
// directions, with two different accounts, which is what makes a privacy setting
// an unlikely explanation.
//
// The reference brackets its call (wwebjs_util.js:131-143):
//
//	Stream.markAvailable();
//	await UpdateUnreadChatAction.sendSeen({chat, threadId: undefined});
//	Stream.markUnavailable();
//
// We call the seen primitive with no bracketing. Announcing the client as ONLINE
// before sending a read receipt is not decoration if the server drops receipts
// from a client that is not present — and that hypothesis predicts exactly what
// was measured.
//
// THE HYPOTHESIS IS TESTED RAW, in the page, before anything is changed in the
// capability. Changing the capability first and re-measuring would confound "the
// bracketing helped" with "the retry helped".
func TestProbeSeenWithStreamAvailable(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEENSTREAM") == "" {
		t.Skip("set WA_PROBE_SEENSTREAM=1 (one message between the lab accounts)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	selfA, peerB := os.Getenv("WA_SELF_A_JID"), os.Getenv("WA_PEER_B_JID")
	if pa == "" || pb == "" || selfA == "" || peerB == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B, WA_SELF_A_JID and WA_PEER_B_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 12*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	sent, err := send.Text(ctx, d.RunnerB, evalB, selfA, "headless seen-stream probe", "probe/seenstream")
	if err != nil {
		t.Fatalf("B->A: %v", err)
	}
	ackOnB := func() int {
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
			return -4
		}
		n, _ := strconv.Atoi(raw)
		return n
	}
	time.Sleep(5 * time.Second)
	before := ackOnB()
	t.Logf("ack on conta-B before: %d", before)
	if before >= 3 {
		t.Skip("already read; no transition to observe")
	}

	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/seenstream/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B: %v", err)
	}

	script := `(() => {
		window.__ss = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,140);
		(async () => {
		const out = {};
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const chat = CC.get(` + strconv.Quote(ident.JID) + `);
			out.chatFound = !!chat;
			if (!chat) { window.__ss = JSON.stringify(out); return; }
			out.unreadBefore = (typeof chat.unreadCount === "number") ? chat.unreadCount : -1;

			let S = null;
			try { S = window.require("WAWebStreamModel").Stream; } catch (e) { out.streamErr = safe(e); }
			out.streamFound = !!S;
			out.hasMarkAvailable = !!(S && typeof S.markAvailable === "function");

			const A = window.require("WAWebUpdateUnreadChatAction");
			out.actionKeys = Object.keys(A).slice(0, 8);

			if (S && typeof S.markAvailable === "function") { S.markAvailable(); out.marked = true; }
			await A.sendSeen({ chat: chat, threadId: undefined });
			if (S && typeof S.markUnavailable === "function") { S.markUnavailable(); }
			out.sent = true;
			out.unreadAfter = (typeof chat.unreadCount === "number") ? chat.unreadCount : -1;
		} catch (e) { out.err = safe(e); }
		window.__ss = JSON.stringify(out);
		})();
		return "kicked";
	})()`
	var ignored string
	if err := evalA(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := evalA(ctx, "window.__ss", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the bracketed sendSeen never answered")
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("bracketed sendSeen on conta-A: %s", raw)

	deadline = time.Now().Add(60 * time.Second)
	for {
		got := ackOnB()
		if got >= 3 {
			t.Logf("HYPOTHESIS CONFIRMED: with Stream.markAvailable() bracketing, the "+
				"read receipt LEFT — ack %d (was %d). Our mark-read omits it.", got, before)
			return
		}
		if time.Now().After(deadline) {
			t.Logf("HYPOTHESIS REFUTED: even bracketed, ack stayed at %d (was %d). "+
				"The bracketing is not what is missing.", got, before)
			return
		}
		time.Sleep(2 * time.Second)
	}
}

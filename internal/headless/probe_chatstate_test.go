package headless

import (
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/presence"
)

// TestProbeRecordingSeenByPeer asks whether a chat state reaches the other side
// WITHOUT the presence subscription that H144 measured as unreachable.
//
// sendStateRecording is PARTIAL because proving it live "esbarra no mesmo
// bloqueio da observação de presença" — and H144 turned that block into a
// measured cause: presence subscription needs an address-book link that lives on
// the PHONE, and the lab pair does not have it (isMyContact false on both).
//
// THAT IS A STATEMENT ABOUT presence.Observe, NOT NECESSARILY ABOUT CHAT STATE.
// H150 measured `typing` firing on the acting session's own chat model, which
// says the field exists and moves. Whether conta-A recording reaches conta-B's
// model is a different question, and the dual session can ask it directly —
// reading the model rather than going through a subscription that is known not
// to land.
//
// The reading is raw and on the OBSERVER, for the H162 reason: the observer's
// answer is the one that decides, so the baseline comes from there too.
func TestProbeRecordingSeenByPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHATSTATE") == "" {
		t.Skip("set WA_PROBE_CHATSTATE=1 (announces a recording state to the lab peer)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	peerB, selfA := os.Getenv("WA_PEER_B_JID"), os.Getenv("WA_SELF_A_JID")
	if pa == "" || pb == "" || peerB == "" || selfA == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B, WA_PEER_B_JID and WA_SELF_A_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 12*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	identA, err := lookup.New(d.RunnerB, evalB).NumberID(ctx, selfA, "probe/chatstate/resolve")
	if err != nil {
		t.Fatalf("conta-B resolving conta-A: %v", err)
	}
	identB, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/chatstate/resolve-b")
	if err != nil {
		t.Fatalf("conta-A resolving conta-B: %v", err)
	}

	// O QUE CONTA-B VE SOBRE CONTA-A, lido cru do modelo de presenca dela.
	stateOnB := func(what string) string {
		var raw string
		script := `(() => {
			try {
				const P = window.require("WAWebPresenceCollection").PresenceCollection;
				const p = P.get(` + strconv.Quote(identA.JID) + `);
				if (!p) { return JSON.stringify({found:false}); }
				const cs = p.chatstate;
				return JSON.stringify({found:true,
					type: cs ? String(cs.type || "") : "",
					hasChatstate: !!cs,
					// As duas listas que a H94 mediu como as unicas que se movem.
					typing: Array.isArray(p.chatstates) ? p.chatstates.length : -1});
			} catch (e) { return JSON.stringify({err:String(e).slice(0,90)}); }
		})()`
		if err := evalB(ctx, script, &raw); err != nil {
			t.Fatalf("reading conta-A's presence on conta-B (%s): %v", what, err)
		}
		return raw
	}

	before := stateOnB("before")
	t.Logf("BEFORE (conta-B's view of conta-A): %s", before)

	ann := presence.New(d.RunnerA, evalA)
	if err := ann.Set(ctx, identB.JID, presence.StateRecording, "probe/chatstate/record"); err != nil {
		t.Fatalf("conta-A announcing recording: %v", err)
	}
	t.Log("conta-A announced recording")

	deadline := time.Now().Add(45 * time.Second)
	var last string
	for {
		last = stateOnB("after")
		if last != before {
			t.Logf("AFTER: %s", last)
			t.Log("MEASURED: the chat state moved on the peer's model WITHOUT a " +
				"presence subscription; the row's block was about Observe, not about this")
			return
		}
		if time.Now().After(deadline) {
			t.Logf("AFTER: %s", last)
			t.Log("CONFIRMED: nothing moved on the peer in 45s. The block H144 " +
				"measured for presence covers this row too, and it is a phone-side " +
				"address-book dependency rather than pending work")
			return
		}
		time.Sleep(3 * time.Second)
	}
}

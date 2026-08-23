package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/capabilities/lookup"
)

// TestProbeChannelTransferOwnership proves transferChannelOwnership, which the
// H136 chain deliberately did NOT execute.
//
// THE REASON IT WAS REFUSED THEN, AND HOW IT IS HANDLED NOW. Transferring makes
// conta-A stop owning the channel, so conta-A can no longer delete it — and a
// probe that created a real channel it could not remove would leave an entity
// standing on a real account. The fix is not to skip the step: it is to move the
// CLEANUP to the new owner. conta-B deletes what conta-B now owns.
//
// The teardown therefore has two paths, and picks by what actually happened
// rather than by what was intended: if the transfer took, B deletes; if it did
// not, A still can and does.
func TestProbeChannelTransferOwnership(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANXFER") == "" {
		t.Skip("set WA_PROBE_CHANXFER=1 (creates a channel, transfers it, deletes it)")
	}
	profileA := os.Getenv("WA_SEND_FROM_PROFILE")
	profileB := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profileA == "" || profileB == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	d, ctx, closeAll := openDual(t, profileA, profileB, 12*time.Minute)
	defer closeAll()
	evalA := d.A.Tab().Evaluate
	evalB := d.B.Tab().Evaluate

	res := lookup.New(d.RunnerA, evalA)
	ident, err := res.NumberID(ctx, peer, "probe/chanxfer")
	if err != nil {
		t.Fatalf("resolving the new owner: %v", err)
	}
	peerResolved := ident.JID
	t.Logf("new owner resolved: %s", ident)

	mA := channel.NewManager(d.RunnerA, evalA)
	mB := channel.NewManager(d.RunnerB, evalB)
	made, err := mA.Create(ctx, "wa-headless transfer "+strconv.FormatInt(time.Now().Unix(), 10),
		"", "probe/chanxfer")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("channel created by A: %s", made)

	transferred := false
	defer func() {
		// WHOEVER OWNS IT, DELETES IT. Choosing by the recorded outcome and not by
		// intent is the point: a transfer that half-happened must not leave both
		// sides assuming the other cleaned up.
		owner, side := mA, "A"
		if transferred {
			owner, side = mB, "B"
		}
		if err := owner.Delete(ctx, made.JID, made.InviteCode, "probe/chanxfer-undo"); err != nil {
			t.Errorf("DELETE BY %s FAILED: a real channel is left standing: %v", side, err)
			// LAST RESORT: try the other side rather than leave it up.
			other, otherSide := mB, "B"
			if transferred {
				other, otherSide = mA, "A"
			}
			if err2 := other.Delete(ctx, made.JID, made.InviteCode, "probe/chanxfer-undo2"); err2 != nil {
				t.Errorf("DELETE BY %s ALSO FAILED: %v — this channel must be removed by hand",
					otherSide, err2)
			} else {
				t.Logf("deleted by %s on the second attempt", otherSide)
			}
			return
		}
		t.Logf("channel deleted by %s", side)
	}()

	park := func(eval func(context.Context, string, *string) error, key, script, what string) map[string]any {
		t.Helper()
		var ignored string
		if err := eval(ctx, script, &ignored); err != nil {
			t.Fatalf("%s kick: %v", what, err)
		}
		deadline := time.Now().Add(45 * time.Second)
		for {
			var raw string
			if err := eval(ctx, "window."+key, &raw); err != nil {
				t.Fatalf("%s read: %v", what, err)
			}
			if raw != "" && raw != "null" {
				var v map[string]any
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					t.Fatalf("%s payload: %v", what, err)
				}
				return v
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never answered", what)
			}
			time.Sleep(400 * time.Millisecond)
		}
	}

	// The new owner has to be an admin first — that is what the invite chain of
	// H136 is for, and it is reused rather than reinvented.
	inviteScript := `(() => {
		window.__x1 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			const C = window.require("WAWebCollections");
			const userWid = W.createWid(` + strconv.Quote(peerResolved) + `);
			let chat = C.Chat.get(userWid) || C.Chat.get(` + strconv.Quote(peerResolved) + `);
			if (!chat) {
				const all = C.Chat.getModelsArray();
				for (const c of all) {
					const id = c.id;
					if (id && id.user === userWid.user) { chat = c; break; }
				}
			}
			if (!chat) { window.__x1 = JSON.stringify({ok:false, why:"no chat with the new owner"}); return; }
			const r = await window.require("WAWebNewsletterSendMsgAction")
				.sendNewsletterAdminInviteMessage(chat, {
					newsletterWid: W.createWid(` + strconv.Quote(made.JID) + `),
					invitee: userWid, inviteMessage: "wa-headless transfer probe", base64Thumb: null,
				});
			window.__x1 = JSON.stringify({ok:true, result:(r && r.messageSendResult) || String(r)});
		} catch (e) { window.__x1 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	t.Logf("invite (A): %v", park(evalA, "__x1", inviteScript, "invite"))

	acceptScript := `(() => {
		window.__x2 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			await window.require("WAWebMexAcceptNewsletterAdminInviteJob")
				.acceptNewsletterAdminInvite(` + strconv.Quote(made.JID) + `);
			window.__x2 = JSON.stringify({ok:true});
		} catch (e) { window.__x2 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	accept := park(evalB, "__x2", acceptScript, "accept")
	t.Logf("accept (B): %v", accept)
	if ok, _ := accept["ok"].(bool); !ok {
		t.Fatalf("the new owner could not become an admin, so the transfer cannot be " +
			"attempted; nothing after this would mean anything")
	}

	// THE TRANSFER. Shapes are the ones H137 established for this family: the
	// channel MODEL and the contact MODEL, not ids and not Wids.
	xferScript := `(() => {
		window.__x3 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			const C = window.require("WAWebCollections");
			const NC = C.WAWebNewsletterCollection;
			const ch = NC.get(` + strconv.Quote(made.JID) + `);
			if (!ch) { window.__x3 = JSON.stringify({ok:false, why:"own channel not in the collection"}); return; }
			const contact = C.Contact.get(W.createWid(` + strconv.Quote(peerResolved) + `));
			if (!contact) { window.__x3 = JSON.stringify({ok:false, why:"new owner not in the contact collection"}); return; }
			await window.require("WAWebChangeNewsletterOwnerAction")
				.changeNewsletterOwnerAction(ch, contact);
			window.__x3 = JSON.stringify({ok:true});
		} catch (e) { window.__x3 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	xfer := park(evalA, "__x3", xferScript, "transfer")
	t.Logf("transfer (A): %v", xfer)
	if ok, _ := xfer["ok"].(bool); ok {
		transferred = true
	}

	// THE POSTCONDITION IS THE MEMBERSHIP, read from the side that should now
	// own it. The call not throwing proves nothing — that is the whole lesson of
	// H133 and H137.
	rB := channel.New(d.RunnerB, evalB)
	back, err := rB.ByInviteCode(ctx, made.InviteCode, "probe/chanxfer-verify")
	if err != nil {
		t.Errorf("reading the channel back from B: %v", err)
		return
	}
	t.Logf("B's view after the transfer: %s", back)

	listB, err := mB.Followed(ctx, "probe/chanxfer-verify")
	if err != nil {
		t.Errorf("Followed (B): %v", err)
		return
	}
	for _, e := range listB {
		if e.JID == made.JID {
			t.Logf("B's membership on the channel: %q", e.Membership)
			if e.Membership == "owner" {
				t.Log("MEASURED: the ownership DID move — B is now the owner")
			} else {
				t.Logf("MEASURED: the call reported ok and B's membership is %q, not "+
					"owner; the transfer did not take", e.Membership)
			}
		}
	}
}

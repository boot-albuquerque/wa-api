package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/chats"
	"wa-api/internal/headless/capabilities/contacts"
	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeDelegationCallSites checks the delegation rows at their OWN inputs
// instead of moving them because a peer moved.
//
// Ten ledger rows are marked "delegação literal": upstream has no logic of its
// own there, it forwards to a Client method. getChatById and getContactById are
// both PROVEN, so the rows forwarding to them look like paperwork.
//
// THEY ARE NOT, AND H148 IS WHY. A delegation is proven when the capability
// works ON THE INPUT THAT CALL SITE ACTUALLY PASSES, and the inputs differ:
// GroupNotification.getContact passes `author` — a group PARTICIPANT — not the
// chat; Chat.getContact passes the conversation's counterpart. On a LID-first
// build, "the capability works" and "the capability works on this jid" have come
// apart three times already.
//
// Broadcast.getChat and Broadcast.getContact are deliberately NOT here: they
// pass a STATUS id, and the broadcast identity rows are PARTIAL on their own
// measurement. Closing those would be exactly the association this refuses.
func TestProbeDelegationCallSites(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DELEG") == "" {
		t.Skip("set WA_PROBE_DELEG=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
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
	eval := sess.Tab().Evaluate
	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found")
	}

	// GroupNotification.getChat passa o jid do GRUPO.
	if _, err := chats.New(runner, eval).ByJID(ctx, gjid, "probe/deleg/gn-getchat"); err != nil {
		t.Errorf("GroupNotification.getChat call site: chats.ByJID on the group jid: %v", err)
	} else {
		t.Log("GroupNotification.getChat call site: the group jid resolves to a chat")
	}

	// GroupNotification.getContact passa `author`, que e' um PARTICIPANTE.
	author := findGroupNotificationAuthor(ctx, t, eval)
	if author == "" {
		// PRODUZIR O FATO, que e' a manobra que fechou a citacao, o voto, os
		// eventos de grupo e as mencoes. Renomear o grupo gera uma notificacao
		// gp2 com autor; o nome antigo e' restaurado logo em seguida.
		md, err := group.New(runner, eval).Metadata(ctx, gjid, "probe/deleg/subject-before")
		if err != nil {
			t.Fatalf("reading the group subject: %v", err)
		}
		old := md.Subject
		if old == "" {
			t.Skip("the lab group has no readable subject to restore; not renaming it blind")
		}
		if _, err := group.New(runner, eval).SetSubject(ctx, gjid, old+" probe", "probe/deleg/rename"); err != nil {
			t.Logf("renaming to produce a notification: %v", err)
		}
		defer func() {
			if _, err := group.New(runner, eval).SetSubject(context.Background(), gjid, old, "probe/deleg/restore"); err != nil {
				t.Errorf("RESTORE FAILED, the lab group keeps the probe subject: %v", err)
			}
		}()
		time.Sleep(6 * time.Second)
		author = findGroupNotificationAuthor(ctx, t, eval)
	}
	if author == "" {
		t.Log("no gp2 notification carries an author even after producing one; " +
			"that call site stays unexercised and the row does not move")
	} else {
		c := contacts.New(runner, eval)
		if _, err := c.ByJID(ctx, author, "probe/deleg/gn-getcontact"); err != nil {
			t.Errorf("GroupNotification.getContact call site: contacts.ByJID on a "+
				"notification author: %v", err)
		} else {
			t.Log("GroupNotification.getContact call site: the author resolves to a contact")
		}
	}

	// Chat.getContact passa a contraparte da conversa.
	counterpart := findOneToOneCounterpart(ctx, t, eval)
	if counterpart == "" {
		t.Log("no one-to-one chat loaded; that call site stays unexercised")
		return
	}
	if _, err := contacts.New(runner, eval).ByJID(ctx, counterpart, "probe/deleg/chat-getcontact"); err != nil {
		t.Errorf("Chat.getContact call site: contacts.ByJID on a conversation "+
			"counterpart: %v", err)
	} else {
		t.Log("Chat.getContact call site: the counterpart resolves to a contact")
	}
}

func findGroupNotificationAuthor(ctx context.Context, t *testing.T, eval func(context.Context, string, *string) error) string {
	t.Helper()
	var raw string
	const script = `(() => {
		try {
			for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
				if (m.type !== "gp2") { continue; }
				const a = m.author || (m.id && m.id.participant);
				const s = (a && a._serialized) ? a._serialized : (typeof a === "string" ? a : "");
				if (s) { return s; }
			}
			return "";
		} catch (e) { return ""; }
	})()`
	if err := eval(ctx, script, &raw); err != nil {
		t.Fatalf("finding a notification author: %v", err)
	}
	return raw
}

func findOneToOneCounterpart(ctx context.Context, t *testing.T, eval func(context.Context, string, *string) error) string {
	t.Helper()
	var raw string
	const script = `(() => {
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const G = window.require("WAWebChatGetters");
			for (const c of CC.getModelsArray()) {
				if (G.getIsGroup(c)) { continue; }
				const s = (c.id && c.id._serialized) ? c.id._serialized : "";
				if (s && s.indexOf("@broadcast") < 0 && s.indexOf("status") < 0) { return s; }
			}
			return "";
		} catch (e) { return ""; }
	})()`
	if err := eval(ctx, script, &raw); err != nil {
		t.Fatalf("finding a one-to-one counterpart: %v", err)
	}
	return raw
}

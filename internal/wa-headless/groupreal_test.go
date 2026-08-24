package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAResolvesAGroupWithoutSending proves the group branch of the send
// path against a real group, and sends NOTHING.
//
// A group has members. A proof that had to send in order to check the
// resolution would be a proof nobody could run, so send.Resolve exists to stop
// exactly where the dispatch would begin — and it shares resolveChatExpr with
// the real senders, which is what makes this a statement about them.
//
// WHY IT EXISTS: measured 2026-08-20, sending to a group was BROKEN and failed
// with a misleading reason. createWid survives a "@g.us" jid, ChatCollection.get
// returns the chat — but queryWidExists, which resolves USERS, answers NULL. So
// every group send died at the identity step reporting NOT_ON_WHATSAPP, for a
// group plainly present in the collection.
//
// The jid is read inside the test and never logged.
func TestRealSPAResolvesAGroupWithoutSending(t *testing.T) {
	requireRealSPA(t)
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Skip("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	// Find a group and an individual to compare against. Counts are logged;
	// identities are not.
	var raw string
	const findScript = `JSON.stringify((() => {
		const all = window.require('WAWebChatCollection').ChatCollection.getModelsArray();
		let group = '', user = '', groups = 0, users = 0;
		for (const c of all) {
			try {
				const id = c.id;
				if (!id || !id._serialized) { continue; }
				if (id.server === 'g.us') { groups++; if (!group) { group = id._serialized; } }
				else if (id.server === 'lid' || id.server === 'c.us') { users++; if (!user) { user = id._serialized; } }
			} catch (e) {}
		}
		return { group: group, user: user, groups: groups, users: users };
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "group/find", func(c context.Context) error {
		return eval(c, findScript, &raw)
	}); err != nil {
		t.Fatalf("finding a group: %v", err)
	}
	var found struct {
		Group  string `json:"group"`
		User   string `json:"user"`
		Groups int    `json:"groups"`
		Users  int    `json:"users"`
	}
	if err := json.Unmarshal([]byte(raw), &found); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	t.Logf("chats: %d group(s), %d individual(s)", found.Groups, found.Users)
	if found.Group == "" {
		t.Skip("this account has no group chat, so the group branch cannot be proven here")
	}

	got, err := send.Resolve(ctx, runner, eval, found.Group, "group/resolve")
	if err != nil {
		t.Fatalf("Resolve(group): %v — before the fix this failed with "+
			"NOT_ON_WHATSAPP, because queryWidExists resolves people and a group "+
			"is not one", err)
	}
	t.Logf("group resolved: %s", got)
	if !got.IsGroup {
		t.Fatal("the group was resolved through the INDIVIDUAL path; the two must " +
			"stay distinguishable, because only one of them has a lid")
	}
	if got.JID != found.Group {
		t.Fatal("a group jid was rewritten during resolution: a group IS its own " +
			"identity and has no lid counterpart to be replaced by")
	}

	// The individual path must keep behaving as before, and must NOT report
	// itself as a group.
	if found.User != "" {
		u, err := send.Resolve(ctx, runner, eval, found.User, "group/resolve-user")
		if err != nil {
			t.Fatalf("Resolve(individual): %v", err)
		}
		t.Logf("individual resolved: %s", u)
		if u.IsGroup {
			t.Fatal("an individual came back flagged as a group")
		}
		if u.JID == "" {
			t.Fatal("an individual resolved to an empty identity")
		}
	}
}

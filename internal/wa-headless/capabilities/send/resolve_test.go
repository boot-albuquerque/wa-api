package send

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// resolveDouble answers the resolve state script. It exists because the live
// proof of the group path cannot run in the gate — a group has members, and a
// test that sends to check a RESOLUTION is a test nobody can run — so the
// branches still need exercising somewhere cheap.
type resolveDouble struct {
	ok      bool
	why     string
	jid     string
	isGroup bool
	kicks   int
}

func (p *resolveDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			*out = fmt.Sprintf(`{"stage":"done","ok":false,"why":%q}`, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","jid":%q,"is_group":%t}`,
			p.jid, p.isGroup)
		return nil
	}
	p.kicks++
	*out = `{"started":true}`
	return nil
}

func resolver(p *resolveDouble) func(string) (Resolution, error) {
	return func(jid string) (Resolution, error) {
		return Resolve(context.Background(), engine.NewRunner(), p.eval, jid, "t/resolve")
	}
}

// TestResolveKeepsAGroupJidIntact. A group IS its own identity: there is no lid
// counterpart to replace it with, and rewriting it would address the message
// somewhere else.
func TestResolveKeepsAGroupJidIntact(t *testing.T) {
	compressClock(t)
	const groupJID = "120363000000000000@g.us"
	got, err := resolver(&resolveDouble{ok: true, jid: groupJID, isGroup: true})(groupJID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got.IsGroup {
		t.Fatal("a group came back flagged as an individual")
	}
	if got.JID != groupJID {
		t.Fatalf("the group jid was rewritten to %q", got.JID)
	}
}

// TestResolveReturnsTheServersIdentityForAPerson is the other branch: an
// individual IS rewritten, from the phone jid the caller typed to the lid the
// server returned (H34).
func TestResolveReturnsTheServersIdentityForAPerson(t *testing.T) {
	compressClock(t)
	got, err := resolver(&resolveDouble{ok: true, jid: lidJID, isGroup: false})(phoneJID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.IsGroup {
		t.Fatal("an individual came back flagged as a group")
	}
	if got.JID != lidJID {
		t.Fatalf("JID=%q, want the resolved %q — the caller's phone jid is not the "+
			"identity this build addresses", got.JID, lidJID)
	}
}

// TestResolveFailureCarriesThePagesReason. Before the group branch existed, a
// group produced NOT_ON_WHATSAPP; a reason that did not travel would have made
// that defect much harder to find (H48).
func TestResolveFailureCarriesThePagesReason(t *testing.T) {
	compressClock(t)
	_, err := resolver(&resolveDouble{ok: false, why: "GROUP_NOT_FOUND"})("120363@g.us")
	if !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if !strings.Contains(err.Error(), "GROUP_NOT_FOUND") {
		t.Fatalf("the page's reason was dropped: %v", err)
	}
}

func TestResolveRedactsTheIdentity(t *testing.T) {
	r := Resolution{JID: "5541999998888@c.us", IsGroup: false}
	s := r.String()
	if strings.Contains(s, "5541999998888") {
		t.Fatalf("String() leaked the identity: %s", s)
	}
	if !strings.Contains(s, "resolved=true") {
		t.Fatalf("a resolved identity must say so without saying which: %s", s)
	}
	if empty := (Resolution{}).String(); !strings.Contains(empty, "resolved=false") {
		t.Fatalf("an unresolved one must be distinguishable: %s", empty)
	}
}

func TestResolveNeverAsksThePageForACancelledCaller(t *testing.T) {
	compressClock(t)
	p := &resolveDouble{ok: true, jid: lidJID}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, engine.NewRunner(), p.eval, phoneJID, "t/resolve"); err == nil {
		t.Fatal("a cancelled context produced a resolution")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

// TestTheResolverScriptCarriesTheGroupBranch. The live proof of this branch
// cannot run in the gate, so the one thing checkable here is that the branch
// the fix added is still in the script the senders share.
func TestTheResolverScriptCarriesTheGroupBranch(t *testing.T) {
	if !strings.Contains(resolveChatExpr, "g.us") {
		t.Fatal("resolveChatExpr no longer distinguishes a group; every group send " +
			"would go through the USER resolution, which answers NULL for one (H48)")
	}
	// And both senders must use the shared expression rather than a copy.
	for name, script := range map[string]string{
		"text":  dispatchScript(phoneJID, "hi"),
		"media": mediaScript(phoneJID, Media{MimeType: "image/png", Data: []byte{1}}),
	} {
		if !strings.Contains(script, "g.us") {
			t.Fatalf("the %s sender does not carry the group branch, so it has its "+
				"own copy of the resolution", name)
		}
	}
}

// TestResultRedactsTheRecipient closes a redaction surface that had no test:
// a send result names a message, and message ids are fine, but nothing else is.
func TestResultRedactsTheRecipient(t *testing.T) {
	r := Result{Waited: 2 * time.Millisecond}
	r.ID.ID = "3EB0ABC"
	s := r.String()
	if !strings.Contains(s, "3EB0ABC") {
		t.Fatalf("the message id is the point of the rendering: %s", s)
	}
	if strings.Contains(s, "@") {
		t.Fatalf("String() rendered something jid-shaped: %s", s)
	}
}

// TestKindOfRefusesWhatIsNeitherTextNorMedia. A revoked message, a system
// notification or a poll must never be accepted as proof that a send happened.
func TestKindOfRefusesWhatIsNeitherTextNorMedia(t *testing.T) {
	for _, typ := range []string{"notification_template", "revoked", "poll_creation", "e2e_notification", ""} {
		if got := kindOf(typ); got != kindAny {
			t.Fatalf("kindOf(%q)=%v, want kindAny — accepting it would let a system "+
				"message stand in for a message the account sent", typ, got)
		}
	}
	for _, typ := range []string{"image", "document", "video", "audio", "ptt", "sticker"} {
		if kindOf(typ) != kindMedia {
			t.Fatalf("kindOf(%q) is not media", typ)
		}
	}
	if kindOf("chat") != kindText {
		t.Fatal(`kindOf("chat") is not text`)
	}
}

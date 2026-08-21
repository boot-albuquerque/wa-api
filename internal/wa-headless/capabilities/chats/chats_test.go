package chats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	answer     string
	err        error
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	p.lastScript = expr
	if p.err != nil {
		return p.err
	}
	*out = p.answer
	return nil
}

func lister(p *pageDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func chatJSON(jid, title string, t int64, unread int, group bool) string {
	return fmt.Sprintf(`{"jid":%q,"title":%q,"t":%d,"unread":%d,"is_group":%t,`+
		`"archived":false,"pinned":false,"muted":false,"read_only":false}`,
		jid, title, t, unread, group)
}

func listJSON(total, withUnread int, chats ...string) string {
	return fmt.Sprintf(`{"ok":true,"total":%d,"with_unread":%d,"chats":[%s]}`,
		total, withUnread, strings.Join(chats, ","))
}

// TestTheTitleComesFromFormattedTitle is the finding this package exists around.
//
// WAWebChatGetters exports getName and it is the obvious reach — measured, it
// answered for 1 of 384 chats while formattedTitle answered for 384. A listing
// built on getName would show a title for one conversation in every 384 AND
// pass every unit test, because a double returns whatever it is told. So the
// assertion is on the SCRIPT: it is the only place a double cannot lie.
func TestTheTitleComesFromFormattedTitle(t *testing.T) {
	p := &pageDouble{answer: listJSON(1, 0, chatJSON("1@lid", "Ana", 100, 0, false))}
	if _, err := lister(p).List(context.Background(), 10, "t/list"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(p.lastScript, "formattedTitle") {
		t.Fatal("the listing does not read formattedTitle, which is the only field " +
			"measured present on every chat")
	}
	// A CALL, not a mention: the script's own comment explains why getName is
	// wrong, and the first version of this assertion failed on that comment.
	// Matching text where a call was meant is how a guard ends up policing
	// prose (H32).
	if strings.Contains(p.lastScript, "getName(") {
		t.Fatal("the listing CALLS getName, measured present on 1 of 384 chats (H39)")
	}
}

// TestMostRecentFirst is the ordering contract. The collection was measured
// newest-first, but nothing documents that — and inheriting an undocumented
// order is how the contact roster nearly shipped a listing that followed the
// page (H39).
func TestMostRecentFirst(t *testing.T) {
	p := &pageDouble{answer: listJSON(3, 0,
		chatJSON("old@lid", "A", 100, 0, false),
		chatJSON("new@lid", "B", 300, 0, false),
		chatJSON("mid@lid", "C", 200, 0, false),
	)}
	got, err := lister(p).List(context.Background(), 10, "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"new@lid", "mid@lid", "old@lid"}
	for i, w := range want {
		if got.Chats[i].JID != w {
			t.Fatalf("position %d is %q, want %q (full: %v)", i, got.Chats[i].JID, w, got.Chats)
		}
	}
}

// TestTiesBreakStably: two conversations can share a timestamp, and a list that
// ordered them differently between calls would be one no caller can diff.
func TestTiesBreakStably(t *testing.T) {
	answer := listJSON(3, 0,
		chatJSON("c@lid", "C", 100, 0, false),
		chatJSON("a@lid", "A", 100, 0, false),
		chatJSON("b@lid", "B", 100, 0, false),
	)
	var first string
	for i := 0; i < 20; i++ {
		got, err := lister(&pageDouble{answer: answer}).List(context.Background(), 10, "t/list")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		var keys []string
		for _, c := range got.Chats {
			keys = append(keys, c.JID)
		}
		joined := strings.Join(keys, "|")
		if first == "" {
			first = joined
			continue
		}
		if joined != first {
			t.Fatalf("run %d ordered ties differently: %s vs %s", i, joined, first)
		}
	}
	if first != "a@lid|b@lid|c@lid" {
		t.Fatalf("ties did not break on the identity: %s", first)
	}
}

// TestTheLimitKeepsTheMostRecent. A limit applied before ordering would return
// whichever chats the page happened to hold first, which is not what "the ten
// most recent" means.
func TestTheLimitKeepsTheMostRecent(t *testing.T) {
	p := &pageDouble{answer: listJSON(3, 0,
		chatJSON("old@lid", "A", 100, 0, false),
		chatJSON("new@lid", "B", 300, 0, false),
		chatJSON("mid@lid", "C", 200, 0, false),
	)}
	got, err := lister(p).List(context.Background(), 2, "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Chats) != 2 || got.Chats[0].JID != "new@lid" || got.Chats[1].JID != "mid@lid" {
		t.Fatalf("the limit did not keep the most recent: %v", got.Chats)
	}
	if !got.Truncated() {
		t.Fatal("Truncated()=false while 2 of 3 were returned")
	}
}

// TestWithUnreadCountsTheWHOLEStore, not the returned page. A caller asking
// "is anything waiting" is not asking about the first ten conversations.
func TestWithUnreadCountsTheWholeStore(t *testing.T) {
	p := &pageDouble{answer: listJSON(384, 122,
		chatJSON("a@lid", "A", 300, 0, false),
		chatJSON("b@lid", "B", 200, 5, false),
	)}
	got, err := lister(p).List(context.Background(), 1, "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.WithUnread != 122 {
		t.Fatalf("WithUnread=%d, want 122 — computing it from the returned page "+
			"would answer a different question", got.WithUnread)
	}
	if got.Total != 384 {
		t.Fatalf("Total=%d, want 384", got.Total)
	}
}

func TestGroupsAreMarkedAsSuch(t *testing.T) {
	p := &pageDouble{answer: listJSON(2, 0,
		chatJSON("120363@g.us", "Grupo", 200, 0, true),
		chatJSON("1@lid", "Ana", 100, 0, false),
	)}
	got, _ := lister(p).List(context.Background(), 10, "t/list")
	if !got.Chats[0].IsGroup || got.Chats[1].IsGroup {
		t.Fatalf("group flags are wrong: %v", got.Chats)
	}
}

func TestTheTitleIsNeverRendered(t *testing.T) {
	c := Chat{JID: "5541999998888@c.us", Title: "Ana Silva"}
	s := c.String()
	for _, secret := range []string{"Ana", "Silva", "5541999998888"} {
		if strings.Contains(s, secret) {
			t.Fatalf("String() leaked %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "title=true") {
		t.Fatalf("a chat with a title must say so without saying it: %s", s)
	}
}

func TestAnUnavailableCollectionIsAnError(t *testing.T) {
	p := &pageDouble{answer: `{"ok":false}`}
	if _, err := lister(p).List(context.Background(), 10, "t/list"); !errors.Is(err, ErrNoCollection) {
		t.Fatalf("got %v, want ErrNoCollection", err)
	}
}

func TestCancelledContextIsNotAnEmptyList(t *testing.T) {
	p := &pageDouble{answer: listJSON(1, 0, chatJSON("a@lid", "A", 100, 0, false))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lister(p).List(ctx, 10, "t/list"); err == nil {
		t.Fatal("a cancelled context produced a chat list")
	}
}

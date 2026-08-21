package chats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// THE CAPABILITY DOES NOT WORK AGAINST THE LIVE BUILD (H78). These tests lock
// the MEASUREMENTS — which primitive is driven, which field is read, and that a
// real change is never claimed as verified — so the next attempt starts from
// what was learned instead of from the two dead ends.

type unreadDouble struct {
	ok            bool
	stage, why    string
	before, after int
	already       bool

	kicks      int
	lastScript string
}

func (p *unreadDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","before":%d,"after":%d,"already":%t}`,
			p.before, p.after, p.already)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func unreader(p *unreadDouble) *Lister { return New(engine.NewRunner(), p.eval) }

// TestTheDiscardedPrimitiveIsNotBack. sendConversationSeen with a negative delta
// was the first guess and it does nothing — measured across sessions, 0 -> 0.
// It came from supposing that marking unread was marking read with the number
// inverted, and that supposition must not return.
func TestTheDiscardedPrimitiveIsNotBack(t *testing.T) {
	p := &unreadDouble{ok: true}
	if _, err := unreader(p).MarkUnread(context.Background(), "1@c.us", "t"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if strings.Contains(p.lastScript, "sendConversationSeen") {
		t.Fatal("the discarded primitive is back; it was measured doing nothing")
	}
	if strings.Contains(p.lastScript, "unreadDelta") {
		t.Fatal("unreadDelta is back; marking unread is not marking read with a negative number")
	}
	if !strings.Contains(p.lastScript, "Cmd.markChatUnread(chat, true)") {
		t.Fatal("the script does not drive the verb the app's own caller uses")
	}
}

// TestTheFieldIsMarkedUnreadNotTheCount. chat.markedUnread is the flag; the
// count is a different question and answering one with the other is what the
// first version did.
func TestTheFieldIsMarkedUnreadNotTheCount(t *testing.T) {
	p := &unreadDouble{ok: true}
	if _, err := unreader(p).MarkUnread(context.Background(), "1@c.us", "t"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if !strings.Contains(p.lastScript, "chat.markedUnread === true") {
		t.Fatal("the no-op check does not consult the flag")
	}
	if !strings.Contains(unreadCountScript("1@c.us"), "chat.markedUnread === true") {
		t.Fatal("the count does not distinguish a deliberate mark")
	}
}

// TestARealChangeIsNeverClaimedVerified. The count a session reads is stale with
// respect to its own writes — measured: 22 in-session against 0 in a fresh one.
func TestARealChangeIsNeverClaimedVerified(t *testing.T) {
	p := &unreadDouble{ok: true, before: 0, after: 0}
	got, err := unreader(p).MarkUnread(context.Background(), "1@c.us", "t")
	if err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if got.Verified {
		t.Fatalf("a real change claims to be verified: %s", got)
	}
	if !strings.Contains(got.String(), "verified=false") {
		t.Fatalf("the rendering hides that nothing was confirmed: %s", got)
	}
	// A no-op IS confirmable: nothing had to move.
	q := &unreadDouble{ok: true, already: true, before: -1, after: -1}
	noop, err := unreader(q).MarkUnread(context.Background(), "1@c.us", "t")
	if err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if !noop.NoOp || !noop.Verified {
		t.Fatalf("a no-op is not reported as confirmed: %s", noop)
	}
}

// TestTheScriptDoesNotWaitOnSomethingThatNeverMoves.
func TestTheScriptDoesNotWaitOnSomethingThatNeverMoves(t *testing.T) {
	p := &unreadDouble{ok: true}
	if _, err := unreader(p).MarkUnread(context.Background(), "1@c.us", "t"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the script waits on a signal this build never sends")
	}
	if strings.Contains(markUnreadResultScript, "settling") {
		t.Fatal("the result script still has a settling branch")
	}
}

func TestUnreadCountReadsAndRefuses(t *testing.T) {
	t.Run("counts", func(t *testing.T) {
		l := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = "7"
			return nil
		})
		n, err := l.UnreadCount(context.Background(), "1@c.us", "t")
		if err != nil || n != 7 {
			t.Fatalf("got (%d, %v), want (7, nil)", n, err)
		}
	})
	t.Run("no chat", func(t *testing.T) {
		l := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = "NO_CHAT"
			return nil
		})
		if _, err := l.UnreadCount(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNoSuchChat) {
			t.Fatalf("got %v, want ErrNoSuchChat", err)
		}
	})
	t.Run("garbage", func(t *testing.T) {
		l := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = "sete"
			return nil
		})
		if _, err := l.UnreadCount(context.Background(), "1@c.us", "t"); err == nil {
			t.Fatal("a non-numeric answer was accepted as a count")
		}
	})
	t.Run("empty jid", func(t *testing.T) {
		called := false
		l := New(engine.NewRunner(), func(context.Context, string, *string) error {
			called = true
			return nil
		})
		if _, err := l.UnreadCount(context.Background(), "  ", "t"); !errors.Is(err, ErrNoSuchChat) {
			t.Fatalf("got %v, want ErrNoSuchChat", err)
		}
		if called {
			t.Fatal("the page was asked about an empty jid")
		}
	})
}

func TestMarkUnreadRefusalsAndFailures(t *testing.T) {
	empty := &unreadDouble{ok: true}
	if _, err := unreader(empty).MarkUnread(context.Background(), "  ", "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	if empty.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", empty.kicks)
	}
	nc := &unreadDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := unreader(nc).MarkUnread(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	boom := &unreadDouble{ok: false, stage: "apply", why: "boom"}
	if _, err := unreader(boom).MarkUnread(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("got %v, want ErrLifecycle", err)
	}
}

func TestCancelledContextMarksNothing(t *testing.T) {
	p := &unreadDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := unreader(p).MarkUnread(ctx, "1@c.us", "t"); err == nil {
		t.Fatal("a cancelled context marked a chat unread")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

package chats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type lifeDouble struct {
	ok         bool
	stage, why string
	count      int

	kicks      int
	lastScript string
}

func (p *lifeDouble) eval(ctx context.Context, expr string, out *string) error {
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","count":%d}`, p.count)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func lifeLister(p *lifeDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func compressLifeClock(t *testing.T) {
	t.Helper()
	ob, ot := lifecycleBudget, lifecycleTick
	lifecycleBudget, lifecycleTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { lifecycleBudget, lifecycleTick = ob, ot })
}

const someChat = "15550001111@c.us"

// TestTheMeasuredShapesAreUsed. Both are synchronous and both were confirmed at
// the app's own call sites, so there is no excuse for guessing either.
func TestTheMeasuredShapesAreUsed(t *testing.T) {
	compressLifeClock(t)
	c := &lifeDouble{ok: true, count: 12}
	if _, err := lifeLister(c).Clear(context.Background(), someChat, true, "t"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if !strings.Contains(c.lastScript, "A.sendClear(chat, true)") {
		t.Fatal("clear does not use the measured two-positional shape")
	}
	d := &lifeDouble{ok: true, count: 12}
	if err := lifeLister(d).Delete(context.Background(), someChat, "t"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !strings.Contains(d.lastScript, "A.sendDelete(chat, true)") {
		t.Fatal("delete does not pass its second argument explicitly")
	}
}

// TestKeepStarredIsTheCallersChoice. Sparing starred messages and destroying
// them are different intentions, and the destructive one must not be a default.
func TestKeepStarredIsTheCallersChoice(t *testing.T) {
	compressLifeClock(t)
	on := &lifeDouble{ok: true, count: 3}
	got, err := lifeLister(on).Clear(context.Background(), someChat, true, "t")
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if !got.KeptStarred || !strings.Contains(on.lastScript, "sendClear(chat, true)") {
		t.Fatalf("keepStarred=true did not reach the call: %s", got)
	}
	off := &lifeDouble{ok: true, count: 3}
	got2, err := lifeLister(off).Clear(context.Background(), someChat, false, "t")
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got2.KeptStarred || !strings.Contains(off.lastScript, "sendClear(chat, false)") {
		t.Fatalf("keepStarred=false did not reach the call: %s", got2)
	}
}

// TestDeleteDoesNotLeaveGroupsSilently. The app's flow is sendExitGroup then
// sendDelete; doing the first on a caller's behalf would decide something they
// did not ask for, and deleting a group chat while staying in the group is a
// real state somebody may want.
func TestDeleteDoesNotLeaveGroupsSilently(t *testing.T) {
	compressLifeClock(t)
	p := &lifeDouble{ok: true}
	if err := lifeLister(p).Delete(context.Background(), "120363000000000000@g.us", "t"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if strings.Contains(p.lastScript, "sendExitGroup(") {
		t.Fatal("deleting a group chat also leaves the group, which the caller did not ask for")
	}
}

// TestTheCountIsTakenBEFORE. After is meaningless — the point of both acts is
// that there is nothing left to count.
func TestTheCountIsTakenBefore(t *testing.T) {
	compressLifeClock(t)
	p := &lifeDouble{ok: true, count: 42}
	got, err := lifeLister(p).Clear(context.Background(), someChat, true, "t")
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got.MessagesBefore != 42 {
		t.Fatalf("the count was not carried: %s", got)
	}
	if strings.Index(p.lastScript, "count++") > strings.Index(p.lastScript, "A.sendClear(") {
		t.Fatal("the count is taken after the clear, when there is nothing to count")
	}
}

// TestNoContentIsReported. A conversation's messages are content.
func TestNoContentIsReported(t *testing.T) {
	s := Emptied{MessagesBefore: 42, KeptStarred: true}.String()
	if !strings.Contains(s, "messagesBefore=42") {
		t.Fatalf("the count is missing: %s", s)
	}
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
}

func TestLifecycleRefusalsCostNoPageCall(t *testing.T) {
	compressLifeClock(t)
	p := &lifeDouble{ok: true}
	if _, err := lifeLister(p).Clear(context.Background(), "  ", true, "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	if err := lifeLister(p).Delete(context.Background(), "   ", "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) to destroy something for an empty jid", p.kicks)
	}
}

func TestAnUnloadedChatIsItsOwnAnswer(t *testing.T) {
	compressLifeClock(t)
	p := &lifeDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := lifeLister(p).Clear(context.Background(), someChat, true, "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	q := &lifeDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if err := lifeLister(q).Delete(context.Background(), someChat, "t"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
}

func TestCancelledContextDestroysNothing(t *testing.T) {
	compressLifeClock(t)
	p := &lifeDouble{ok: true, count: 5}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lifeLister(p).Clear(ctx, someChat, true, "t"); err == nil {
		t.Fatal("a cancelled context emptied a conversation")
	}
	if err := lifeLister(p).Delete(ctx, someChat, "t"); err == nil {
		t.Fatal("a cancelled context deleted a conversation")
	}
	if p.kicks != 0 {
		t.Fatalf("destroyed something %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAHungPageIsATimeoutNotASilentSuccess(t *testing.T) {
	compressLifeClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	l := New(engine.NewRunner(), stuck)
	if _, err := l.Clear(context.Background(), someChat, true, "t"); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("got %v, want ErrLifecycle", err)
	}
}

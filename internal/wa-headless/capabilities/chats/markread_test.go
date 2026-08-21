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

type markDouble struct {
	ok            bool
	stage         string
	why           string
	before, after int
	kicks         int
	lastScript    string
}

func (p *markDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "seen"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","before":%d,"after":%d}`, p.before, p.after)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func marker(p *markDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func compressMarkClock(t *testing.T) {
	t.Helper()
	ob, ot := markBudget, markTick
	markBudget, markTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { markBudget, markTick = ob, ot })
}

// TestAConversationStillUnreadIsAFailure is the postcondition, and the reason
// this verifies at all: the page accepting the call is not the account having
// acknowledged anything, and nothing else would tell the caller.
func TestAConversationStillUnreadIsAFailure(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 5, after: 5}
	_, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark")
	if !errors.Is(err, ErrStillUnread) {
		t.Fatalf("got %v, want ErrStillUnread", err)
	}
	if !strings.Contains(err.Error(), "5 before") {
		t.Fatalf("the error must carry the counts: %v", err)
	}
}

// TestNothingToAcknowledgeIsSuccess. An idle conversation is the ordinary case,
// and reporting it as a failure would make every quiet chat look broken.
func TestNothingToAcknowledgeIsSuccess(t *testing.T) {
	compressMarkClock(t)
	got, err := marker(&markDouble{ok: true, before: 0, after: 0}).
		MarkRead(context.Background(), "1@lid", "t/mark")
	if err != nil {
		t.Fatalf("an already-read conversation produced an error: %v", err)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for a conversation with nothing unread: %s", got)
	}
}

func TestAcknowledgingReportsWhatItCleared(t *testing.T) {
	compressMarkClock(t)
	got, err := marker(&markDouble{ok: true, before: 7, after: 0}).
		MarkRead(context.Background(), "1@lid", "t/mark")
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if !got.Changed() || got.Before != 7 || got.After != 0 {
		t.Fatalf("the counts were lost: %s", got)
	}
}

// TestTheAcknowledgementNamesWhatWasRead. sendConversationSeen takes the chat's
// lastReceivedKey; acknowledging without naming the message is not something
// the protocol offers, so a call that dropped it would be asking for something
// that does not exist.
func TestTheAcknowledgementNamesWhatWasRead(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 3, after: 0}
	if _, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if !strings.Contains(p.lastScript, "lastReceivedKey") {
		t.Fatal("the acknowledgement does not name the message it acknowledges")
	}
	if !strings.Contains(p.lastScript, "sendConversationSeen({") {
		t.Fatal("sendConversationSeen is not called with a single object; the app's " +
			"own call passes {chat, key, threadId, unreadDelta}")
	}
}

// TestAnIdleConversationIsNotAcknowledgedAtAll. Sending a receipt for a message
// the account may not have is a real outward effect, and there is nothing to
// gain from it.
func TestAnIdleConversationIsNotAcknowledgedAtAll(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 0, after: 0}
	if _, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	// The guard lives in the page script, so the script is where it is checked.
	if !strings.Contains(p.lastScript, "before === 0") {
		t.Fatal("the script has no early exit for a conversation with nothing unread")
	}
}

// TestTheChatIsResolvedBeforeLookup. This build files chats under the identity
// the server assigns, so a lookup by the number a caller typed finds nothing
// (H34, H39) — which would report ErrNoSuchChat for a conversation that exists.
func TestTheChatIsResolvedBeforeLookup(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 1, after: 0}
	if _, err := marker(p).MarkRead(context.Background(), "5541999998888@c.us", "t/mark"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	// THE CALL, not the definition. The first version looked for
	// "queryWidExists", which appears inside spa.ResolveIdentityExpr and is
	// therefore present even when the resolution is never INVOKED — the control
	// that removed the call passed, which is how this was caught. Second time
	// today that a guard matched prose instead of behaviour (see the chat
	// listing's getName assertion).
	if !strings.Contains(p.lastScript, "await resolveIdentity(") {
		t.Fatal("the chat is looked up without CALLING the identity resolution first; " +
			"this build files chats under the identity the server assigns, so a " +
			"lookup by the caller's number finds nothing (H34, H39)")
	}
}

func TestAMissingConversationIsErrNoSuchChat(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
}

func TestABlankJIDNeverReachesThePage(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true}
	if _, err := marker(p).MarkRead(context.Background(), "  ", "t/mark"); !errors.Is(err, ErrNoSuchChat) {
		t.Fatalf("got %v, want ErrNoSuchChat", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) with no conversation", p.kicks)
	}
}

func TestCancelledContextSendsNoReceipt(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 5, after: 0}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := marker(p).MarkRead(ctx, "1@lid", "t/mark"); err == nil {
		t.Fatal("a cancelled context sent a read receipt")
	}
	if p.kicks != 0 {
		t.Fatalf("sent %d receipt(s) for a caller that had given up", p.kicks)
	}
}

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

// withoutComments strips // comments before a script is asserted against.
//
// IT EXISTS BECAUSE A GUARD MATCHED ITS OWN COMMENT — the eleventh time in this
// repository, and the second today. The order assertion below failed against a
// COMMENT explaining the order.
func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// THE PRIMITIVE CHANGED, AND THE OLD ASSERTION WENT WITH IT.
//
// This test used to require lastReceivedKey in the call, because
// sendConversationSeen takes it. That was a correct assertion about the wrong
// primitive: measured side by side on the same chat, sendConversationSeen left
// unreadCount at 1, and WAWebUpdateUnreadChatAction.sendSeen — which is what the
// reference calls — took it from 1 to 0 (H160). The row was demoted in H82 for
// exactly the counter that did not move.
//
// The new call names no message: sendSeen({chat, threadId}) acknowledges the
// conversation, and requiring a key it does not take would be asking for
// something that does not exist — the same reasoning the old comment gave, now
// pointing at the call that works.
func TestTheAcknowledgementUsesThePrimitiveThatMoves(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 3, after: 0}
	if _, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	code := withoutComments(p.lastScript)
	if !strings.Contains(code, "WAWebUpdateUnreadChatAction") {
		t.Fatal("the acknowledgement does not use WAWebUpdateUnreadChatAction, the " +
			"only primitive measured to move unreadCount on this build")
	}
	if !strings.Contains(code, "sendSeen({") {
		t.Fatal("sendSeen is not called with a single object; the reference passes " +
			"{chat, threadId}")
	}
	if strings.Contains(code, "sendConversationSeen") {
		t.Fatal("the old primitive is still called; it leaves unreadCount untouched " +
			"and is what made this row's postcondition fail")
	}
}

// THE PRESENCE ANNOUNCEMENT IS UNDONE EVEN WHEN THE ACKNOWLEDGEMENT THROWS.
//
// markAvailable announces this client as present. Leaving that on after a failed
// call is an outward-facing side effect nothing here asked for, and it would
// outlive the error — so the undo lives in a finally, not after the call.
//
// It is asserted on the SCRIPT because a double supplies the outcome either way,
// and this is a rule about ORDER: inverting it passes every other test.
func TestThePresenceAnnouncementIsUndoneOnFailure(t *testing.T) {
	compressMarkClock(t)
	p := &markDouble{ok: true, before: 3, after: 0}
	if _, err := marker(p).MarkRead(context.Background(), "1@lid", "t/mark"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	code := withoutComments(p.lastScript)
	if !strings.Contains(code, "markAvailable") || !strings.Contains(code, "markUnavailable") {
		t.Fatal("the call is not bracketed by the stream presence the reference uses")
	}
	iFinally := strings.Index(code, "} finally {")
	if iFinally < 0 {
		t.Fatal("the presence undo is not in a finally, so a failed acknowledgement " +
			"would leave this client announced as online")
	}
	if !strings.Contains(code[iFinally:], "markUnavailable") {
		t.Fatal("markUnavailable is not inside the finally, so a failed " +
			"acknowledgement would leave this client announced as online")
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

// TestMarkResultRendersItsCounts. An untested rendering is how a reporting
// surface silently loses the number a caller reads.
func TestMarkResultRendersItsCounts(t *testing.T) {
	r := MarkResult{Before: 7, After: 0, Waited: 2 * time.Second}
	s := r.String()
	if !strings.Contains(s, "before=7") || !strings.Contains(s, "after=0") {
		t.Fatalf("the counts are missing: %s", s)
	}
	if !strings.Contains(s, "changed=true") {
		t.Fatalf("an acknowledgement that cleared something must say so: %s", s)
	}
	idle := MarkResult{}
	if !strings.Contains(idle.String(), "changed=false") {
		t.Fatalf("an idle conversation must be distinguishable: %s", idle)
	}
}

// TestAStalledPageIsNotAnAcknowledgement: accepting the kick and never settling
// must not be reported as a conversation marked read.
func TestAStalledPageIsNotAnAcknowledgement(t *testing.T) {
	compressMarkClock(t)
	p := &stallingMarkDouble{}
	_, err := New(engine.NewRunner(), p.eval).MarkRead(context.Background(), "1@lid", "t/mark")
	if !errors.Is(err, ErrMarkRead) {
		t.Fatalf("got %v, want ErrMarkRead", err)
	}
	if !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a stall must be distinguishable from a refusal: %v", err)
	}
}

type stallingMarkDouble struct{}

func (p *stallingMarkDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		*out = `{"stage":"pending","ok":false,"why":""}`
		return nil
	}
	*out = `{"started":true}`
	return nil
}

package forward

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	ok               bool
	stage, why       string
	newID            string
	bodyLen          int
	settleAfterReads int

	reads      int
	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		p.reads++
		switch {
		case !p.ok:
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
		case p.reads <= p.settleAfterReads:
			*out = `{"stage":"settling","ok":false,"why":""}`
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","newID":%q,"bodyLen":%d}`,
				p.newID, p.bodyLen)
		}
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func fwd(p *pageDouble) *Forwarder { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := forwardBudget, forwardTick
	forwardBudget, forwardTick = 300*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { forwardBudget, forwardTick = ob, ot })
}

const (
	srcID   = "3EB0000000000000000001"
	chatJID = "15550001111@c.us"
)

// TestTheCallSiteShapeIsUsed. Neither exported function's toString() shows more
// than an opaque single argument, and the chat model has no forward method — so
// the shape was read from the app's own call site, and this test freezes it.
func TestTheCallSiteShapeIsUsed(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, newID: "3EB0000000000000000002", bodyLen: 10}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); err != nil {
		t.Fatalf("To: %v", err)
	}
	if !strings.Contains(p.lastScript, "F.forwardMessagesToChats({") {
		t.Fatal("the call does not use the measured object shape")
	}
	for _, key := range []string{"msgs: [msg]", "chats: [chat]", "includeCaption:"} {
		if !strings.Contains(p.lastScript, key) {
			t.Fatalf("the call site key %q is missing", key)
		}
	}
}

// TestItDoesNotOpenAConversation. findOrCreateLatestChat is what the app's own
// forward flow uses; this package deliberately does not, because opening a
// conversation with somebody in order to re-send them a message is a bigger act
// than the caller asked for.
func TestItDoesNotOpenAConversation(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	// The needle is the CALL, not the name: the package comment mentions
	// findOrCreateLatestChat, and matching the bare name would find the prose.
	if strings.Contains(p.lastScript, "findOrCreateLatestChat(") {
		t.Fatal("the script creates a chat that did not exist")
	}
	if !strings.Contains(p.lastScript, "if (!chat) { park(") {
		t.Fatal("the missing chat is looked up but does not guard a return")
	}
}

// TestTheCopyIsIdentifiedByAnIdSetNotATimestamp is the postcondition's shape,
// and it is strict because a timestamp comparison in this module once accepted a
// stranger's message that arrived 23 seconds BEFORE the send.
func TestTheCopyIsIdentifiedByAnIdSetNotATimestamp(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, newID: "3EB0000000000000000002", bodyLen: 10}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); err != nil {
		t.Fatalf("To: %v", err)
	}
	if !strings.Contains(p.lastScript, "const seen = new Set()") {
		t.Fatal("the script does not record the ids already in the target chat")
	}
	// And the result script has to use all three conditions.
	for _, cond := range []string{"m.id.fromMe", "s.chatKey", "s.seen.has(m.id.id)"} {
		if !strings.Contains(resultScript, cond) {
			t.Fatalf("the copy check is missing the %q condition", cond)
		}
	}
	if strings.Contains(resultScript, "m.t >") || strings.Contains(resultScript, "m.t >=") {
		t.Fatal("the copy is identified by timestamp, which is the check that produced a false positive")
	}
}

// TestNoCopyIsAFailure.
func TestNoCopyIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, newID: "", bodyLen: 0}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); !errors.Is(err, ErrNotDelivered) {
		t.Fatalf("got %v, want ErrNotDelivered", err)
	}
}

// TestACopyThatNeverAppearsIsNotDeliveredNotATimeout.
func TestACopyThatNeverAppearsIsNotDeliveredNotATimeout(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, settleAfterReads: 1 << 30}
	err := errFrom(fwd(p).To(context.Background(), srcID, chatJID, true, "t"))
	if !errors.Is(err, ErrNotDelivered) {
		t.Fatalf("got %v, want ErrNotDelivered", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("an undelivered forward is reported as a hung page: %v", err)
	}
}

// TestTheAwaitIsNotTheCompletion — applied before it could cost a live failure,
// which is the whole point of writing H61 down.
func TestTheAwaitIsNotTheCompletion(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, newID: "3EB0000000000000000002", bodyLen: 10, settleAfterReads: 2}
	got, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t")
	if err != nil {
		t.Fatalf("To: %v", err)
	}
	if got.NewID == "" {
		t.Fatalf("the settled copy was not picked up: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the first answer was accepted instead of waited on", p.reads)
	}
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
}

// TestTheReasonsFieldSurvives. The app's forward errors carry a reasons field,
// and it is the only part a caller can act on.
func TestTheReasonsFieldSurvives(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "apply", why: "failed reasons=[\"BLOCKED\"]"}
	_, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t")
	if err == nil || !strings.Contains(err.Error(), "BLOCKED") {
		t.Fatalf("the reasons were flattened away: %v", err)
	}
	if !strings.Contains(forwardScript(srcID, chatJID, true), "e.reasons") {
		t.Fatal("the script does not read the reasons field off the error")
	}
}

// TestANonForwardableMessageIsItsOwnAnswer.
func TestANonForwardableMessageIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_FORWARDABLE"}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); !errors.Is(err, ErrNotForwardable) {
		t.Fatalf("got %v, want ErrNotForwardable", err)
	}
	if !strings.Contains(p.lastScript, "if (msg.canForward === false) {") {
		t.Fatal("the script does not consult the message's own forwardability")
	}
}

// TestIncludeCaptionIsTheCallersChoice. Forwarding media with and without its
// caption are different acts, and defaulting silently would be choosing what
// other people read.
func TestIncludeCaptionIsTheCallersChoice(t *testing.T) {
	compressClock(t)
	on := &pageDouble{ok: true, newID: "x"}
	if _, err := fwd(on).To(context.Background(), srcID, chatJID, true, "t"); err != nil {
		t.Fatalf("To: %v", err)
	}
	if !strings.Contains(on.lastScript, "includeCaption: true") {
		t.Fatal("includeCaption=true did not reach the call")
	}
	off := &pageDouble{ok: true, newID: "x"}
	if _, err := fwd(off).To(context.Background(), srcID, chatJID, false, "t"); err != nil {
		t.Fatalf("To: %v", err)
	}
	if !strings.Contains(off.lastScript, "includeCaption: false") {
		t.Fatal("includeCaption=false did not reach the call")
	}
}

func TestNoContentIsReported(t *testing.T) {
	s := Result{NewID: "3EB0", BodyLen: 45}.String()
	if !strings.Contains(s, "bodyLen=45") {
		t.Fatalf("the length is missing: %s", s)
	}
	if strings.Contains(s, "body=") && !strings.Contains(s, "bodyLen=") {
		t.Fatalf("the rendering carries a body: %s", s)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	for _, tc := range []struct {
		name, msg, chat string
		want            error
	}{
		{"no message", "  ", chatJID, ErrNoMessage},
		{"no chat", srcID, "   ", ErrNoChat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pageDouble{ok: true}
			if _, err := fwd(p).To(context.Background(), tc.msg, tc.chat, true, "t"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if p.kicks != 0 {
				t.Fatalf("the page was asked %d time(s)", p.kicks)
			}
		})
	}
}

func TestAMissingSourceIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := fwd(p).To(context.Background(), srcID, chatJID, true, "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
}

func TestCancelledContextForwardsNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, newID: "x"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fwd(p).To(ctx, srcID, chatJID, true, "t"); err == nil {
		t.Fatal("a cancelled context forwarded a message")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func errFrom(_ Result, err error) error { return err }

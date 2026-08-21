package chatstate

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
	ok       bool
	stage    string
	why      string
	was      bool
	jid      string
	pinned   int
	pinLimit int

	// value is what the verify read reports. It is SEPARATE from ok, because
	// "the page accepted the call" and "the flag moved" are the two things this
	// package exists to keep apart.
	value bool

	kicks      int
	verifies   int
	lastScript string
	lastVerify string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// EXPLICIT, UNIQUE MARKERS. Four scripts run here and three of them share
	// substrings — the set kick embeds ResolveIdentityExpr, which contains
	// createWid(, and both read scripts carry the read key. Routing on anything
	// less than a marker unique to one script has now broken this double twice,
	// each time failing an assertion for a reason unrelated to the code.
	switch {
	case strings.Contains(expr, readKey+`"] = null`):
		p.verifies++
		p.lastVerify = expr
		*out = `"reading"`
		return nil
	case strings.Contains(expr, `JSON.stringify(window["`+readKey+`"])`):
		*out = fmt.Sprintf(`{"found":true,"value":%t}`, p.value)
		return nil
	case strings.Contains(expr, "const s = window["):
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q,"was":%t,"pinned":%d,"pin_limit":%d}`,
				stage, p.why, p.was, p.pinned, p.pinLimit)
			return nil
		}
		// `after` is what the WRITE script reports, read from the chat object it
		// just changed. It is separate from `was` so a double can express the
		// case this package exists to catch: the page accepted the call and the
		// conversation did not move.
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","was":%t,"after":%t,"jid":%q}`,
			p.was, p.value, p.jid)
		return nil
	default:
		p.kicks++
		p.lastScript = expr
		*out = `{"started":true}`
		return nil
	}
}

func setter(p *pageDouble) *Setter { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := stateBudget, stateTick
	stateBudget, stateTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { stateBudget, stateTick = ob, ot })
}

const (
	phoneJID = "5541999998888@c.us"
	lidJID   = "123456789@lid"
)

// TestAFlagThatDoesNotMoveIsAFailure is the postcondition. The page accepting
// the call is not the conversation having changed, and this is one of the few
// operations here where that difference is observable — so it is checked.
func TestAFlagThatDoesNotMoveIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: false}
	_, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch")
	if !errors.Is(err, ErrUnchanged) {
		t.Fatalf("got %v, want ErrUnchanged", err)
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Fatalf("the error must name which flag: %v", err)
	}
}

func TestArchivingReportsWhatItChanged(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	got, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch")
	if err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	if !got.Changed() || got.Was || !got.After {
		t.Fatalf("the change was not reported: %s", got)
	}
}

// TestSettingAFlagThatIsAlreadySetIsSuccess. Asking for a state a conversation
// already has is not an error, and reporting it as one would make idempotent
// callers look broken.
func TestSettingAFlagThatIsAlreadySetIsSuccess(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: true, jid: lidJID, value: true}
	got, err := setter(p).SetPinned(context.Background(), phoneJID, true, "t/pin")
	if err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for a conversation already in that state: %s", got)
	}
}

// TestTheIdentityIsResolvedBeforeAnythingIsTouched.
//
// This test used to assert that the VERIFY read used the resolved identity,
// because the verify was a separate lookup. It is not any more: the write
// script reports what the conversation became, read from the object it just
// changed, so there is nothing to look up twice.
//
// The concern the old test protected is still real and now lives in one place:
// this build files chats under the identity the server assigns, so a script
// that took the caller's phone number at face value would find nothing (H34,
// H39) — the same split that broke group sending (H48).
func TestTheIdentityIsResolvedBeforeAnythingIsTouched(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	if _, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch"); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	resolveAt := strings.Index(p.lastScript, "await resolveIdentity(")
	applyAt := strings.Index(p.lastScript, "setArchive(")
	if resolveAt < 0 {
		t.Fatal("the script never CALLS the identity resolution")
	}
	if applyAt < 0 {
		t.Fatal("the archive call is not in the script")
	}
	if resolveAt > applyAt {
		t.Fatal("the conversation is changed BEFORE the identity is resolved")
	}
}

// TestThePinLimitIsExplainedRatherThanHitBlindly. The page would refuse anyway;
// the difference is whether the caller is told why, with the numbers.
func TestThePinLimitIsExplainedRatherThanHitBlindly(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "apply", why: "PIN_LIMIT", pinned: 3, pinLimit: 3}
	_, err := setter(p).SetPinned(context.Background(), phoneJID, true, "t/pin")
	if !errors.Is(err, ErrPinLimit) {
		t.Fatalf("got %v, want ErrPinLimit", err)
	}
	if !strings.Contains(err.Error(), "3 of 3") {
		t.Fatalf("the error must carry the numbers: %v", err)
	}
}

// TestTheLimitIsCheckedBeforeAsking, and only when PINNING. A guard on the way
// out would refuse to undo what it just allowed.
func TestTheLimitIsCheckedBeforeAskingAndOnlyWhenPinning(t *testing.T) {
	compressClock(t)
	pin := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	if _, err := setter(pin).SetPinned(context.Background(), phoneJID, true, "t/pin"); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if !strings.Contains(pin.lastScript, "getPinLimit") {
		t.Fatal("pinning does not consult the limit")
	}

	unpin := &pageDouble{ok: true, was: true, jid: lidJID, value: false}
	if _, err := setter(unpin).SetPinned(context.Background(), phoneJID, false, "t/unpin"); err != nil {
		t.Fatalf("SetPinned(false): %v", err)
	}
	if !strings.Contains(unpin.lastScript, "if (want)") {
		t.Fatal("the limit check is not conditional on pinning; unpinning must never " +
			"be refused for a limit it cannot exceed")
	}
}

// TestTheActionLayerIsUsedNotTheBridge, the same choice groups and presence
// made: the action carries the app's own guards.
func TestTheActionLayerIsUsedNotTheBridge(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	if _, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch"); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	if strings.Contains(p.lastScript, "WAWebChatArchiveBridge") {
		t.Fatal("the script reaches past the action to the bridge")
	}
	if !strings.Contains(p.lastScript, "WAWebSetArchiveChatAction") {
		t.Fatal("the archive action is not used")
	}
}

// TestChangingStateNeverCreatesAConversation. Archiving something that does not
// exist is not a thing to do quietly.
func TestChangingStateNeverCreatesAConversation(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	if _, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch"); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	// THE CALL, not the word. The script's own comment explains why the send
	// path falls through to findOrCreateLatestChat, and matching the mention
	// failed the test for prose — the THIRD time in one day that a guard
	// policed text where behaviour was meant (see the chat listing's getName
	// and mark-read's queryWidExists).
	if strings.Contains(p.lastScript, "findOrCreateLatestChat(") {
		t.Fatal("the script CALLS findOrCreateLatestChat; changing a conversation's " +
			"state must never bring one into existence")
	}
	if !strings.Contains(p.lastScript, "findExistingChat(") {
		t.Fatal("the script does not use the find-without-creating half of the pair")
	}
}

func TestAMissingConversationIsErrNoChat(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
}

func TestABlankJIDNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := setter(p).SetPinned(context.Background(), "  ", true, "t/pin"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) with no conversation", p.kicks)
	}
}

func TestCancelledContextChangesNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: false, jid: lidJID, value: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := setter(p).SetArchived(ctx, phoneJID, true, "t/arch"); err == nil {
		t.Fatal("a cancelled context changed a conversation")
	}
	if p.kicks != 0 {
		t.Fatalf("changed %d conversation(s) for a caller that had given up", p.kicks)
	}
}

func TestChangeRendersItsFields(t *testing.T) {
	c := Change{Was: false, After: true, Waited: time.Second}
	if !strings.Contains(c.String(), "changed=true") {
		t.Fatalf("the rendering hides the change: %s", c)
	}
	idle := Change{Was: true, After: true}
	if !strings.Contains(idle.String(), "changed=false") {
		t.Fatalf("an unchanged conversation must be distinguishable: %s", idle)
	}
}

// TestARedundantRequestNeverReachesTheApp is the fix for H55, and it is
// measured rather than defensive.
//
// One controlled run against the live account: asking for the OPPOSITE of a
// conversation's current state was accepted; asking for the state it already
// had threw ActionError("Could not perform action."). So the app refuses a
// redundant request, and asking anyway means being told off for doing nothing.
func TestARedundantRequestNeverReachesTheApp(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, was: true, jid: lidJID, value: true}
	got, err := setter(p).SetArchived(context.Background(), phoneJID, true, "t/arch")
	if err != nil {
		t.Fatalf("asking for a state the conversation already has produced an error: %v", err)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for a no-op: %s", got)
	}
	// The guard lives in the page script, so that is where it is checked.
	if !strings.Contains(p.lastScript, "was === want") {
		t.Fatal("the script has no early exit for a request that changes nothing; " +
			"the app answers ActionError to those")
	}
}

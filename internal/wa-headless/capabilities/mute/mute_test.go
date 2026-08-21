package mute

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates the SETTLING protocol, because the real page has one.
type pageDouble struct {
	ok               bool
	stage, why       string
	before, want     int64
	already          bool
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
		case p.already:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"ALREADY","already":true,"before":%d,"after":%d}`,
				p.before, p.before)
		case p.reads <= p.settleAfterReads:
			*out = fmt.Sprintf(`{"stage":"settling","ok":false,"why":"","before":%d,"after":%d}`,
				p.before, p.before)
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":false,"before":%d,"after":%d}`,
				p.before, p.want)
		}
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func muter(p *pageDouble) *Muter { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := muteBudget, muteTick
	muteBudget, muteTick = 300*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { muteBudget, muteTick = ob, ot })
}

const chatJID = "15550001111@c.us"

// TestSendDeviceIsTrue is the single most important assertion in this package.
//
// The model's body only reaches the bridge when sendDevice === true. Omit it and
// the mute is LOCAL: the model still updates, so the expiration still moves, so
// every other postcondition here still passes — and notifications keep arriving
// on the account's phone. It is a silent half-success built into the API, and
// nothing but this assertion catches it.
func TestSendDeviceIsTrue(t *testing.T) {
	compressClock(t)
	on := &pageDouble{ok: true, before: 0, want: 1787000000}
	if _, err := muter(on).For(context.Background(), chatJID, 8, "t"); err != nil {
		t.Fatalf("For: %v", err)
	}
	if !strings.Contains(on.lastScript, "model.mute({ expiration: wanted, sendDevice: true") {
		t.Fatal("mute does not pass sendDevice: true; the change would be local only")
	}
	off := &pageDouble{ok: true, before: 1787000000, want: 0}
	if _, err := muter(off).Off(context.Background(), chatJID, "t"); err != nil {
		t.Fatalf("Off: %v", err)
	}
	if !strings.Contains(off.lastScript, "model.unmute({ sendDevice: true") {
		t.Fatal("unmute does not pass sendDevice: true; the change would be local only")
	}
}

// TestTheModelIsDrivenNotTheBridge. The bridge takes an object whose key is a
// minifier artefact; the model's methods are synchronous and legible.
func TestTheModelIsDrivenNotTheBridge(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, want: 1787000000}
	if _, err := muter(p).For(context.Background(), chatJID, 8, "t"); err != nil {
		t.Fatalf("For: %v", err)
	}
	if strings.Contains(p.lastScript, "sendConversationMute(") {
		t.Fatal("the script calls the bridge directly")
	}
	if strings.Contains(p.lastScript, "$MuteImpl") {
		t.Fatal("the script passes a minifier artefact as a contract")
	}
	if !strings.Contains(p.lastScript, "MC.get(chat.id)") {
		t.Fatal("the script does not fetch the mute model")
	}
}

// TestTheAwaitIsNotTheCompletion — the lesson H61 paid for live, applied here
// before it could cost a second failure. Both layers are asserted: the Go side
// keeps reading, and the script hands the decision over instead of reading the
// field straight after the await.
func TestTheAwaitIsNotTheCompletion(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, want: 1787000000, settleAfterReads: 2}
	got, err := muter(p).For(context.Background(), chatJID, 8, "t")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.After != 1787000000 {
		t.Fatalf("the settled value was not picked up: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the first answer was accepted instead of waited on", p.reads)
	}
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
	if strings.Contains(p.lastScript, "after: Number(model.expiration") {
		t.Fatal("the script reads the field straight after the await")
	}
}

// TestAnExpirationThatNeverSettlesIsUnchangedNotATimeout.
func TestAnExpirationThatNeverSettlesIsUnchangedNotATimeout(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, want: 1787000000, settleAfterReads: 1 << 30}
	_, err := muter(p).For(context.Background(), chatJID, 8, "t")
	if !errors.Is(err, ErrExpirationUnchanged) {
		t.Fatalf("got %v, want ErrExpirationUnchanged", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("an expiration that did not move is reported as a hung page: %v", err)
	}
}

// TestADoneWithNoChangeIsAFailure covers the other postcondition branch. It
// exists because a negative control on the sibling capability proved the two
// branches need separate tests (ARMADILHAS.md).
func TestADoneWithNoChangeIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 1787000000, want: 1787000000, already: false}
	_, err := muter(p).For(context.Background(), chatJID, 8, "t")
	if !errors.Is(err, ErrExpirationUnchanged) {
		t.Fatalf("got %v, want ErrExpirationUnchanged", err)
	}
	if !strings.Contains(err.Error(), "before=1787000000 after=1787000000") {
		t.Fatalf("the error must carry the measurement: %v", err)
	}
}

// TestAlwaysGoesThroughThePagesSentinel. An arbitrary huge hour count would
// produce an ordinary far-future expiry, which looks the same and is not.
func TestAlwaysGoesThroughThePagesSentinel(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, want: 1 << 40}
	if _, err := muter(p).For(context.Background(), chatJID, Always, "t"); err != nil {
		t.Fatalf("For(Always): %v", err)
	}
	if !strings.Contains(p.lastScript, "Number.POSITIVE_INFINITY") {
		t.Fatal("Always does not reach the page's sentinel path")
	}
	if !strings.Contains(p.lastScript, "calculateMuteExpiration(") {
		t.Fatal("the script does not use the page's own converter")
	}
}

// TestTheChatsOwnRefusalIsAsked. canMute rejects this account's own chat and
// groups it is not in.
func TestTheChatsOwnRefusalIsAsked(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "CANNOT_MUTE"}
	if _, err := muter(p).For(context.Background(), chatJID, 8, "t"); !errors.Is(err, ErrCannotMute) {
		t.Fatalf("got %v, want ErrCannotMute", err)
	}
	// The needle is the BRANCH, not the predicate: a computed canMute that
	// guards nothing is the hole a negative control found in the edit
	// capability on the same day.
	if !strings.Contains(p.lastScript, "if (MU.canMute && !MU.canMute(chat)) {") {
		t.Fatal("canMute is computed but does not guard a return")
	}
}

func TestBadDurationsCostNoPageCall(t *testing.T) {
	compressClock(t)
	for _, hours := range []int{0, -2, -100} {
		p := &pageDouble{ok: true}
		if _, err := muter(p).For(context.Background(), chatJID, hours, "t"); !errors.Is(err, ErrBadDuration) {
			t.Fatalf("hours=%d: got %v, want ErrBadDuration", hours, err)
		}
		if p.kicks != 0 {
			t.Fatalf("hours=%d: the page was asked %d time(s)", hours, p.kicks)
		}
	}
	// Off is the legitimate way to reach zero, and it must NOT be refused.
	ok := &pageDouble{ok: true, before: 1787000000, want: 0}
	if _, err := muter(ok).Off(context.Background(), chatJID, "t"); err != nil {
		t.Fatalf("Off was refused: %v", err)
	}
}

func TestAMissingChatIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := muter(p).For(context.Background(), chatJID, 8, "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	q := &pageDouble{ok: false, stage: "find", why: "NO_MUTE_MODEL"}
	if _, err := muter(q).Off(context.Background(), chatJID, "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	empty := &pageDouble{ok: true}
	if _, err := muter(empty).Off(context.Background(), "  ", "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if empty.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for an empty jid", empty.kicks)
	}
}

func TestARedundantMuteIsANoOp(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, already: true, before: 1787000000, want: 1787000000}
	got, err := muter(p).For(context.Background(), chatJID, 8, "t")
	if err != nil {
		t.Fatalf("a redundant mute produced an error: %v", err)
	}
	if !got.AlreadyInState || got.Changed() {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

func TestTheRenderingCarriesNoIdentity(t *testing.T) {
	s := Result{Before: 0, After: 1787000000}.String()
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
	if !strings.Contains(s, "muted=true") {
		t.Fatalf("the rendering does not say whether the chat ends up silenced: %s", s)
	}
}

func TestCancelledContextMutesNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, want: 1787000000}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := muter(p).For(ctx, chatJID, 8, "t"); err == nil {
		t.Fatal("a cancelled context muted a chat")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

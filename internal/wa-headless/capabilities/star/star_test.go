package star

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates the SETTLING protocol, not a simplified version of it.
// The real page reports 'settling' until the flag catches up, and a double that
// answered 'done' on the first read would hide the very defect the live proof
// found — ARMADILHAS.md calls this the permissive-double trap.
type pageDouble struct {
	ok               bool
	stage, why       string
	before, want     bool
	already          bool
	settleAfterReads int // how many reads report 'settling' before the flag moves

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
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"ALREADY","already":true,"before":%t,"after":%t}`,
				p.before, p.before)
		case p.reads <= p.settleAfterReads:
			*out = fmt.Sprintf(`{"stage":"settling","ok":false,"why":"","before":%t,"after":%t}`,
				p.before, p.before)
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":false,"before":%t,"after":%t}`,
				p.before, p.want)
		}
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func starrer(p *pageDouble) *Starrer { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := starBudget, starTick
	starBudget, starTick = 300*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { starBudget, starTick = ob, ot })
}

const msgID = "3EB0000000000000000000"

// TestTheAwaitIsNotTheCompletion is the finding, and it was paid for live: the
// first version read msg.star immediately after the await and reported
// before=false after=false against a star that did land 696ms later.
//
// The double reports 'settling' twice before the flag moves. A capability that
// trusts the await fails this test.
func TestTheAwaitIsNotTheCompletion(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: false, want: true, settleAfterReads: 2}
	got, err := starrer(p).Star(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Star: %v", err)
	}
	if !got.After || !got.Changed() {
		t.Fatalf("the settled value was not picked up: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the capability accepted the first answer instead of waiting", p.reads)
	}

	// AND THE SCRIPT HAS TO PARK THE MODEL. The assertion above exercises the
	// Go side only, and a negative control proved that: rewriting the page
	// script to trust the await left this test GREEN, because the double keeps
	// answering 'settling' whatever the script says. Locking the Go polling
	// without locking the script would leave the measured defect free to come
	// back on the side it actually lived on.
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
	if strings.Contains(p.lastScript, "after: !!msg.star") {
		t.Fatal("the script reads the flag straight after the await, which is the measured defect")
	}
}

// TestADoneWithNoChangeIsAFailure covers the OTHER postcondition path: the page
// says it finished, was not a no-op, and the flag reads the same. A negative
// control found this uncovered — the deadline path was tested and this one was
// not, and they are different branches with different mutations.
func TestADoneWithNoChangeIsAFailure(t *testing.T) {
	compressClock(t)
	// want equals before, and already is false: the page claims it did
	// something and nothing moved.
	p := &pageDouble{ok: true, before: false, want: false, already: false}
	_, err := starrer(p).Star(context.Background(), msgID, "t")
	if !errors.Is(err, ErrFlagUnchanged) {
		t.Fatalf("got %v, want ErrFlagUnchanged", err)
	}
	if !strings.Contains(err.Error(), "before=false after=false") {
		t.Fatalf("the error must carry the measurement: %v", err)
	}
}

// TestAFlagThatNeverSettlesIsUnchangedNotATimeout. The distinction matters to a
// caller: "the page hung" and "the star did not take" have different repairs.
func TestAFlagThatNeverSettlesIsUnchangedNotATimeout(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: false, want: true, settleAfterReads: 1 << 30}
	_, err := starrer(p).Star(context.Background(), msgID, "t")
	if !errors.Is(err, ErrFlagUnchanged) {
		t.Fatalf("got %v, want ErrFlagUnchanged", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a flag that did not move is reported as a hung page: %v", err)
	}
}

// TestTheClockStaysOnTheGoSide. Invariant 6. The probe that discovered the
// delay used setTimeout; the capability must not, or the wait becomes a
// duration nothing in Go can see or compress.
func TestTheClockStaysOnTheGoSide(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: false, want: true}
	if _, err := starrer(p).Star(context.Background(), msgID, "t"); err != nil {
		t.Fatalf("Star: %v", err)
	}
	// The needles are CALLS, with the open paren. Matching the bare name found
	// the word inside the script's own comment explaining why it does not use a
	// timer — the guard-matches-prose trap in ARMADILHAS.md, hit for the third
	// time in one day, twice by the person who wrote the rule down.
	for _, banned := range []string{"setTimeout(", "setInterval(", "Date.now()"} {
		if strings.Contains(p.lastScript, banned) {
			t.Fatalf("the script uses the page's clock (%s)", banned)
		}
	}
}

// TestTheParkedModelIsNeverSerialised. The state holds a live page model; the
// result script reads one boolean off it and builds its own answer.
func TestTheParkedModelIsNeverSerialised(t *testing.T) {
	if !strings.Contains(resultScript, "s.msg && s.msg.star") {
		t.Fatal("the result script does not read the flag off the parked model")
	}
	if strings.Contains(resultScript, "JSON.stringify(s)") {
		t.Fatal("the parked model is handed to JSON.stringify")
	}
}

// TestTheMeasuredCallShapeIsUsed — chat model and an ARRAY of message models,
// through Cmd, settled by experiment rather than by the bridge's name.
func TestTheMeasuredCallShapeIsUsed(t *testing.T) {
	compressClock(t)
	on := &pageDouble{ok: true, before: false, want: true}
	if _, err := starrer(on).Star(context.Background(), msgID, "t"); err != nil {
		t.Fatalf("Star: %v", err)
	}
	if !strings.Contains(on.lastScript, "Cmd.sendStarMsgs(chat, [msg], true)") {
		t.Fatal("star does not use the measured shape")
	}
	off := &pageDouble{ok: true, before: true, want: false}
	if _, err := starrer(off).Unstar(context.Background(), msgID, "t"); err != nil {
		t.Fatalf("Unstar: %v", err)
	}
	if !strings.Contains(off.lastScript, "Cmd.sendUnstarMsgs(chat, [msg], true)") {
		t.Fatal("unstar does not use the measured shape")
	}
	// And NOT the collection that the parity table wrongly nominated.
	if strings.Contains(on.lastScript, "StarredMsgCollection") {
		t.Fatal("the script consults the collection that was measured throwing")
	}
}

// TestARedundantStarIsANoOp.
func TestARedundantStarIsANoOp(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, already: true, before: true, want: true}
	got, err := starrer(p).Star(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("a redundant star produced an error: %v", err)
	}
	if !got.AlreadyInState || got.Changed() {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

func TestAMissingMessageOrChatIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := starrer(p).Star(context.Background(), msgID, "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	q := &pageDouble{ok: false, stage: "find", why: "NO_CHAT"}
	if _, err := starrer(q).Star(context.Background(), msgID, "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
}

func TestNoIdentityIsReported(t *testing.T) {
	s := Result{Before: false, After: true}.String()
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := starrer(p).Star(context.Background(), "   ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for a call that could not work", p.kicks)
	}
}

func TestCancelledContextStarsNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: false, want: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := starrer(p).Star(ctx, msgID, "t"); err == nil {
		t.Fatal("a cancelled context starred a message")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

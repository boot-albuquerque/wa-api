package pin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// THE CAPABILITY DOES NOT WORK AGAINST THE LIVE BUILD (H81). These tests lock
// the measurements — the two enums, the duration, the message model, and that a
// real change is never claimed verified — so the next attempt starts from what
// was learned.

type pinDouble struct {
	ok         bool
	stage, why string
	seconds    int

	kicks      int
	lastScript string
}

func (p *pinDouble) eval(ctx context.Context, expr string, out *string) error {
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","seconds":%d,"source":"WAWebPinMsgConstants.PIN_STATE"}`, p.seconds)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func pinner(p *pinDouble) *Pinner { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := pinBudget, pinTick
	pinBudget, pinTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { pinBudget, pinTick = ob, ot })
}

const msgID = "3EB0000000000000000000"

// TestTheStateNumbersComeFromThePage. Two enums carry them — a UI one and a wire
// one — and they happen to agree today. Reading beats writing them down anyway:
// the day they disagree, a hard-coded 1 pins when asked to unpin, and nobody
// would read that failure as a renumbered enum.
func TestTheStateNumbersComeFromThePage(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: true, seconds: 604800}
	if _, err := pinner(p).Message(context.Background(), msgID, "t"); err != nil {
		t.Fatalf("Message: %v", err)
	}
	if !strings.Contains(p.lastScript, "const states = K.PIN_STATE;") {
		t.Fatal("the script does not read the state vocabulary from the page")
	}
	if strings.Contains(p.lastScript, "state = on ? 1 : 2") {
		t.Fatal("the numbers are hard-coded")
	}
	if !strings.Contains(p.lastScript, "states.PIN : states.UNPIN") {
		t.Fatal("the state is not chosen from the page's own names")
	}
}

// TestAMissingVocabularyIsItsOwnAnswer. Inventing 1 and 2 would work until it
// did not, so a build that does not expose the enum is refused by name.
func TestAMissingVocabularyIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: false, stage: "vocabulary", why: "NO_VOCABULARY"}
	if _, err := pinner(p).Message(context.Background(), msgID, "t"); !errors.Is(err, ErrNoVocabulary) {
		t.Fatalf("got %v, want ErrNoVocabulary", err)
	}
	if !strings.Contains(p.lastScript, "typeof states.PIN !== 'number'") {
		t.Fatal("the vocabulary check does not verify the values are numbers")
	}
}

// TestTheDurationComesFromThePageToo. A pin EXPIRES, and choosing the number
// here would be choosing how long other people see it.
func TestTheDurationComesFromThePageToo(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: true, seconds: 604800}
	got, err := pinner(p).Message(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.Seconds != 604800 {
		t.Fatalf("the duration was not carried: %s", got)
	}
	if !strings.Contains(p.lastScript, "K.getPinExpiryDuration(opt)") {
		t.Fatal("the script does not use the page's own converter")
	}
	// Unpinning carries no duration: it is not a shorter pin.
	off := &pinDouble{ok: true, seconds: 0}
	back, err := pinner(off).Unpin(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	if back.Seconds != 0 {
		t.Fatalf("unpinning carried a duration: %s", back)
	}
}

// TestARealPinIsNeverClaimedVerified.
func TestARealPinIsNeverClaimedVerified(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: true, seconds: 604800}
	got, err := pinner(p).Message(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.Verified {
		t.Fatalf("a real pin claims to be verified: %s", got)
	}
	if !strings.Contains(got.String(), "verified=false") {
		t.Fatalf("the rendering hides that nothing was confirmed: %s", got)
	}
	if strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the script waits on a collection this build does not refresh in-session")
	}
}

// TestPinnedInTriesEveryPlausibleOwnerField. Which field carries the parent was
// never established, and picking one would work until it did not.
func TestPinnedInTriesEveryPlausibleOwnerField(t *testing.T) {
	s := pinnedInScript("1@c.us")
	for _, f := range []string{"parentMsgKey", "chatId", "parentMsgId"} {
		if !strings.Contains(s, f) {
			t.Fatalf("the pinned list does not consider %s", f)
		}
	}
}

func TestNothingPinnedIsAnEmptySliceNotNil(t *testing.T) {
	l := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
		*out = `{"ok":true,"why":"","ids":null}`
		return nil
	})
	ids, err := l.PinnedIn(context.Background(), "1@c.us", "t")
	if err != nil {
		t.Fatalf("PinnedIn: %v", err)
	}
	if ids == nil {
		t.Fatal("a chat with nothing pinned returned nil")
	}
}

func TestPinRefusalsAndFailures(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: true}
	if _, err := pinner(p).Message(context.Background(), "  ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
	nl := &pinDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := pinner(nl).Message(context.Background(), msgID, "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	boom := &pinDouble{ok: false, stage: "apply", why: "boom"}
	if _, err := pinner(boom).Message(context.Background(), msgID, "t"); !errors.Is(err, ErrPin) {
		t.Fatalf("got %v, want ErrPin", err)
	}
	nc := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
		*out = `{"ok":false,"why":"NO_CHAT","ids":[]}`
		return nil
	})
	if _, err := nc.PinnedIn(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if _, err := nc.PinnedIn(context.Background(), "  ", "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	bad := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
		*out = "not json"
		return nil
	})
	if _, err := bad.PinnedIn(context.Background(), "1@c.us", "t"); err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
}

func TestAHungPageIsATimeoutForPins(t *testing.T) {
	compressClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	if _, err := New(engine.NewRunner(), stuck).Message(context.Background(), msgID, "t"); !errors.Is(err, ErrPin) {
		t.Fatalf("got %v, want ErrPin", err)
	}
}

func TestCancelledContextPinsNothing(t *testing.T) {
	compressClock(t)
	p := &pinDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pinner(p).Message(ctx, msgID, "t"); err == nil {
		t.Fatal("a cancelled context pinned a message")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

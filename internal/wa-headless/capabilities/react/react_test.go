package react

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
	had      bool
	hasAfter bool

	kicks      int
	verifies   int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case strings.Contains(expr, `"found"`) || strings.Contains(expr, "found: true"):
		p.verifies++
		*out = fmt.Sprintf(`{"found":true,"has":%t,"sticky":%t,"sum":-1}`, p.hasAfter, p.hasAfter)
		return nil
	case strings.Contains(expr, "const s = window["):
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "react"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","had":%t}`, p.had)
		return nil
	default:
		p.kicks++
		p.lastScript = expr
		*out = `{"started":true}`
		return nil
	}
}

func reactor(p *pageDouble) *Reactor { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := reactBudget, reactTick
	reactBudget, reactTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { reactBudget, reactTick = ob, ot })
}

// TestAddingIsVERIFIEDAndRemovingIsNOT is the asymmetry, and it is measured
// rather than chosen.
//
// Adding flips hasReaction false -> true within a second, so waiting for it is
// a real postcondition. Removing does not flip it back in the session that
// performed it — three FRESH sessions confirmed the removal actually worked —
// so polling would wait forever for something that already happened. A Result
// that hid the difference would let "removed" read as strongly as "added".
func TestAddingIsVerifiedAndRemovingIsNot(t *testing.T) {
	compressClock(t)

	add := &pageDouble{ok: true, had: false, hasAfter: true}
	got, err := reactor(add).Add(context.Background(), "MSG1", "\U0001F44D", "t/add")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !got.Verified || !got.Has {
		t.Fatalf("adding must be checked against the page: %s", got)
	}
	if add.verifies == 0 {
		t.Fatal("Add never re-read the page; its postcondition is not being waited for")
	}

	rem := &pageDouble{ok: true, had: true, hasAfter: true} // the sticky flag
	cleared, err := reactor(rem).Remove(context.Background(), "MSG1", "t/remove")
	if err != nil {
		t.Fatalf("Remove: %v — polling a flag this build never flips would make a "+
			"capability that always fails at something that always works", err)
	}
	if cleared.Verified {
		t.Fatal("Remove claimed to be verified")
	}
	if rem.verifies != 0 {
		t.Fatalf("Remove polled the page %d time(s) for a flag that does not flip", rem.verifies)
	}
}

// TestAddingWithoutAnEmojiIsRefused: an empty emoji is how the protocol REMOVES,
// so accepting it in Add would silently do the opposite of what was asked.
func TestAddingWithoutAnEmojiIsRefused(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	_, err := reactor(p).Add(context.Background(), "MSG1", "   ", "t/add")
	if !errors.Is(err, ErrReact) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "Remove") {
		t.Fatalf("the error should point at the call that does clear one: %v", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) to add an empty reaction", p.kicks)
	}
}

// TestAReactionThatNeverAppearsIsAFailure. The page accepting the call is not
// the message having changed.
func TestAReactionThatNeverAppearsIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: false, hasAfter: false}
	_, err := reactor(p).Add(context.Background(), "MSG1", "\U0001F44D", "t/add")
	if !errors.Is(err, ErrUnreacted) {
		t.Fatalf("got %v, want ErrUnreacted", err)
	}
}

// TestTheMessageModelIsPassed, not its id. Three capabilities in one day paid a
// correction each for assuming the other way (H40, H46, H49).
func TestTheMessageModelIsPassed(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: false, hasAfter: true}
	if _, err := reactor(p).Add(context.Background(), "MSG1", "\U0001F44D", "t/add"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !strings.Contains(p.lastScript, "sendReactionToMsg(msg,") {
		t.Fatal("sendReactionToMsg is not called with the message MODEL")
	}
}

// TestThePagesOwnRuleIsRespected. WAWebReactionsUtils.canReactToMessage exists
// because some messages cannot carry a reaction; asking anyway is asking the app
// to break its own guard.
func TestThePagesOwnRuleIsRespected(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: false, hasAfter: true}
	if _, err := reactor(p).Add(context.Background(), "MSG1", "\U0001F44D", "t/add"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !strings.Contains(p.lastScript, "canReactToMessage") {
		t.Fatal("the script does not consult the page's own rule about what can " +
			"carry a reaction")
	}
}

func TestAnUnreactableMessageIsItsOwnError(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_REACTABLE"}
	_, err := reactor(p).Add(context.Background(), "MSG1", "\U0001F44D", "t/add")
	if !errors.Is(err, ErrNotReactable) {
		t.Fatalf("got %v, want ErrNotReactable: a message that cannot carry a "+
			"reaction is a different problem from one that is missing", err)
	}
}

func TestAMissingMessageIsErrNoMessage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := reactor(p).Add(context.Background(), "MSG1", "\U0001F44D", "t/add"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
}

func TestABlankIDNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := reactor(p).Add(context.Background(), "  ", "\U0001F44D", "t/add"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) with no message", p.kicks)
	}
}

func TestCancelledContextReactsToNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: false, hasAfter: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reactor(p).Add(ctx, "MSG1", "\U0001F44D", "t/add"); err == nil {
		t.Fatal("a cancelled context reacted")
	}
	if p.kicks != 0 {
		t.Fatalf("reacted %d time(s) for a caller that had given up", p.kicks)
	}
}

// TestResultRendersTheAsymmetry. String() is a redaction and reporting surface,
// and an untested one is how a leak or a missing field ships. Here the field
// that matters is Verified: a rendering that hid it would let "removed" read
// as strongly as "added".
func TestResultRendersTheAsymmetry(t *testing.T) {
	added := Result{Had: false, Has: true, Verified: true, Waited: time.Second}
	if !strings.Contains(added.String(), "verified=true") {
		t.Fatalf("an added reaction must show that it was checked: %s", added)
	}
	removed := Result{Had: true, Has: false, Verified: false}
	if !strings.Contains(removed.String(), "verified=false") {
		t.Fatalf("a removed reaction must show that it was NOT checked: %s", removed)
	}
	if strings.Contains(removed.String(), "verified=true") {
		t.Fatalf("the rendering contradicts itself: %s", removed)
	}
}

// TestAStalledPageIsItsOwnFailure: the page accepting the kick and then never
// settling is not the same as refusing, and a caller that could not tell them
// apart would retry the wrong one.
func TestAStalledPageIsItsOwnFailure(t *testing.T) {
	compressClock(t)
	p := &stallingDouble{}
	_, err := New(engine.NewRunner(), p.eval).Add(context.Background(), "MSG1", "\U0001F44D", "t/add")
	if !errors.Is(err, ErrReact) {
		t.Fatalf("got %v, want ErrReact", err)
	}
	if !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("the message must distinguish a stall from a refusal: %v", err)
	}
}

// stallingDouble accepts the kick and then answers "pending" forever.
type stallingDouble struct{}

func (p *stallingDouble) eval(ctx context.Context, expr string, out *string) error {
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

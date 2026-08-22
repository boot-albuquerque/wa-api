package react

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
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

	// mineAfter is what the reactions record says about THIS account after the
	// call, and mineFails makes that read refuse.
	//
	// THEY ARE SEPARATE FROM hasAfter ON PURPOSE. hasAfter drives the sticky
	// flag, which is what Add waits on; mineAfter drives the reactions record,
	// which is what Remove waits on. A double that answered both from one field
	// would let a Remove that polled the sticky flag look verified.
	mineAfter bool
	mineFails bool
	mines     int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' KICK NEM LEITURA (H177): caindo no ramo padrao ela vira
	// lastScript e soma um kick, e todo teste que afirma sobre o script passa a
	// inspecionar o de limpeza.
	if strings.Contains(expr, "delete window.") {
		*out = "ok"
		return nil
	}
	switch {
	case strings.Contains(expr, "window."+stateKeyMine+" ||"):
		if p.mineFails {
			*out = `{"ok":false,"why":"BOOM","mine":false}`
			return nil
		}
		*out = fmt.Sprintf(`{"ok":true,"notFound":false,"mine":%t}`, p.mineAfter)
		return nil
	case strings.Contains(expr, stateKeyMine):
		p.mines++
		p.lastScript = expr
		*out = "kicked"
		return nil
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
// BOTH HALVES ARE VERIFIED NOW, and the asymmetry this test used to assert is
// gone because its CAUSE went, not because the standard dropped.
//
// The old contract was honest on the evidence it had: hasReaction is sticky, so
// polling it after a removal waits forever for something that already happened.
// H154 found the source that was missing — the reactions record, fetched by the
// id OBJECT, whose hasReactionByMe DOES go false — so Remove now has a real
// postcondition and uses it.
//
// THE TWO HALVES WAIT ON DIFFERENT SIGNALS, which is why the double carries two
// fields. Add waits on the sticky flag; Remove waits on the record. A Remove
// that polled the sticky flag would report "still there" forever, and this test
// would catch that because mineAfter and hasAfter disagree here.
func TestBothAddingAndRemovingAreVerified(t *testing.T) {
	compressClock(t)

	add := &pageDouble{ok: true, had: false, hasAfter: true, mineAfter: true}
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

	// hasAfter stays TRUE — the sticky flag never flips — while the record says
	// the reaction is ours no more. That disagreement is the whole point.
	rem := &pageDouble{ok: true, had: true, hasAfter: true, mineAfter: false}
	cleared, err := reactor(rem).Remove(context.Background(), "MSG1", "t/remove")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !cleared.Verified {
		t.Fatal("Remove did not verify, though the reactions record is available")
	}
	if cleared.Has {
		t.Fatal("Remove reported the reaction still present, reading the sticky " +
			"flag instead of the record")
	}
	if rem.mines == 0 {
		t.Fatal("Remove never asked the reactions record, so whatever it verified " +
			"was not the removal")
	}
	if rem.verifies != 0 {
		t.Fatalf("Remove polled the sticky flag %d time(s); it does not flip", rem.verifies)
	}
}

// A VERIFICATION THAT FAILS DOES NOT INVENT A VERDICT. The page accepted the
// removal; what is unknown is the after, and Verified false is exactly how this
// type says that. Reporting Verified true here would be the silent success
// invariant 14 forbids; returning an error would fail a call that worked.
func TestARefusedVerificationLeavesRemoveUnverified(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: true, hasAfter: true, mineFails: true}
	got, err := reactor(p).Remove(context.Background(), "MSG1", "t/remove")
	if err != nil {
		t.Fatalf("a removal the page accepted must not fail because the check did: %v", err)
	}
	if got.Verified {
		t.Fatalf("a refused verification was reported as verified: %s", got)
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
	// A LIBERACAO NAO E' KICK NEM LEITURA (H177): caindo no ramo padrao ela vira
	// lastScript e soma um kick, e todo teste que afirma sobre o script passa a
	// inspecionar o de limpeza.
	if strings.Contains(expr, "delete window.") {
		*out = "ok"
		return nil
	}
	if strings.Contains(expr, "const s = window[") {
		*out = `{"stage":"pending","ok":false,"why":""}`
		return nil
	}
	*out = `{"started":true}`
	return nil
}

// byMe COMES FROM THE PAGE, and this is asserted on the SCRIPT because the
// double supplies the answer either way — a result assertion here would measure
// the double, which is the trap this repository catalogued and which a first
// negative control for this rule walked straight into.
//
// Deriving "is it mine" by comparing this account's jid against the senders is
// the comparison that has gone wrong here twice (H136, H148), and on a LID-first
// build it goes wrong silently: the jids simply never match.
func TestTheMineScriptUsesThePagesOwnFlag(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: true, hasAfter: true, mineAfter: false}
	if _, err := reactor(p).Remove(context.Background(), "MSG1", "t"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	code := p.lastScript
	if !strings.Contains(code, "g.byMe") {
		t.Fatal("the script does not read the page's own byMe flag")
	}
	if strings.Contains(code, "senderUserJid ===") || strings.Contains(code, "g.senders||[]).length") {
		t.Fatal("the script decides ownership from the senders list instead of the " +
			"page's flag; on a LID-first build that comparison fails silently")
	}
}

// THE SHARED EXPRESSION IS EMBEDDED, not copied. capabilities/message reports
// the same fact to callers; two copies would drift into a removal that verifies
// here and reads back as present there.
func TestTheMineScriptEmbedsTheSharedExpression(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, had: true, hasAfter: true, mineAfter: false}
	if _, err := reactor(p).Remove(context.Background(), "MSG1", "t"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !strings.Contains(p.lastScript, spa.ReactionsForMessageExpr) {
		t.Fatal("the script no longer embeds spa.ReactionsForMessageExpr, so this " +
			"package and capabilities/message now hold two copies of the same query")
	}
}

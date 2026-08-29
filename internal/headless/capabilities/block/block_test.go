package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

type pageDouble struct {
	ok            bool
	stage, why    string
	before, after int
	already       bool

	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' KICK NEM LEITURA (H177): ela roda depois de a resposta
	// ser tomada, e caindo no ramo padrao ela vira lastScript e soma um kick.
	if strings.Contains(expr, "delete window.") {
		*out = "ok"
		return nil
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":%q,"before":%d,"after":%d,"already":%t}`,
			p.why, p.before, p.after, p.already)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func blocker(p *pageDouble) *Blocker { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := blockBudget, blockTick
	blockBudget, blockTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { blockBudget, blockTick = ob, ot })
}

const peer = "15550001111@c.us"

// TestTheTwoCallsHaveDifferentShapes is the finding this package was built
// around, and it is the same finding as H58 in a different module:
// blockContact takes ONE OBJECT and unblockContact takes TWO POSITIONAL
// arguments, exported side by side from the same module. Both were read from
// the functions' own toString() against the live build.
//
// The test exists because the cheap mistake is to "tidy" one into the other.
func TestTheTwoCallsHaveDifferentShapes(t *testing.T) {
	compressClock(t)

	b := &pageDouble{ok: true, before: 0, after: 1}
	if _, err := blocker(b).Block(context.Background(), peer, "t/b"); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if !strings.Contains(b.lastScript, "A.blockContact({ contact: contact, blockEntryPoint:") {
		t.Fatal("block is not called with the measured single-object shape")
	}

	u := &pageDouble{ok: true, before: 1, after: 0}
	if _, err := blocker(u).Unblock(context.Background(), peer, "t/u"); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if strings.Contains(u.lastScript, "A.unblockContact({") {
		t.Fatal("unblock is called with an object; its source takes two positional arguments")
	}
	if !strings.Contains(u.lastScript, "A.unblockContact(contact,") {
		t.Fatal("unblock does not use the positional shape read from its source")
	}
}

// TestTheContactModelIsPassedNotTheWid. Five "reading '<field>' of undefined"
// failures in this module came from passing an id where a model was wanted.
func TestTheContactModelIsPassedNotTheWid(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, after: 1}
	if _, err := blocker(p).Block(context.Background(), peer, "t"); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if !strings.Contains(p.lastScript, "CC.get(resolved.wid)") {
		t.Fatal("the script does not look the contact MODEL up")
	}
	if strings.Contains(p.lastScript, "contact: resolved.wid") {
		t.Fatal("the wid is passed where the model is wanted")
	}
}

// TestTheAppsOwnPreconditionIsCheckedHere. The bundle carries a throw for
// "trying to block a pn contact without a chat". Letting it throw would give a
// caller a stray string from somebody else's telemetry path; ErrNoChatToBlockFrom
// is something they can act on.
func TestTheAppsOwnPreconditionIsCheckedHere(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "apply", why: "NO_CHAT"}
	_, err := blocker(p).Block(context.Background(), peer, "t")
	if !errors.Is(err, ErrNoChatToBlockFrom) {
		t.Fatalf("got %v, want ErrNoChatToBlockFrom", err)
	}
	// THE NEEDLE IS THE BRANCH. Asserting only that the lookup APPEARS lets a
	// mutation that computes the chat and ignores it pass — the same hole that
	// a negative control found in the edit capability's gate test on the same
	// day. The needle names the guarded return instead.
	if !strings.Contains(p.lastScript, "if (!chat && contact.id") {
		t.Fatal("the chat is looked up but does not guard a return")
	}
	if !strings.Contains(p.lastScript, "ChatCollection.get(contact.id)") {
		t.Fatal("the script does not check for the chat the app requires")
	}
}

// TestARedundantBlockIsANoOpNotAFailure — same shape as an already-archived
// chat (H55): the caller's intent is already satisfied.
func TestARedundantBlockIsANoOpNotAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, why: "ALREADY", already: true, before: 3, after: 3}
	got, err := blocker(p).Block(context.Background(), peer, "t")
	if err != nil {
		t.Fatalf("a redundant block produced an error: %v", err)
	}
	if !got.AlreadyInState || got.Changed() {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

// TestABlocklistThatDoesNotMoveIsAFailure is the postcondition. A caller who
// believes somebody is blocked stops watching for them, so a call that returns
// without moving the list must not read as success.
func TestABlocklistThatDoesNotMoveIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 2, after: 2}
	_, err := blocker(p).Block(context.Background(), peer, "t")
	if !errors.Is(err, ErrBlocklistUnchanged) {
		t.Fatalf("got %v, want ErrBlocklistUnchanged", err)
	}
	if !strings.Contains(err.Error(), "before=2 after=2") {
		t.Fatalf("the error must carry the measurement: %v", err)
	}
}

// TestOnlyCountsAreReported. A blocklist is a list of people; this package
// renders numbers.
func TestOnlyCountsAreReported(t *testing.T) {
	s := Result{Before: 0, After: 1}.String()
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
	if !strings.Contains(s, "before=0") || !strings.Contains(s, "after=1") {
		t.Fatalf("the counts are missing: %s", s)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	for _, tc := range []struct {
		name, jid string
		want      error
	}{
		{"group", "120363000000000000@g.us", ErrGroup},
		{"empty", "   ", ErrNoContact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pageDouble{ok: true}
			if _, err := blocker(p).Block(context.Background(), tc.jid, "t"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if p.kicks != 0 {
				t.Fatalf("the page was asked %d time(s) for a call that could not work", p.kicks)
			}
		})
	}
}

func TestAnUnresolvableIdentityIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "resolve", why: "NOT_ON_WHATSAPP"}
	if _, err := blocker(p).Block(context.Background(), peer, "t"); !errors.Is(err, ErrNotOnWhatsApp) {
		t.Fatalf("got %v, want ErrNotOnWhatsApp", err)
	}
	q := &pageDouble{ok: false, stage: "contact", why: "NO_CONTACT"}
	if _, err := blocker(q).Unblock(context.Background(), peer, "t"); !errors.Is(err, ErrNoContact) {
		t.Fatalf("got %v, want ErrNoContact", err)
	}
}

func TestCancelledContextBlocksNobody(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, before: 0, after: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := blocker(p).Block(ctx, peer, "t"); err == nil {
		t.Fatal("a cancelled context blocked somebody")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAPageThatNeverSettlesIsATimeout(t *testing.T) {
	compressClock(t)
	stuck := func(ctx context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	b := New(engine.NewRunner(), stuck)
	_, err := b.Block(context.Background(), peer, "t")
	if !errors.Is(err, ErrBlock) || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("got %v, want a settle timeout", err)
	}
}

// listDouble answers the LIST script only. It is a separate double from
// pageDouble on purpose: the list is a plain read with no kick-and-park cycle,
// and a double that answered both would let a List that accidentally ran the
// block script look like it worked.
type listDouble struct {
	answer string
	reads  int
	last   string
}

func (d *listDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.reads++
	d.last = expr
	*out = d.answer
	return nil
}

func TestTheListNamesWhoIsBlocked(t *testing.T) {
	d := &listDouble{answer: `{"ok":true,"why":"","blocked":["1@lid","2@c.us"]}`}
	got, err := New(engine.NewRunner(), d.eval).List(context.Background(), "t")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0] != "1@lid" || got[1] != "2@c.us" {
		t.Fatalf("the list did not come back whole: %#v", got)
	}
}

// AN EMPTY BLOCKLIST IS NOT AN ERROR. It is the normal state of most accounts —
// this one included, measured at 0 — and reporting it as a failure would make
// the healthy case indistinguishable from a broken read.
func TestAnEmptyBlocklistIsNotAnError(t *testing.T) {
	d := &listDouble{answer: `{"ok":true,"why":"","blocked":[]}`}
	got, err := New(engine.NewRunner(), d.eval).List(context.Background(), "t")
	if err != nil {
		t.Fatalf("an empty blocklist must not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want nothing, got %#v", got)
	}
}

// A REFUSAL IS NOT AN EMPTY LIST, and this is the distinction that makes the
// method usable: "nobody is blocked" and "the read failed" are the same shape
// once an error is swallowed, and a caller acting on the first would unblock
// nobody while believing it had checked.
func TestARefusedReadIsNotAnEmptyBlocklist(t *testing.T) {
	d := &listDouble{answer: `{"ok":false,"why":"BOOM","blocked":[]}`}
	got, err := New(engine.NewRunner(), d.eval).List(context.Background(), "t")
	if err == nil {
		t.Fatalf("a refused read came back as a clean empty list: %#v", got)
	}
	if !errors.Is(err, ErrBlock) {
		t.Fatalf("want ErrBlock, got %v", err)
	}
}

// THE ROSTER IS NOT THE BLOCKLIST. Of 945 contacts on this account, ZERO carry
// an isBlocked field (H146), so a version that filtered the roster would return
// nothing forever and look healthy doing it.
func TestTheListReadsTheBlocklistAndNotTheRoster(t *testing.T) {
	d := &listDouble{answer: `{"ok":true,"why":"","blocked":[]}`}
	if _, err := New(engine.NewRunner(), d.eval).List(context.Background(), "t"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(d.last, "BlocklistCollection") {
		t.Fatal("the script does not read BlocklistCollection")
	}
	for _, wrong := range []string{"ContactCollection", "isBlocked"} {
		if strings.Contains(d.last, wrong) {
			t.Errorf("the script reads %q; the roster carries no block state on "+
				"this build, so filtering it returns nothing forever", wrong)
		}
	}
}

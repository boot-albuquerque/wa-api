package revoke

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
	ok      bool
	stage   string
	why     string
	as      string
	revoked bool

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
				stage = "revoke"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","as":%q,"revoked":%t}`, p.as, p.revoked)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func revoker(p *pageDouble) *Revoker { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := revokeBudget, revokeTick
	revokeBudget, revokeTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { revokeBudget, revokeTick = ob, ot })
}

// TestAMessageStillThereIsAFailure is the postcondition, and it matters more
// here than anywhere else in this module: deleting for everyone cannot be
// undone, so reporting it without checking would leave a caller believing a
// message is gone from other people's phones when it is not.
func TestAMessageStillThereIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, as: "sender", revoked: false}
	_, err := revoker(p).ForEveryone(context.Background(), "MSG1", false, "t/rev")
	if !errors.Is(err, ErrStillThere) {
		t.Fatalf("got %v, want ErrStillThere", err)
	}
}

func TestARevokedMessageReportsWhichEntitlementWasUsed(t *testing.T) {
	compressClock(t)
	got, err := revoker(&pageDouble{ok: true, as: "sender", revoked: true}).
		ForEveryone(context.Background(), "MSG1", false, "t/rev")
	if err != nil {
		t.Fatalf("ForEveryone: %v", err)
	}
	if got.As != "sender" {
		t.Fatalf("As=%q, want sender", got.As)
	}
	if !strings.Contains(got.String(), "as=sender") {
		t.Fatalf("the rendering hides which act happened: %s", got)
	}
}

// TestTheEntitlementIsAskedOfThePage. WhatsApp has a time window for deleting
// your own message and an admin path for somebody else's; picking without
// asking is asking for something this account may not have.
func TestTheEntitlementIsAskedOfThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, as: "sender", revoked: true}
	if _, err := revoker(p).ForEveryone(context.Background(), "MSG1", false, "t/rev"); err != nil {
		t.Fatalf("ForEveryone: %v", err)
	}
	if !strings.Contains(p.lastScript, "canSenderRevokeMsg(") {
		t.Fatal("the script never asks whether this account may revoke as the sender")
	}
	if !strings.Contains(p.lastScript, "Cmd.Revoke.") {
		t.Fatal("the script does not use the page's own Revoke enum")
	}
}

// TestAMessageOutsideTheWindowIsItsOwnError. A caller can act on this — delete
// it locally instead — and a generic refusal would not tell them that.
func TestAMessageOutsideTheWindowIsItsOwnError(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_REVOCABLE"}
	if _, err := revoker(p).ForEveryone(context.Background(), "MSG1", false, "t/rev"); !errors.Is(err, ErrNotRevocable) {
		t.Fatalf("got %v, want ErrNotRevocable", err)
	}
}

// TestTheRecordIsPassedNotTheMessage. sendRevoke's first argument is
// {type, data}; passing the message itself is the shape that cost H40, H46 and
// H49 a correction each, in the other direction.
func TestTheRecordIsPassedNotTheMessage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, as: "sender", revoked: true}
	if _, err := revoker(p).ForEveryone(context.Background(), "MSG1", false, "t/rev"); err != nil {
		t.Fatalf("ForEveryone: %v", err)
	}
	if !strings.Contains(p.lastScript, "sendRevoke({ type: 'message', data: msg }") {
		t.Fatal("sendRevoke is not called with the record the app's own code passes")
	}
}

// TestClearMediaIsTheCallersChoice. Destroying the local copy as well is a
// different intention, and defaulting to destroying more than was asked is not
// a default this package makes on somebody's behalf.
func TestClearMediaIsTheCallersChoice(t *testing.T) {
	compressClock(t)
	keep := &pageDouble{ok: true, as: "sender", revoked: true}
	if _, err := revoker(keep).ForEveryone(context.Background(), "MSG1", false, "t/rev"); err != nil {
		t.Fatalf("ForEveryone: %v", err)
	}
	if !strings.Contains(keep.lastScript, "kind, false)") {
		t.Fatalf("clearMedia=false did not reach the page")
	}
	clear := &pageDouble{ok: true, as: "sender", revoked: true}
	if _, err := revoker(clear).ForEveryone(context.Background(), "MSG1", true, "t/rev"); err != nil {
		t.Fatalf("ForEveryone: %v", err)
	}
	if !strings.Contains(clear.lastScript, "kind, true)") {
		t.Fatalf("clearMedia=true did not reach the page")
	}
}

func TestAMissingMessageIsErrNoMessage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := revoker(p).ForEveryone(context.Background(), "MSG1", false, "t/rev"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
}

func TestABlankIDNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := revoker(p).ForEveryone(context.Background(), "  ", false, "t/rev"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to delete %d time(s) with no message", p.kicks)
	}
}

// TestCancelledContextDeletesNothing matters more here than elsewhere: this is
// the one act in the module that cannot be undone.
func TestCancelledContextDeletesNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, as: "sender", revoked: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := revoker(p).ForEveryone(ctx, "MSG1", false, "t/rev"); err == nil {
		t.Fatal("a cancelled context deleted a message for everyone")
	}
	if p.kicks != 0 {
		t.Fatalf("deleted %d message(s) for a caller that had given up", p.kicks)
	}
}

func TestAStalledPageIsNotADeletion(t *testing.T) {
	compressClock(t)
	p := &stallingDouble{}
	_, err := New(engine.NewRunner(), p.eval).ForEveryone(context.Background(), "MSG1", false, "t/rev")
	if !errors.Is(err, ErrRevoke) {
		t.Fatalf("got %v, want ErrRevoke", err)
	}
	if !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a stall must be distinguishable from a refusal: %v", err)
	}
}

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

// localDouble answers ForMe's postcondition field, which pageDouble does not
// carry — and that is deliberate rather than laziness. A double that answered
// both `revoked` and `loaded` would let a ForMe test pass while the script asked
// the revoked question, which is exactly the confusion ErrStillLoaded exists to
// prevent.
type localDouble struct {
	ok     bool
	stage  string
	why    string
	loaded bool

	kicks       int
	loadedReads int
	lastScript  string
}

func (p *localDouble) eval(ctx context.Context, expr string, out *string) error {
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
	// A PERGUNTA DA POS-CONDICAO E' UM SCRIPT SEPARADO, e o duble responde a ela
	// separadamente de proposito: se as duas respostas viessem do mesmo campo, um
	// ForMe que nunca perguntasse passaria.
	if strings.Contains(expr, "return { loaded: true }") {
		p.loadedReads++
		*out = fmt.Sprintf(`{"loaded":%t}`, p.loaded)
		return nil
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "delete"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = `{"stage":"done","ok":true,"why":""}`
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func localRevoker(p *localDouble) *Revoker { return New(engine.NewRunner(), p.eval) }

// THE POSTCONDITION. A local delete that leaves the message in the collection
// did not happen, and saying otherwise is the silent success invariant 14 exists
// to forbid.
func TestALocallyDeletedMessageStillLoadedIsAFailure(t *testing.T) {
	compressClock(t)
	p := &localDouble{ok: true, loaded: true}
	if _, err := localRevoker(p).ForMe(context.Background(), "3EB0", false, "t"); !errors.Is(err, ErrStillLoaded) {
		t.Fatalf("want ErrStillLoaded, got %v", err)
	}
}

func TestALocalDeleteThatLandsReportsItself(t *testing.T) {
	compressClock(t)
	p := &localDouble{ok: true, loaded: false}
	res, err := localRevoker(p).ForMe(context.Background(), "3EB0", false, "t")
	if err != nil {
		t.Fatalf("ForMe: %v", err)
	}
	if res.As != asLocal {
		t.Fatalf("a local delete claimed entitlement %q; it uses none", res.As)
	}
}

// THE LOCAL DELETE MUST NOT ASK PERMISSION. canSenderRevokeMsg answers a
// question about OTHER PEOPLE's copies, and consulting it here would refuse —
// on their behalf — an act that never leaves this device. It is asserted on the
// script because the double supplies the outcome either way.
func TestTheLocalDeleteDoesNotConsultTheRevokeEntitlement(t *testing.T) {
	compressClock(t)
	p := &localDouble{ok: true, loaded: false}
	if _, err := localRevoker(p).ForMe(context.Background(), "3EB0", false, "t"); err != nil {
		t.Fatalf("ForMe: %v", err)
	}
	for _, cap := range []string{"canSenderRevokeMsg", "canAdminRevokeMsg", "sendRevoke"} {
		if strings.Contains(p.lastScript, cap) {
			t.Errorf("the local delete script uses %q, which belongs to the "+
				"for-everyone path and would refuse a purely local act", cap)
		}
	}
	if !strings.Contains(p.lastScript, "sendDeleteMsgs") {
		t.Fatal("the script does not call sendDeleteMsgs, which is the name this " +
			"build actually exports for a local delete")
	}
}

// THE COLLECTION IS RE-READ, not the model in hand. A removed model stays a
// perfectly valid object in the variable that holds it, so asking IT whether it
// is gone answers "no" forever.
func TestTheLocalDeleteVerifiesAgainstTheCollection(t *testing.T) {
	compressClock(t)
	p := &localDouble{ok: true, loaded: false}
	if _, err := localRevoker(p).ForMe(context.Background(), "3EB0", false, "t"); err != nil {
		t.Fatalf("ForMe: %v", err)
	}
	if p.loadedReads == 0 {
		t.Fatal("the postcondition never asked the collection whether the message " +
			"is still there, so it would confirm the deletion against the very " +
			"object it just deleted")
	}
}

func TestAnEmptyIDNeverReachesThePageForALocalDelete(t *testing.T) {
	p := &localDouble{ok: true}
	if _, err := localRevoker(p).ForMe(context.Background(), "  ", false, "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("want ErrNoMessage, got %v", err)
	}
	if p.kicks != 0 {
		t.Fatalf("an empty id reached the page %d times", p.kicks)
	}
}

func TestAMessageNotLoadedCannotBeDeletedLocally(t *testing.T) {
	compressClock(t)
	p := &localDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := localRevoker(p).ForMe(context.Background(), "3EB0", false, "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("want ErrNoMessage, got %v", err)
	}
}

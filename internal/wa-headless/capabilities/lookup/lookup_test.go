package lookup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// pageDouble imitates ONE REAL RULE, and it is the rule the whole package exists
// for: this build is LID-first, so the identity the server returns is NOT the
// jid that was asked for. Measured across this repository — 397 of 399 messages
// under @lid (H34) — and a double that echoed the input would be more agreeable
// than the world and would let a caller believe no resolution happened.
type pageDouble struct {
	answer     string
	lastScript string
	kicks      int
}

func (d *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func res(d *pageDouble) *Resolver { return New(engine.NewRunner(), d.eval) }

// THE RESOLUTION IS REPORTED, NOT HIDDEN. A caller that asked about a phone jid
// and got a LID back needs to know the two differ — that is the entire point of
// asking.
func TestAResolvedIdentitySaysItResolved(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"123456789@lid","isGroup":false}`}
	got, err := res(d).NumberID(context.Background(), "5541999998888@c.us", "t")
	if err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if !got.Resolved {
		t.Error("Resolved is false although the server answered a different jid")
	}
	if got.AskedFor == "" {
		t.Error("AskedFor was dropped, so the caller cannot compare")
	}
	if got.JID == got.AskedFor {
		t.Error("the answer echoes the input")
	}
	if s := got.String(); strings.Contains(s, "5541") || strings.Contains(s, "123456789") {
		t.Errorf("Identity.String carries identity: %s", s)
	}
}

// AND AN UNRESOLVED ONE SAYS SO. Same jid back is a legitimate answer and must
// not read as a resolution.
func TestAnEchoedIdentityIsNotAResolution(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"1@lid","isGroup":false}`}
	got, err := res(d).NumberID(context.Background(), "1@lid", "t")
	if err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if got.Resolved {
		t.Error("Resolved is true for an answer identical to the input")
	}
}

// NOT ON WHATSAPP IS ITS OWN ERROR. The question was asked and answered; a
// caller deciding whether to offer a send button needs that apart from a read
// that failed.
func TestNotOnWhatsAppIsItsOwnError(t *testing.T) {
	d := &pageDouble{answer: `{"ok":false,"why":"` + whyNotOnWhatsApp + `"}`}
	_, err := res(d).NumberID(context.Background(), "5500000000000@c.us", "t")
	if !errors.Is(err, ErrNotOnWhatsApp) {
		t.Fatalf("err = %v, want ErrNotOnWhatsApp", err)
	}
	if errors.Is(err, ErrRead) {
		t.Error("a definite no is being reported as a failed read")
	}
	other := &pageDouble{answer: `{"ok":false,"why":"WID_NULL"}`}
	_, err = res(other).NumberID(context.Background(), "x@c.us", "t")
	if !errors.Is(err, ErrRead) || errors.Is(err, ErrNotOnWhatsApp) {
		t.Fatalf("err = %v; a different refusal must not read as absent", err)
	}
}

// A GROUP SHORT-CIRCUITS. The resolution answers PEOPLE, and asking it about a
// group would answer NOT_ON_WHATSAPP for something that plainly exists — which
// spa/identity.go already handles and this package must not undo.
func TestAGroupIsCarriedThroughAsAGroup(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"120-1@g.us","isGroup":true}`}
	got, err := res(d).NumberID(context.Background(), "120-1@g.us", "t")
	if err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if !got.IsGroup {
		t.Error("the group flag was lost")
	}
}

// THE SCRIPT EMBEDS THE MODULE'S OWN RESOLUTION, and does not carry a copy.
//
// This is the assertion that matters most here: a pasted copy would be a second
// resolution free to drift from the one send uses, and the drift would surface
// as a send failing for a number this package had just approved.
func TestTheScriptEmbedsTheSharedResolution(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"1@lid"}`}
	if _, err := res(d).NumberID(context.Background(), "1@c.us", "t"); err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if !strings.Contains(d.lastScript, spa.ResolveIdentityExpr) {
		t.Fatal("the script does not embed spa.ResolveIdentityExpr; a copy would " +
			"drift from the resolution send actually uses")
	}
}

// AND THE REASON STRING IS THE SAME ONE spa ANSWERS. A rename on that side would
// otherwise turn a definite "not on WhatsApp" into a generic read failure.
func TestTheAbsentReasonMatchesTheSharedExpression(t *testing.T) {
	if !strings.Contains(spa.ResolveIdentityExpr, whyNotOnWhatsApp) {
		t.Fatalf("spa.ResolveIdentityExpr no longer answers %q; this package would "+
			"stop recognising a number that does not exist", whyNotOnWhatsApp)
	}
}

func TestAnEmptyJIDNeverReachesThePage(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true}`}
	if _, err := res(d).NumberID(context.Background(), "  ", "t"); !errors.Is(err, ErrNoJID) {
		t.Fatalf("err = %v, want ErrNoJID", err)
	}
	if d.kicks != 0 {
		t.Error("an empty jid reached the page")
	}
}

func TestACancelledCallerNeverAsks(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"1@lid"}`}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := res(d).NumberID(ctx, "1@c.us", "t"); err == nil {
		t.Fatal("a cancelled context produced an answer")
	}
	if d.kicks != 0 {
		t.Error("the page was asked for a caller that had given up")
	}
}

func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &pageDouble{answer: ""}
	_, err := res(d).NumberID(context.Background(), "1@c.us", "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

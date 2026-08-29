package lookup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
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
	releases   int
}

func (d *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' LEITURA (H178): ela roda depois de a resposta ser tomada.
	if strings.Contains(expr, "delete window."+stateKeyPrefix) {
		d.releases++
		*out = "ok"
		return nil
	}
	if strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func res(d *pageDouble) *Resolver { return New(engine.NewRunner(), d.eval) }

// withoutComments strips // comments before a script is asserted against.
//
// IT EXISTS BECAUSE A GUARD MATCHED ITS OWN COMMENT, which is the tenth time
// this repository has hit that: the assertion "the script does not call
// getCurrentLid" failed against a comment EXPLAINING why it does not call
// getCurrentLid. A test that reads prose as code passes and fails for reasons
// unrelated to behaviour.
func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

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

// A LID INPUT DOES NOT QUERY. The identity that was handed in is already one
// half of the answer, and asking the server for something already in hand would
// spend a round trip per contact on a roster walk.
func TestALidInputIsNotResolvedAgain(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"lid":"1@lid","pn":"","queried":false}`}
	got, err := res(d).LidAndPhone(context.Background(), "1@lid", "t")
	if err != nil {
		t.Fatalf("LidAndPhone: %v", err)
	}
	if got.Queried {
		t.Fatal("a lid input reported a server query")
	}
	// A REGRA E' ESTRUTURAL E TEM DE SER AFIRMADA SOBRE O SCRIPT. O duble
	// devolve `queried` a partir do seu proprio campo, entao um script que
	// consultasse assim mesmo passaria — foi o que um primeiro controle
	// negativo mostrou ao nao morder.
	code := withoutComments(d.lastScript)
	iReturn := strings.Index(code, "queried: false })")
	iResolve := strings.Index(code, "await resolve(")
	if iReturn < 0 || iResolve < 0 {
		t.Fatalf("script shape changed (return=%d resolve=%d)", iReturn, iResolve)
	}
	if iResolve < iReturn {
		t.Fatal("the resolution is reached before the lid branch returns, so a lid " +
			"input would still cost a server round trip per contact")
	}
	if got.LID != "1@lid" {
		t.Fatalf("the lid that was handed in did not come back: %#v", got)
	}
	if got.Complete() {
		t.Fatal("Complete() claimed both halves when the phone side is empty")
	}
}

// AN ABSENT HALF STAYS ABSENT. The whole point of this type is to keep "the page
// did not produce it" distinct from "there is none", and a version that filled
// the gap by echoing the input would report a phone number it invented.
func TestAnAbsentHalfIsNotFilledIn(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"lid":"1@lid","pn":"","queried":false}`}
	got, err := res(d).LidAndPhone(context.Background(), "1@lid", "t")
	if err != nil {
		t.Fatalf("LidAndPhone: %v", err)
	}
	if got.PN != "" {
		t.Fatalf("the missing phone side was filled in with %q", got.PN)
	}
}

// THE SCRIPT BRANCHES ON isLid BEFORE ASKING FOR THE PHONE NUMBER. Calling
// getPhoneNumber with a phone jid throws "WaWebLidPnCache - Invalid get call
// (not lid)" — measured by making that exact mistake.
func TestThePairScriptGuardsGetPhoneNumber(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"lid":"1@lid","pn":"","queried":false}`}
	if _, err := res(d).LidAndPhone(context.Background(), "1@lid", "t"); err != nil {
		t.Fatalf("LidAndPhone: %v", err)
	}
	code := d.lastScript
	iGuard := strings.Index(code, `asked.server === "lid"`)
	iCall := strings.Index(code, "getPhoneNumber")
	if iGuard < 0 || iCall < 0 {
		t.Fatalf("script missing the guard or the call (guard=%d call=%d)", iGuard, iCall)
	}
	if iGuard > iCall {
		t.Fatal("getPhoneNumber is reached before the isLid guard, so a phone jid " +
			"would throw inside the page")
	}
}

// THE PHONE SIDE DOES NOT COME FROM getCurrentLid, and this is the divergence
// from the reference. Its helper calls queryWidExists and then getCurrentLid
// again; measured here, that second call still comes back empty for a peer this
// module resolves every day, so the helper would answer {} about somebody
// perfectly reachable. The lid comes from the resolution's own result.
func TestThePairScriptTakesTheLidFromTheResolution(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"lid":"1@lid","pn":"55@c.us","queried":true}`}
	if _, err := res(d).LidAndPhone(context.Background(), "55@c.us", "t"); err != nil {
		t.Fatalf("LidAndPhone: %v", err)
	}
	if strings.Contains(withoutComments(d.lastScript), "getCurrentLid") {
		t.Fatal("the script asks getCurrentLid, which was measured empty even after " +
			"the query on this build; the lid is in the resolution result")
	}
	if !strings.Contains(d.lastScript, spa.ResolveIdentityExpr) {
		t.Fatal("the script does not embed the shared resolution, so this package " +
			"now holds a second opinion competing with the one send uses")
	}
}

func TestAPairForSomebodyNotOnWhatsAppIsItsOwnError(t *testing.T) {
	d := &pageDouble{answer: `{"ok":false,"why":"` + whyNotOnWhatsApp + `"}`}
	if _, err := res(d).LidAndPhone(context.Background(), "55@c.us", "t"); !errors.Is(err, ErrNotOnWhatsApp) {
		t.Fatalf("want ErrNotOnWhatsApp, got %v", err)
	}
}

func TestAnEmptyJIDNeverReachesThePageForAPair(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true}`}
	if _, err := res(d).LidAndPhone(context.Background(), "  ", "t"); !errors.Is(err, ErrNoJID) {
		t.Fatalf("want ErrNoJID, got %v", err)
	}
	if d.kicks != 0 {
		t.Fatalf("an empty jid reached the page %d times", d.kicks)
	}
}

// The rendering carries shape, never an identity.
func TestThePairRenderingIsQuiet(t *testing.T) {
	p := Pair{LID: "1234567890@lid", PN: "5516999999999@c.us", Queried: true}
	s := p.String()
	for _, leak := range []string{"1234567890", "5516", "999999999"} {
		if strings.Contains(s, leak) {
			t.Fatalf("the rendering leaks %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, "lid=true") || !strings.Contains(s, "pn=true") {
		t.Fatalf("the rendering lost the shape: %s", s)
	}
}

// TWO RESOLUTIONS MUST NOT SHARE A PAGE GLOBAL (H178).
//
// This package is the one most exposed to the crossing measured in H177,
// because it runs INSIDE other capabilities: a send resolving an identity while
// a reader polls is the ordinary production shape, not a contrived race.
func TestTwoResolutionsDoNotShareAKey(t *testing.T) {
	a, b := nextStateKey(), nextStateKey()
	if a == b {
		t.Fatalf("two resolutions got the same key %q; concurrent callers would "+
			"overwrite each other's answers", a)
	}
	if !strings.HasPrefix(a, stateKeyPrefix) {
		t.Fatalf("the key left the module's namespace: %q", a)
	}
}

func TestTheResolveScriptParksOnTheGivenKey(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"1@lid"}`}
	if _, err := res(d).NumberID(context.Background(), "55@c.us", "t"); err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if strings.Contains(d.lastScript, "window."+stateKeyPrefix+" =") {
		t.Fatal("the script writes the shared global directly; two concurrent " +
			"resolutions would overwrite each other again")
	}
	if !strings.Contains(d.lastScript, stateKeyPrefix+"_") {
		t.Fatal("the script does not park on a per-call key")
	}
}

func TestTheResolutionKeyIsReleased(t *testing.T) {
	d := &pageDouble{answer: `{"ok":true,"jid":"1@lid"}`}
	if _, err := res(d).NumberID(context.Background(), "55@c.us", "t"); err != nil {
		t.Fatalf("NumberID: %v", err)
	}
	if d.releases == 0 {
		t.Fatal("the per-call key was never released; this package resolves inside " +
			"others, so a long session would accumulate one global per resolution")
	}
}

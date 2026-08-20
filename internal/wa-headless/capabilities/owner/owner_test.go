package owner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble answers with raw JSON text, because that is what the production
// evaluator hands back — spa.Evaluator decodes into a *string and this package
// unmarshals it. A double returning a Go struct would skip the decoding the
// capability actually does, and decoding is where a shape change bites first
// (ARMADILHAS §1: the double must imitate the REAL rule).
type pageDouble struct {
	answer string
	err    error
	calls  int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	//
	// engine.Tab.Evaluate derives from the caller's context, so a cancelled or
	// expired ctx fails there. A double that ignored ctx would be better
	// behaved than the world in exactly the axis ARMADILHAS §1 warns about —
	// and it would let a capability fabricate a successful answer for a caller
	// that had already given up.
	if err := ctx.Err(); err != nil {
		return err
	}
	p.calls++
	if p.err != nil {
		return p.err
	}
	*out = p.answer
	return nil
}

func refresh(t *testing.T, p *pageDouble) (Identity, error) {
	t.Helper()
	return Refresh(context.Background(), engine.NewRunner(), p.eval, "test/owner")
}

const paired = `{"ok":true,
	"pn":{"user":"5511999999999","server":"c.us","_serialized":"5511999999999@c.us"},
	"lid":{"user":"123456789","server":"lid","_serialized":"123456789@lid"},
	"display_name":"Someone"}`

func TestReadsBothIdentifiersSeparately(t *testing.T) {
	got, err := refresh(t, &pageDouble{answer: paired})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !got.Present() {
		t.Fatal("Present() is false on a paired answer")
	}
	// The DIVERGENCE from whatsapp-web.js, asserted: both survive. wwebjs does
	// `pn || lid`, which would leave LID empty here, and a caller could not
	// tell whether it holds a phone-number identity or a LID.
	if !got.PN.Present() || !got.LID.Present() {
		t.Fatalf("pn_present=%v lid_present=%v: both identifiers must survive. "+
			"Collapsing them the way wwebjs does discards WHICH one answered, and "+
			"LID and PN are not interchangeable when talking to the server",
			got.PN.Present(), got.LID.Present())
	}
	if got.PN.Server != "c.us" || got.LID.Server != "lid" {
		t.Fatalf("pn.server=%q lid.server=%q: the two were swapped or merged",
			got.PN.Server, got.LID.Server)
	}
	if got.DisplayName == "" {
		t.Fatal("display name was dropped")
	}
}

// TestStringRedacts is the PII guard, and it is a test rather than a comment
// because the failure mode is someone writing %v in a log years from now.
func TestStringRedacts(t *testing.T) {
	got, err := refresh(t, &pageDouble{answer: paired})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	for _, rendered := range []string{got.String(), got.GoString(), got.PN.String()} {
		for _, secret := range []string{"5511999999999", "123456789", "Someone"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("a rendered Identity leaked %q: %s. This value IS the account, "+
					"and a %%v in a log is the way it escapes", secret, rendered)
			}
		}
	}
	if !strings.Contains(got.String(), "pn=true") {
		t.Fatalf("String() should still report SHAPE so it is useful: %s", got.String())
	}
}

// TestAnsweredButLoggedOutIsNotAProbeError separates the two negatives. A page
// that answers "no account" is a working browser showing a QR; a page that
// could not be asked tells us nothing. Same repair for both would be wrong.
func TestAnsweredButLoggedOutIsNotAProbeError(t *testing.T) {
	_, err := refresh(t, &pageDouble{answer: `{"ok":true,"pn":null,"lid":null,"display_name":""}`})
	if !errors.Is(err, ErrNoOwner) {
		t.Fatalf("err = %v, want ErrNoOwner", err)
	}
}

func TestProbeFailureIsNotReportedAsNoOwner(t *testing.T) {
	boom := errors.New("evaluate exploded")
	_, err := refresh(t, &pageDouble{err: boom})
	if errors.Is(err, ErrNoOwner) {
		t.Fatal("a page that could not be ASKED was reported as having no owner; that is " +
			"an instrument inventing a result about the account")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the probe error", err)
	}
}

// TestGarbledAnswerIsNotNoOwner is the third negative, and it is separate from
// the second because a JSON shape change ships with a Meta build: blaming the
// account for it would send someone to re-pair a perfectly good session.
func TestGarbledAnswerIsNotNoOwner(t *testing.T) {
	_, err := refresh(t, &pageDouble{answer: `not json at all`})
	if err == nil {
		t.Fatal("a garbled answer produced no error")
	}
	if errors.Is(err, ErrNoOwner) {
		t.Fatal("a parsing failure was reported as a logged-out session")
	}
}

// TestUnresolvedModuleIsNamed covers the case the boot inventory is supposed to
// prevent. It should be unreachable in production — spa.ModuleUserPrefsMeUser is
// verified at startup since 2026-08-19 — and the message says so, because a
// failure that contradicts an invariant is worth more than a generic one.
func TestUnresolvedModuleIsNamed(t *testing.T) {
	_, err := refresh(t, &pageDouble{answer: `{"ok":false}`})
	if err == nil {
		t.Fatal("an unresolved module produced no error")
	}
	if errors.Is(err, ErrNoOwner) {
		t.Fatal("an unresolved MODULE was reported as a logged-out ACCOUNT")
	}
	if !strings.Contains(err.Error(), "WAWebUserPrefsMeUser") {
		t.Fatalf("the error does not name the module: %v", err)
	}
}

// TestOnlyTheIdentityModuleIsRead locks the DoD from EXECUCAO.md LOOP 04.3:
// the inventory grows only with what this capability uses. A capability that
// quietly reached for a second module would make the boot's guarantee
// incomplete without anything failing.
func TestOnlyTheIdentityModuleIsRead(t *testing.T) {
	script := readScript()
	const want = "WAWebUserPrefsMeUser"
	if !strings.Contains(script, want) {
		t.Fatalf("the script does not read %s at all", want)
	}
	if n := strings.Count(script, "window.require("); n != 1 {
		t.Fatalf("the script calls window.require %d times; this capability declares one "+
			"module and the boot inventory only guarantees the ones declared", n)
	}
}

// TestCancelledContextIsNotAFabricatedSuccess exercises the axis the doubles
// used to ignore.
//
// engine.Tab.Evaluate derives from the caller's context, so in production a
// cancelled ctx fails. The doubles in this file returned their canned answer
// regardless, which made every capability test blind to cancellation: a
// capability that swallowed the error from runner.Do would still pass.
//
// A caller that has given up must not receive a manufactured answer — that is
// worse than an error, because it looks like data.
func TestCancelledContextIsNotAFabricatedSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := func() error {
		_, e := Refresh(ctx, engine.NewRunner(), (&pageDouble{answer: paired}).eval, "t/cancel")
		return e
	}(); err == nil {
		t.Fatal("a cancelled context produced a successful answer; the caller had already " +
			"given up and got manufactured data instead of an error")
	}
}

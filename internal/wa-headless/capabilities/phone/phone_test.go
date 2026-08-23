package phone

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates ONE REAL RULE, and it is the rule that gives this package
// a reason to exist: THE PAGE DOES NOT REFUSE GARBAGE. Measured 2026-08-22
// against this build (probe_settings_test.go):
//
//	findCC("notaphone") -> "not"
//	findCC("1")         -> "1"
//
// So the double answers findCC by returning the first three characters of
// whatever it is handed, exactly as the page does. A double that refused
// nonsense would be stricter than the world and would hide the very defect the
// input guard exists to prevent (ARMADILHAS §1).
type pageDouble struct {
	answer     string
	lastScript string
	kicks      int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' LEITURA (H177): ela roda DEPOIS de a resposta ser
	// tomada, e conta-la faz um teste de numero de voltas medir uma volta que
	// nao existe.
	if strings.Contains(expr, "delete window."+stateKeyPrefix) {
		return nil
	}
	if strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		*out = p.answer
		return nil
	}
	p.kicks++
	p.lastScript = expr
	d := between(expr, `const d = "`, `"`)
	// The page's actual behaviour: the first three characters, whatever they are.
	cc := d
	if len(cc) > 3 {
		cc = cc[:3]
	}
	p.answer = `{"ok":true,"formatted":"+` + d + `","cc":"` + cc + `"}`
	*out = "kicked"
	return nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

func rd(p *pageDouble) *Reader { return New(engine.NewRunner(), p.eval) }

// GARBAGE NEVER REACHES THE PAGE, because the page answers it.
//
// This is the measured defect: findCC("notaphone") is "not", which has the exact
// shape of a real country code. A caller would believe it.
func TestGarbageNeverReachesThePage(t *testing.T) {
	for _, junk := range []string{"notaphone", "55abc41", "hello world", "++", "e10"} {
		p := &pageDouble{}
		if _, err := rd(p).Lookup(context.Background(), junk, "t"); !errors.Is(err, ErrNotANumber) {
			t.Errorf("Lookup(junk) err = %v, want ErrNotANumber", err)
		}
		if p.kicks != 0 {
			t.Errorf("junk reached the page %d time(s); findCC would have answered it", p.kicks)
		}
	}
}

// AND THE ERROR MUST NOT QUOTE THE INPUT. An error is a log line waiting to
// happen, and this one would carry a phone number.
func TestTheRefusalDoesNotQuoteTheNumber(t *testing.T) {
	p := &pageDouble{}
	_, err := rd(p).Lookup(context.Background(), "5541999998888x", "t")
	if err == nil {
		t.Fatal("a number with a letter was accepted")
	}
	if strings.Contains(err.Error(), "5541") || strings.Contains(err.Error(), "999998888") {
		t.Fatalf("the error carries the input: %v", err)
	}
}

// A NONSENSE ANSWER IS REFUSED TOO, and it is a different error.
//
// The input guard cannot know every shape the page's table will accept, so the
// answer is checked as well. When that fires it means the guard has a gap —
// which is our bug, not the caller's, and the two errors say so.
func TestANonsenseAnswerIsItsOwnError(t *testing.T) {
	p := &pageDouble{}
	// Force the page to answer a non-numeric code for a well-formed input.
	if err := p.eval(context.Background(), "kick", new(string)); err != nil {
		t.Fatalf("priming the double: %v", err)
	}
	bad := &fixedDouble{answer: `{"ok":true,"formatted":"+55 41 9","cc":"not"}`}
	_, err := New(engine.NewRunner(), bad.eval).Lookup(context.Background(), "5541999998888", "t")
	if !errors.Is(err, ErrNonsenseAnswer) {
		t.Fatalf("err = %v, want ErrNonsenseAnswer", err)
	}
	if errors.Is(err, ErrNotANumber) {
		t.Error("a page-side gap is being reported as a caller mistake")
	}
}

type fixedDouble struct{ answer string }

func (p *fixedDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' LEITURA (H177): ela roda DEPOIS de a resposta ser
	// tomada, e conta-la faz um teste de numero de voltas medir uma volta que
	// nao existe.
	if strings.Contains(expr, "delete window."+stateKeyPrefix) {
		return nil
	}
	if strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		*out = p.answer
		return nil
	}
	*out = "kicked"
	return nil
}

// A REAL NUMBER SURVIVES ITS DECORATIONS, including the jid suffixes this build
// actually hands around — callers hold jids, not numbers.
func TestDecorationsAndJidSuffixesAreStripped(t *testing.T) {
	for _, in := range []string{
		"5541999998888", "+55 41 99999-8888", "(55) 41 999998888",
		"5541999998888@c.us", "5541999998888@s.whatsapp.net", "5541999998888@lid",
	} {
		got, ok := Digits(in)
		if !ok {
			t.Errorf("Digits(%q) refused a real number", in)
			continue
		}
		if got != "5541999998888" {
			t.Errorf("Digits(%q) = %q, want the bare digits", in, got)
		}
	}
}

func TestAnEmptyOrOverlongNumberIsRefused(t *testing.T) {
	for _, in := range []string{"", "   ", "@c.us", "1234567890123456"} {
		if _, ok := Digits(in); ok {
			t.Errorf("Digits(%q) accepted it", in)
		}
	}
}

// The successful path, and the SHAPE of the rendering must not leak into logs.
func TestASuccessfulLookupCarriesBothAndSaysNeither(t *testing.T) {
	p := &pageDouble{}
	got, err := rd(p).Lookup(context.Background(), "5541999998888@c.us", "t")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.CountryCode != "554" {
		t.Fatalf("cc = %q; the double answers the first three characters, as the page does", got.CountryCode)
	}
	if got.Formatted == "" {
		t.Error("the formatted rendering was dropped")
	}
	if s := got.String(); strings.Contains(s, "5541") || strings.Contains(s, "999998888") {
		t.Errorf("Number.String carries the number: %s", s)
	}
}

// THE PAGE IS ASKED WITH THE SUFFIX IT EXPECTS, which the checks above cannot
// see: the double supplies the answer. formattedPhoneNumber wants a jid.
func TestThePageIsAskedWithTheSuffixItExpects(t *testing.T) {
	p := &pageDouble{}
	if _, err := rd(p).Lookup(context.Background(), "5541999998888", "t"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !strings.Contains(p.lastScript, jidSuffix) {
		t.Error("the script does not append the jid suffix formattedPhoneNumber expects")
	}
	if !strings.Contains(p.lastScript, "findCC") {
		t.Error("the script never asks for a country code")
	}
}

func TestACancelledCallerNeverAsks(t *testing.T) {
	p := &pageDouble{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rd(p).Lookup(ctx, "5541999998888", "t"); err == nil {
		t.Fatal("a cancelled context produced an answer")
	}
	if p.kicks != 0 {
		t.Errorf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	stall := &fixedDouble{answer: ""}
	_, err := New(engine.NewRunner(), stall.eval).Lookup(context.Background(), "5541999998888", "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

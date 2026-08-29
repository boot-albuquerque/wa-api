package poll

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

type double struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		d.reads++
		if d.reads <= d.pendingReads {
			*out = ""
			return nil
		}
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func mgr(d *double) *Manager { return New(engine.NewRunner(), d.eval) }

func fast(t *testing.T) {
	t.Helper()
	ob, ot := Budget, Tick
	Budget, Tick = 300*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { Budget, Tick = ob, ot })
}

// withoutComments removes // line comments so an assertion about what a script
// DOES is not satisfied by what it SAYS. Duplicated per package because a test
// helper cannot cross packages without becoming production code nothing calls;
// this repository has written a guard that matched its own prose eight times.
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

// An empty message id never reaches the page.
func TestAnEmptyMessageIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := mgr(d).Votes(context.Background(), "  ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Errorf("Votes = %v, want ErrNoMessage", err)
	}
	if err := mgr(d).Vote(context.Background(), "", []string{"A"}, "t"); !errors.Is(err, ErrNoMessage) {
		t.Errorf("Vote = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty message id reached the page")
	}
}

// THE PAGE'S REASONS BECOME THIS PACKAGE'S ERRORS, so a caller reacts without
// parsing strings — and "not found" and "not a poll" stay apart, because one is
// a stale id and the other is a caller pointing at the wrong thing.
func TestThePageReasonsBecomeTypedErrors(t *testing.T) {
	for why, want := range map[string]error{
		"NO_MESSAGE":        ErrNotFound,
		"NOT_A_POLL":        ErrNotAPoll,
		"NO_OPTION_MATCHED": ErrUnknownOption,
	} {
		d := &double{answer: `{"ok":false,"why":"` + why + `"}`}
		if _, err := mgr(d).Votes(context.Background(), "X", "t"); !errors.Is(err, want) {
			t.Errorf("Votes with %s = %v, want %v", why, err, want)
		}
	}
	// Anything else keeps the page's words behind the generic error.
	d := &double{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	if _, err := mgr(d).Votes(context.Background(), "X", "t"); !errors.Is(err, ErrRead) ||
		!strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

// EVERY OPTION IS PRESENT, INCLUDING THE UNVOTED ONES. A tally that omitted them
// would make "nobody chose this" indistinguishable from "this build did not
// report it".
func TestUnvotedOptionsArePresentWithZero(t *testing.T) {
	d := &double{answer: `{"ok":true,"options":{"0":3,"1":0,"2":1},"voters":4,"rows":5}`}
	got, err := mgr(d).Votes(context.Background(), "X", "t")
	if err != nil {
		t.Fatalf("Votes: %v", err)
	}
	if len(got.Options) != 3 {
		t.Fatalf("options = %v, want all three", got.Options)
	}
	if got.Options[1] != 0 {
		t.Errorf("an unvoted option is missing or wrong: %v", got.Options)
	}
	// ROWS AND VOTERS ARE DIFFERENT NUMBERS, and conflating them would hide
	// that somebody changed their mind.
	if got.Rows == got.Voters {
		t.Error("rows and voters were collapsed into one number")
	}

	// AND THE ZERO-FILL IS ASSERTED ON THE SCRIPT, because that is where it
	// lives.
	//
	// The check above passes against a double that was TOLD the answer, so
	// deleting the page-side pre-fill left it green — a negative control that
	// did not bite, which is how this gap was found. The parsing and the
	// producing are two different properties and only one of them was tested.
	if !strings.Contains(withoutComments(d.lastScript), `options[String(o.localId)] = 0;`) {
		t.Fatal("the page script does not pre-fill every option with zero; an unvoted " +
			"option would be indistinguishable from one this build did not report")
	}
}

// A NAME THAT MATCHES NOTHING IS A REFUSAL, NOT AN OMISSION.
//
// The page's send takes a SET of local ids. An unmatched name silently produces
// a smaller set — a vote for fewer things than the caller asked for, reported as
// success.
func TestAPartiallyMatchedVoteIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true,"matched":1}`}
	err := mgr(d).Vote(context.Background(), "X", []string{"A", "B"}, "t")
	if !errors.Is(err, ErrUnknownOption) {
		t.Fatalf("err = %v, want ErrUnknownOption", err)
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("the error does not say how many matched: %v", err)
	}

	d2 := &double{answer: `{"ok":true,"matched":2}`}
	if err := mgr(d2).Vote(context.Background(), "X", []string{"A", "B"}, "t"); err != nil {
		t.Fatalf("a fully matched vote failed: %v", err)
	}
}

// An empty option list is refused rather than sent as a withdrawal, because the
// two are indistinguishable and only one was asked for.
func TestAnEmptyVoteIsRefused(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"", "   "}} {
		d := &double{answer: `{"ok":true}`}
		if err := mgr(d).Vote(context.Background(), "X", in, "t"); !errors.Is(err, ErrUnknownOption) {
			t.Errorf("Vote(%v) = %v, want ErrUnknownOption", in, err)
		}
		if d.kicks != 0 {
			t.Error("an empty vote reached the page")
		}
	}
}

// THE KEY IS THE MESSAGE'S OWN, NOT ONE REBUILT FROM A STRING.
//
// The reference does MsgKey.fromString(msg.id._serialized), which throws on this
// build. Going back to it would break the read with a message about a null
// string, in a place where a caller would read it as "no votes".
func TestTheVoteReadUsesTheMessagesOwnKey(t *testing.T) {
	d := &double{answer: `{"ok":true,"options":{}}`}
	if _, err := mgr(d).Votes(context.Background(), "X", "t"); err != nil {
		t.Fatalf("Votes: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, `equals(["parentMsgKey"], m.id.toString())`) {
		t.Error("the read does not query by the message's own key")
	}
	if strings.Contains(code, "fromString(") {
		t.Error("the read rebuilds the key from a string; that throws on this build")
	}
}

// The parked loop is bounded and polls.
func TestTheParkedLoopIsBounded(t *testing.T) {
	fast(t)
	d := &double{pendingReads: 1 << 30}
	if _, err := mgr(d).Votes(context.Background(), "X", "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

// The tally renders no identity and no option text.
func TestTheTallyIsQuiet(t *testing.T) {
	tl := Tally{MessageID: "3EB0ABCDEF", Options: map[int]int{0: 2}, Voters: 2, Rows: 3}
	if strings.Contains(tl.String(), "3EB0") {
		t.Errorf("Tally.String carries the message id: %s", tl.String())
	}
}

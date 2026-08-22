package message

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
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
	if strings.HasPrefix(expr, "window."+stateKey) {
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

func rd(d *double) *Reader { return New(engine.NewRunner(), d.eval) }

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

// An empty id never reaches the page.
func TestAnEmptyMessageIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).OriginOf(context.Background(), "  ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Errorf("err = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty id reached the page")
	}
}

// THE SENDER OF A GROUP MESSAGE IS NOT THE CHAT.
//
// In a one-to-one the participant is null and the sender IS the chat; in a group
// the participant is who spoke. Conflating them attributes the speech to the
// group, which is the whole reason Origin has two fields.
func TestTheGroupSenderIsNotTheChat(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"120-1@g.us","group":true,` +
		`"sender":"5541999999999@lid","fromMe":false,"t":1700000000}`}
	got, err := rd(d).OriginOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("OriginOf: %v", err)
	}
	if got.SenderJID == got.ChatJID {
		t.Fatal("the sender equals the chat on a group message")
	}
	if got.SenderIsChat {
		t.Error("SenderIsChat is true for a group message")
	}
	if !got.IsGroup {
		t.Error("the group flag was lost")
	}
	if got.At.IsZero() {
		t.Error("the timestamp was dropped")
	}

	// AND THE SCRIPT MUST DERIVE IT, which the check above cannot see.
	//
	// The double supplies `sender` ready-made, so replacing the whole derivation
	// with `sender = chat` left this test green — a negative control that did
	// not bite, and the third time today that a check asserted on the PARSING
	// while the property lived in the PRODUCTION.
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "id.participant") {
		t.Fatal("the script does not read the participant; in a group the sender " +
			"would collapse into the chat")
	}
}

// And in a one-to-one they legitimately coincide, which is recorded rather than
// hidden — a caller comparing them should not have to.
func TestInAOneToOneTheSenderIsTheChat(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"1@lid","group":false,"sender":"1@lid"}`}
	got, err := rd(d).OriginOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("OriginOf: %v", err)
	}
	if !got.SenderIsChat {
		t.Fatal("SenderIsChat is false when they are the same jid")
	}
}

// GROUP IS ASKED, NOT INFERRED FROM THE SUFFIX. Inferring from "@g.us" is one
// build change away from being wrong, and this build has already changed its
// identity namespace once (LID).
func TestTheGroupFlagIsAskedOfThePage(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"1@g.us"}`}
	_, _ = rd(d).OriginOf(context.Background(), "3EB0", "t")
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "getIsGroup") {
		t.Error("the script does not ask the page whether the chat is a group")
	}
}

// A message this session has not loaded is its own error, not a read failure:
// loading is the caller's job and the repairs differ.
func TestAnUnloadedMessageIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).OriginOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	d2 := &double{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	_, err := rd(d2).OriginOf(context.Background(), "3EB0", "t")
	if !errors.Is(err, ErrRead) || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrRead and not ErrNotFound", err)
	}
}

// The lookup falls back to a scan by RAW id, because that is the id every other
// capability in this module reports and therefore the only one a caller has.
func TestTheLookupScansByTheRawID(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	_, _ = rd(d).OriginOf(context.Background(), "3EB0", "t")
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "c.id.id ===") {
		t.Error("the script does not scan by the raw message id")
	}
}

// The rendering carries no identity.
func TestTheOriginRenderingIsQuiet(t *testing.T) {
	o := Origin{ChatJID: "120-1@g.us", SenderJID: "5541999999999@lid", IsGroup: true}
	s := o.String()
	if strings.Contains(s, "5541") || strings.Contains(s, "120-1") {
		t.Errorf("Origin.String carries identity: %s", s)
	}
}

// The parked loop is bounded.
func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{pendingReads: 1 << 30}
	if _, err := rd(d).OriginOf(context.Background(), "3EB0", "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

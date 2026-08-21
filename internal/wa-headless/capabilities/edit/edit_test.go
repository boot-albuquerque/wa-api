package edit

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
	ok                bool
	stage, why        string
	fromLen, toLen    int
	applied, recorded bool

	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
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
		*out = fmt.Sprintf(
			`{"stage":"done","ok":true,"why":"","fromLen":%d,"toLen":%d,"applied":%t,"recorded":%t}`,
			p.fromLen, p.toLen, p.applied, p.recorded)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func editor(p *pageDouble) *Editor { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := editBudget, editTick
	editBudget, editTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { editBudget, editTick = ob, ot })
}

const msgID = "3EB0000000000000000000"

// TestTheCallIsThreePositional. sendMessageEdit is SYNCHRONOUS, which is the
// only reason its shape could be read at all — ARMADILHAS.md records that the
// async siblings return an apply(this, arguments) wrapper and tell you nothing.
// This test freezes what the readable one said.
func TestTheCallIsThreePositional(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 9, applied: true, recorded: true}
	if _, err := editor(p).Text(context.Background(), msgID, "new text", "t"); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.sendMessageEdit(msg, text, {})") {
		t.Fatal("the call does not use the measured three-positional shape")
	}
	if strings.Contains(p.lastScript, "sendMessageEdit({") {
		t.Fatal("the call was turned into an object")
	}
}

// TestTheAppsOwnGateIsAskedFirst. sendMessageEdit opens by rejecting on
// canEditText/canEditCaption with the text "Cannot edit message", which names
// nothing. Asking first is what lets ErrNotEditable carry the window.
func TestTheAppsOwnGateIsAskedFirst(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "allowed", why: "NOT_EDITABLE"}
	_, err := editor(p).Text(context.Background(), msgID, "new", "t")
	if !errors.Is(err, ErrNotEditable) {
		t.Fatalf("got %v, want ErrNotEditable", err)
	}
	if !strings.Contains(err.Error(), "1200s") {
		t.Fatalf("the refusal does not carry the measured window: %v", err)
	}
	// THE NEEDLE IS THE BRANCH, not the predicate. An earlier version of this
	// test asserted only that "Cap.canEditText(msg)" appeared in the script,
	// and a negative control that changed `if (!allowed)` to `if (false)`
	// PASSED: the predicate was still computed, still present in the text, and
	// no longer controlled anything. Consulting a gate and ignoring it is the
	// exact defect this test is for, so the assertion has to name the return it
	// guards.
	if !strings.Contains(p.lastScript, "if (!allowed) { park(") {
		t.Fatal("the gate is computed but does not guard a return")
	}
	if !strings.Contains(p.lastScript, "Cap.canEditText(msg)") {
		t.Fatal("the script does not consult the app's own gate")
	}
	// And it is asked BEFORE the call, not after. The needle is the CALL —
	// "A.sendMessageEdit(" — and not the bare name: the script's own comment
	// mentions sendMessageEdit above the gate, so searching for the name finds
	// the prose and reports the order backwards. That is the guard-matches-prose
	// trap in ARMADILHAS.md, and it caught this test on its first run.
	if strings.Index(p.lastScript, "Cap.canEditText(") > strings.Index(p.lastScript, "A.sendMessageEdit(") {
		t.Fatal("the gate is consulted after the call, which makes it decoration")
	}
}

// TestSomebodyElsesMessageIsItsOwnAnswer.
func TestSomebodyElsesMessageIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_OURS"}
	if _, err := editor(p).Text(context.Background(), msgID, "new", "t"); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("got %v, want ErrNotOurs", err)
	}
	if !strings.Contains(p.lastScript, "!msg.id.fromMe") {
		t.Fatal("the script does not check whose message it is")
	}
}

// TestABodyThatDidNotMoveIsAFailure is the postcondition. A caller who believes
// an edit landed believes readers see text they do not.
func TestABodyThatDidNotMoveIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 5, applied: false, recorded: true}
	_, err := editor(p).Text(context.Background(), msgID, "new", "t")
	if !errors.Is(err, ErrUnchanged) {
		t.Fatalf("got %v, want ErrUnchanged", err)
	}
	if !strings.Contains(err.Error(), "fromLen=5 toLen=5") {
		t.Fatalf("the error must carry the measurement: %v", err)
	}
}

// TestRecordedIsCheckedByValueNotPresence. latestEditMsgKey is DEFINED on
// messages that were never edited — the probe measured that — so a script
// testing for the FIELD would report every message as edited.
func TestRecordedIsCheckedByValueNotPresence(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 9, applied: true, recorded: false}
	if _, err := editor(p).Text(context.Background(), msgID, "new text", "t"); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if !strings.Contains(p.lastScript, "msg.latestEditMsgKey != null") {
		t.Fatal("the script tests for the field rather than its value")
	}
	if strings.Contains(p.lastScript, "latestEditMsgKey !== undefined") {
		t.Fatal("presence is used as the signal; the probe measured it as always present")
	}
}

// TestAppliedWithoutRecordedIsReportedNotHidden. It is a real state, and a
// caller deciding whether readers will see an "edited" mark needs it.
func TestAppliedWithoutRecordedIsReportedNotHidden(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 9, applied: true, recorded: false}
	got, err := editor(p).Text(context.Background(), msgID, "new text", "t")
	if err != nil {
		t.Fatalf("an applied-but-unrecorded edit was turned into an error: %v", err)
	}
	if got.Recorded {
		t.Fatal("the unrecorded state was reported as recorded")
	}
}

// TestNoTextIsReported. A message body is content.
func TestNoTextIsReported(t *testing.T) {
	s := Result{FromLen: 5, ToLen: 9, Recorded: true}.String()
	for _, bad := range []string{"hello", "body", "text="} {
		if strings.Contains(s, bad) {
			t.Fatalf("the rendering leaks content (%q): %s", bad, s)
		}
	}
	if !strings.Contains(s, "fromLen=5") || !strings.Contains(s, "toLen=9") {
		t.Fatalf("the lengths are missing: %s", s)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	for _, tc := range []struct {
		name, id, text string
		want           error
	}{
		{"no id", "  ", "x", ErrNoMessage},
		{"empty text", msgID, "", ErrEmptyText},
		{"too long", msgID, strings.Repeat("x", MaxTextBytes+1), ErrTooLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pageDouble{ok: true}
			if _, err := editor(p).Text(context.Background(), tc.id, tc.text, "t"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if p.kicks != 0 {
				t.Fatalf("the page was asked %d time(s) for a call that could not work", p.kicks)
			}
		})
	}
}

// TestEmptyTextIsNotAQuietDeletion. Emptying a message is revoke's job, and
// letting edit do it by accident is the kind of destructive surprise this
// module refuses elsewhere too.
func TestEmptyTextIsNotAQuietDeletion(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 0, applied: true}
	_, err := editor(p).Text(context.Background(), msgID, "", "t")
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("got %v, want ErrEmptyText", err)
	}
	if !strings.Contains(err.Error(), "revoke") {
		t.Fatalf("the refusal does not point at the capability that does this: %v", err)
	}
}

func TestAMissingMessageIsItsOwnAnswer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "NOT_LOADED"}
	if _, err := editor(p).Text(context.Background(), msgID, "new", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
}

func TestCancelledContextEditsNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 5, toLen: 9, applied: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := editor(p).Text(ctx, msgID, "new text", "t"); err == nil {
		t.Fatal("a cancelled context edited a message")
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
	_, err := New(engine.NewRunner(), stuck).Text(context.Background(), msgID, "new", "t")
	if !errors.Is(err, ErrEdit) || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("got %v, want a settle timeout", err)
	}
}

package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type subjDouble struct {
	ok               bool
	stage, why       string
	fromLen, toLen   int
	field            string
	already          bool
	settleAfterReads int

	reads      int
	kicks      int
	lastScript string
}

func (p *subjDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		p.reads++
		switch {
		case !p.ok:
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
		case p.already:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"ALREADY","already":true,"fromLen":%d,"toLen":%d,"field":%q}`,
				p.fromLen, p.fromLen, p.field)
		case p.reads <= p.settleAfterReads:
			*out = fmt.Sprintf(`{"stage":"settling","ok":false,"why":"","fromLen":%d}`, p.fromLen)
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":false,"fromLen":%d,"toLen":%d,"field":%q}`,
				p.fromLen, p.toLen, p.field)
		}
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func renamer(p *subjDouble) *Manager { return New(engine.NewRunner(), p.eval) }

// TestTheSubjectIsPassedExplicitly. The app's own signature is
// setGroupSubject(chat, subject = "") — the default CLEARS the name, and a call
// that omits the argument would rename a group to nothing.
func TestTheSubjectIsPassedExplicitly(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, toLen: 9, field: "formattedTitle"}
	if _, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t"); err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.setGroupSubject(chat, want)") {
		t.Fatal("the call does not pass the subject explicitly")
	}
	if strings.Contains(p.lastScript, "A.setGroupSubject(chat)") {
		t.Fatal("the call relies on the app's default, which is the empty string")
	}
}

// TestAnEmptySubjectIsRefused, and the refusal explains why rather than just
// saying no.
func TestAnEmptySubjectIsRefused(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true}
	for _, s := range []string{"", "   ", "\t\n"} {
		_, err := renamer(p).SetSubject(context.Background(), testGroupJID, s, "t")
		if !errors.Is(err, ErrEmptySubject) {
			t.Fatalf("subject %q: got %v, want ErrEmptySubject", s, err)
		}
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) to clear a group's name", p.kicks)
	}
}

// TestTheFieldIsReportedNotAssumed. Which field holds a group's subject was
// never measured on this build before, and the live run answered
// formattedTitle. Reporting it is what lets the next reader check the claim
// instead of trusting it.
func TestTheFieldIsReportedNotAssumed(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, toLen: 9, field: "formattedTitle"}
	got, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t")
	if err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	if got.Field != "formattedTitle" {
		t.Fatalf("the field was not carried through: %s", got)
	}
	// All three candidates have to be tried, in both scripts, or a build that
	// moves the subject elsewhere would report a rename that did not happen.
	for _, f := range []string{"'subject'", "'name'", "'formattedTitle'"} {
		if !strings.Contains(p.lastScript, f) {
			t.Fatalf("the kick script does not consider %s", f)
		}
		if !strings.Contains(subjectResultScript, f) {
			t.Fatalf("the result script does not consider %s", f)
		}
	}
}

// TestASubjectThatNeverSettlesIsUnchangedNotATimeout.
func TestASubjectThatNeverSettlesIsUnchangedNotATimeout(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, settleAfterReads: 1 << 30}
	_, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t")
	if !errors.Is(err, ErrSubjectUnchanged) {
		t.Fatalf("got %v, want ErrSubjectUnchanged", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a subject that did not change is reported as a hung page: %v", err)
	}
}

// TestTheAwaitIsNotTheCompletion — carried across from H61 rather than
// rediscovered.
func TestTheAwaitIsNotTheCompletionForRename(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, toLen: 9, field: "name", settleAfterReads: 2}
	if _, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t"); err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the first answer was accepted instead of waited on", p.reads)
	}
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
}

// TestNoAdminGuess. Renaming is governed by a per-group setting that may allow
// every member, so refusing on an admin guess would deny an act the group
// permits. The page's own refusal is passed through instead.
func TestNoAdminGuess(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, toLen: 9, field: "name"}
	if _, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t"); err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	if strings.Contains(p.lastScript, "iAmAdmin(") {
		t.Fatal("the rename guesses at admin rights the group setting may not require")
	}
}

func TestARedundantRenameIsANoOp(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, already: true, fromLen: 9, field: "name"}
	got, err := renamer(p).SetSubject(context.Background(), testGroupJID, "new name", "t")
	if err != nil {
		t.Fatalf("a redundant rename produced an error: %v", err)
	}
	if !got.AlreadyInState || got.Changed() {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

func TestRenameRefusalsCostNoPageCall(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true}
	if _, err := renamer(p).SetSubject(context.Background(), "1@c.us", "x", "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	long := strings.Repeat("x", MaxSubjectBytes+1)
	if _, err := renamer(p).SetSubject(context.Background(), testGroupJID, long, "t"); !errors.Is(err, ErrSubjectTooLong) {
		t.Fatalf("got %v, want ErrSubjectTooLong", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

func TestTheRenameRenderingCarriesNoName(t *testing.T) {
	s := Rename{FromLen: 49, ToLen: 72, Field: "formattedTitle"}.String()
	if !strings.Contains(s, "fromLen=49") || !strings.Contains(s, "toLen=72") {
		t.Fatalf("the lengths are missing: %s", s)
	}
	if strings.Contains(s, "subject=") || strings.Contains(s, "name=") {
		t.Fatalf("the rendering carries a group's name: %s", s)
	}
}

func TestCancelledContextRenamesNothing(t *testing.T) {
	compressPartClock(t)
	p := &subjDouble{ok: true, fromLen: 5, toLen: 9, field: "name"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renamer(p).SetSubject(ctx, testGroupJID, "new name", "t"); err == nil {
		t.Fatal("a cancelled context renamed a group")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

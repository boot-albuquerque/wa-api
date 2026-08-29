package profile

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
	ok               bool
	stage, why       string
	fromLen, toLen   int
	already          bool
	settleAfterReads int

	reads      int
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
		p.reads++
		switch {
		case !p.ok:
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
		case p.already:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"ALREADY","already":true,"fromLen":%d,"toLen":%d}`,
				p.fromLen, p.fromLen)
		case p.reads <= p.settleAfterReads:
			*out = fmt.Sprintf(`{"stage":"settling","ok":false,"why":"","fromLen":%d}`, p.fromLen)
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":false,"fromLen":%d,"toLen":%d}`,
				p.fromLen, p.toLen)
		}
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
	ob, ot := profileBudget, profileTick
	profileBudget, profileTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { profileBudget, profileTick = ob, ot })
}

// TestTheBuildsOwnGateIsAskedFirst, and the refusal explains itself. On the lab
// account canSetMyPushname() is FALSE — measured — so this is the path that
// account actually takes, not a hypothetical.
func TestTheBuildsOwnGateIsAskedFirst(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "allowed", why: "CANNOT_SET"}
	_, err := editor(p).SetDisplayName(context.Background(), "novo nome", "t")
	if !errors.Is(err, ErrCannotSetDisplayName) {
		t.Fatalf("got %v, want ErrCannotSetDisplayName", err)
	}
	if !strings.Contains(err.Error(), "business") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
	// The needle is the BRANCH, not the predicate.
	if !strings.Contains(p.lastScript, "if (Conn.canSetMyPushname && !Conn.canSetMyPushname()) {") {
		t.Fatal("the gate is computed but does not guard a return")
	}
	if strings.Index(p.lastScript, "canSetMyPushname()") > strings.Index(p.lastScript, "A.setPushname(") {
		t.Fatal("the gate is consulted after the call")
	}
}

// TestTheUICallbackIsOmitted. setPushname(name, onDone) — the app passes a
// function that refocuses an edit button, and a headless session has none.
func TestTheUICallbackIsOmitted(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 3, toLen: 9}
	if _, err := editor(p).SetDisplayName(context.Background(), "novo nome", "t"); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.setPushname(want)") {
		t.Fatal("the call does not use the measured single-argument form")
	}
}

// TestTheAwaitIsNotTheCompletion — carried from H61.
func TestTheAwaitIsNotTheCompletion(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 3, toLen: 9, settleAfterReads: 2}
	got, err := editor(p).SetDisplayName(context.Background(), "novo nome", "t")
	if err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if got.ToLen != 9 {
		t.Fatalf("the settled value was not picked up: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the first answer was accepted instead of waited on", p.reads)
	}
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
}

func TestANameThatNeverSettlesIsUnchangedNotATimeout(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 3, settleAfterReads: 1 << 30}
	_, err := editor(p).SetDisplayName(context.Background(), "novo nome", "t")
	if !errors.Is(err, ErrDisplayNameUnchanged) {
		t.Fatalf("got %v, want ErrDisplayNameUnchanged", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a name that did not change is reported as a hung page: %v", err)
	}
}

func TestARedundantRenameIsANoOp(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, already: true, fromLen: 9}
	got, err := editor(p).SetDisplayName(context.Background(), "novo nome", "t")
	if err != nil {
		t.Fatalf("a redundant rename produced an error: %v", err)
	}
	if !got.NoOp {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := editor(p).SetDisplayName(context.Background(), "   ", "t"); !errors.Is(err, ErrEmptyDisplayName) {
		t.Fatalf("got %v, want ErrEmptyDisplayName", err)
	}
	long := strings.Repeat("x", MaxDisplayNameBytes+1)
	if _, err := editor(p).SetDisplayName(context.Background(), long, "t"); !errors.Is(err, ErrDisplayNameTooLong) {
		t.Fatalf("got %v, want ErrDisplayNameTooLong", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

// TestNoNameIsReported. A display name identifies a person.
func TestNoNameIsReported(t *testing.T) {
	s := NameChange{FromLen: 3, ToLen: 9}.String()
	if !strings.Contains(s, "fromLen=3") || !strings.Contains(s, "toLen=9") {
		t.Fatalf("the lengths are missing: %s", s)
	}
	if strings.Contains(s, "name=") && !strings.Contains(s, "Len=") {
		t.Fatalf("the rendering carries a name: %s", s)
	}
}

func TestCancelledContextRenamesNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, fromLen: 3, toLen: 9}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := editor(p).SetDisplayName(ctx, "novo nome", "t"); err == nil {
		t.Fatal("a cancelled context renamed the account")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

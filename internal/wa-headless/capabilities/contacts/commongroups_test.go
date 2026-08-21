package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type commonDouble struct {
	ok         bool
	stage, why string
	jids       []string

	kicks      int
	lastScript string
}

func (p *commonDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "find"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		quoted := make([]string, 0, len(p.jids))
		for _, j := range p.jids {
			quoted = append(quoted, fmt.Sprintf("%q", j))
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","jids":[%s]}`, strings.Join(quoted, ","))
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func commonLister(p *commonDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func compressCommonClock(t *testing.T) {
	t.Helper()
	ob, ot := commonBudget, commonTick
	commonBudget, commonTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { commonBudget, commonTick = ob, ot })
}

const somePeer = "15550001111@c.us"

// TestNullMeansSelfNotNone is the distinction the whole capability turns on.
// findCommonGroups returns Promise.resolve(null) for this account's own contact
// and a collection otherwise; collapsing the two would answer a refusal with an
// empty list, and a caller would read "you share no groups with yourself" as a
// fact rather than a category error.
func TestNullMeansSelfNotNone(t *testing.T) {
	compressCommonClock(t)
	self := &commonDouble{ok: false, stage: "find", why: "IS_SELF"}
	if _, err := commonLister(self).CommonGroupsWith(context.Background(), somePeer, "t"); !errors.Is(err, ErrIsSelf) {
		t.Fatalf("got %v, want ErrIsSelf", err)
	}
	if !strings.Contains(self.lastScript, "if (found === null || found === undefined) {") {
		t.Fatal("the script does not treat null as its own answer")
	}

	// And an EMPTY collection is a successful empty answer.
	none := &commonDouble{ok: true, jids: nil}
	got, err := commonLister(none).CommonGroupsWith(context.Background(), somePeer, "t")
	if err != nil {
		t.Fatalf("no groups in common produced an error: %v", err)
	}
	if len(got.JIDs) != 0 {
		t.Fatalf("expected an empty answer: %s", got)
	}
}

// TestTheContactModelIsPassed. findCommonGroups unproxies its argument, which is
// this build's way of asking for a model — the same lesson as five earlier
// "reading '<field>' of undefined" failures.
func TestTheContactModelIsPassed(t *testing.T) {
	compressCommonClock(t)
	p := &commonDouble{ok: true, jids: []string{"120363000000000000@g.us"}}
	if _, err := commonLister(p).CommonGroupsWith(context.Background(), somePeer, "t"); err != nil {
		t.Fatalf("CommonGroupsWith: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.findCommonGroups(contact)") {
		t.Fatal("the call does not pass the contact model")
	}
	if strings.Contains(p.lastScript, "findCommonGroups(r.wid)") {
		t.Fatal("a wid is passed where a model is wanted")
	}
}

// TestOnlyACountIsRendered. A list of somebody's groups is a profile of that
// person, so the rendering carries the number and the caller carries the list.
func TestOnlyACountIsRendered(t *testing.T) {
	s := CommonGroups{JIDs: []string{"120363000000000000@g.us", "120363000000000001@g.us"}}.String()
	if !strings.Contains(s, "count=2") {
		t.Fatalf("the count is missing: %s", s)
	}
	if strings.Contains(s, "@g.us") {
		t.Fatalf("the rendering lists the groups: %s", s)
	}
}

func TestCommonGroupsRefusalsCostNoPageCall(t *testing.T) {
	compressCommonClock(t)
	p := &commonDouble{ok: true}
	if _, err := commonLister(p).CommonGroupsWith(context.Background(), "   ", "t"); !errors.Is(err, ErrNoContact) {
		t.Fatalf("got %v, want ErrNoContact", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

func TestAnUnknownContactIsItsOwnAnswer(t *testing.T) {
	compressCommonClock(t)
	for _, why := range []string{"NO_CONTACT", "NOT_ON_WHATSAPP", "WID_NULL"} {
		p := &commonDouble{ok: false, stage: "contact", why: why}
		if _, err := commonLister(p).CommonGroupsWith(context.Background(), somePeer, "t"); !errors.Is(err, ErrNoContact) {
			t.Fatalf("%s: got %v, want ErrNoContact", why, err)
		}
	}
}

func TestCancelledContextAsksNothing(t *testing.T) {
	compressCommonClock(t)
	p := &commonDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := commonLister(p).CommonGroupsWith(ctx, somePeer, "t"); err == nil {
		t.Fatal("a cancelled context still queried the page")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAHungPageIsATimeoutForCommonGroups(t *testing.T) {
	compressCommonClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	if _, err := New(engine.NewRunner(), stuck).CommonGroupsWith(context.Background(), somePeer, "t"); !errors.Is(err, ErrCommonGroups) {
		t.Fatalf("got %v, want ErrCommonGroups", err)
	}
}

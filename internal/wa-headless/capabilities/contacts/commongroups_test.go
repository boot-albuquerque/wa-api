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

// --- about -------------------------------------------------------------

type aboutDouble struct {
	ok         bool
	stage, why string
	text       string
	fetched    bool

	kicks      int
	lastScript string
}

func (p *aboutDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "read"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","text":%q,"fetched":%t}`,
			p.text, p.fetched)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func aboutLister(p *aboutDouble) *Lister { return New(engine.NewRunner(), p.eval) }

// TestDisabledIsNotEmpty is the distinction the gate exists for. A build with
// text status receiving off would otherwise be indistinguishable from a contact
// who wrote nothing, and those are different answers.
func TestDisabledIsNotEmpty(t *testing.T) {
	compressCommonClock(t)
	off := &aboutDouble{ok: false, stage: "gate", why: "DISABLED"}
	if _, err := aboutLister(off).AboutOf(context.Background(), somePeer, "t"); !errors.Is(err, ErrAboutDisabled) {
		t.Fatalf("got %v, want ErrAboutDisabled", err)
	}
	// The needle is the BRANCH, not the predicate.
	if !strings.Contains(off.lastScript, "if (G.receiveTextStatusEnabled && !G.receiveTextStatusEnabled()) {") {
		t.Fatal("the gate is computed but does not guard a return")
	}

	empty := &aboutDouble{ok: true, text: ""}
	got, err := aboutLister(empty).AboutOf(context.Background(), somePeer, "t")
	if err != nil {
		t.Fatalf("an empty about produced an error: %v", err)
	}
	if got.Text != "" {
		t.Fatalf("expected an empty about: %s", got)
	}
}

// TestTheWidIsPassedNotTheModel. getTextStatus takes a wid; its neighbour
// findCommonGroups takes a model. There is no rule — only reading.
func TestTheWidIsPassedNotTheModel(t *testing.T) {
	compressCommonClock(t)
	p := &aboutDouble{ok: true, text: "hello"}
	if _, err := aboutLister(p).AboutOf(context.Background(), somePeer, "t"); err != nil {
		t.Fatalf("AboutOf: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.getTextStatus(r.wid)") {
		t.Fatal("the call does not pass the wid")
	}
	if strings.Contains(p.lastScript, "getTextStatus(contact)") {
		t.Fatal("a model is passed where a wid is wanted")
	}
}

// TestTheAboutTextIsNeverRendered. It is something a person wrote about
// themselves.
func TestTheAboutTextIsNeverRendered(t *testing.T) {
	s := About{Text: "disponível para conversar", Fetched: true}.String()
	if strings.Contains(s, "disponível") {
		t.Fatalf("the rendering carries the text: %s", s)
	}
	// The length is in RUNES, because an about full of accents is not longer
	// than one without them — Go's len would say otherwise.
	if !strings.Contains(s, "len=25") {
		t.Fatalf("the rune length is wrong or missing: %s", s)
	}
}

func TestAboutRefusalsCostNoPageCall(t *testing.T) {
	compressCommonClock(t)
	p := &aboutDouble{ok: true}
	if _, err := aboutLister(p).AboutOf(context.Background(), "  ", "t"); !errors.Is(err, ErrNoContact) {
		t.Fatalf("got %v, want ErrNoContact", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
	for _, why := range []string{"NOT_ON_WHATSAPP", "WID_NULL"} {
		q := &aboutDouble{ok: false, stage: "resolve", why: why}
		if _, err := aboutLister(q).AboutOf(context.Background(), somePeer, "t"); !errors.Is(err, ErrNoContact) {
			t.Fatalf("%s: got %v, want ErrNoContact", why, err)
		}
	}
}

func TestAboutTriesEveryKnownFieldName(t *testing.T) {
	for _, f := range []string{"'status'", "'text'", "'textStatus'"} {
		if !strings.Contains(aboutScript(somePeer), f) {
			t.Fatalf("the script does not try the %s field", f)
		}
	}
}

func TestAHungPageIsATimeoutForAbout(t *testing.T) {
	compressCommonClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	if _, err := New(engine.NewRunner(), stuck).AboutOf(context.Background(), somePeer, "t"); !errors.Is(err, ErrAbout) {
		t.Fatalf("got %v, want ErrAbout", err)
	}
}

func TestCancelledContextReadsNoAbout(t *testing.T) {
	compressCommonClock(t)
	p := &aboutDouble{ok: true, text: "x"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := aboutLister(p).AboutOf(ctx, somePeer, "t"); err == nil {
		t.Fatal("a cancelled context still read an about")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

// --- labels ------------------------------------------------------------

func labelEval(answer string) func(context.Context, string, *string) error {
	return func(ctx context.Context, _ string, out *string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		*out = answer
		return nil
	}
}

// TestAnEmptyLabelSetIsAnAnswer. A personal account has no labels, and this
// package cannot tell that apart from a business account that made none — so it
// reports what it sees rather than guessing which.
func TestAnEmptyLabelSetIsAnAnswer(t *testing.T) {
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","rows":[]}`))
	got, err := l.ListLabels(context.Background(), "t")
	if err != nil {
		t.Fatalf("an empty label set produced an error: %v", err)
	}
	if len(got.All) != 0 {
		t.Fatalf("expected an empty set: %s", got)
	}
}

// TestALabelsNameIsNeverRendered. It is something the account's owner wrote.
func TestALabelsNameIsNeverRendered(t *testing.T) {
	s := Label{ID: "1", Name: "Cliente novo", ColorIndex: 2, Count: 5}.String()
	if strings.Contains(s, "Cliente") {
		t.Fatalf("the rendering carries the name: %s", s)
	}
	if !strings.Contains(s, "id=1") || !strings.Contains(s, "nameLen=12") {
		t.Fatalf("the id or the rune length is wrong: %s", s)
	}
	if !strings.Contains(s, "count=5") {
		t.Fatalf("the count is missing: %s", s)
	}
}

// TestTheLabelSetRenderingIsACount.
func TestTheLabelSetRenderingIsACount(t *testing.T) {
	s := Labels{All: []Label{{ID: "1", Name: "x"}, {ID: "2", Name: "y"}}}.String()
	if !strings.Contains(s, "count=2") {
		t.Fatalf("the count is missing: %s", s)
	}
	if strings.Contains(s, "id=") {
		t.Fatalf("the set rendering lists its members: %s", s)
	}
}

// TestAnUnlabelledChatReturnsAnEmptySliceNotNil. "This chat is not labelled" is
// an answer, and a nil slice invites a caller to treat it as a failure.
func TestAnUnlabelledChatReturnsAnEmptySliceNotNil(t *testing.T) {
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","ids":null}`))
	ids, err := l.LabelsOfChat(context.Background(), somePeer, "t")
	if err != nil {
		t.Fatalf("LabelsOfChat: %v", err)
	}
	if ids == nil {
		t.Fatal("an unlabelled chat returned nil")
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids, got %d", len(ids))
	}
}

// TestChatLabelIdsAreStrings. A build that stored numbers there would otherwise
// produce a silently different type.
func TestChatLabelIdsAreStrings(t *testing.T) {
	if !strings.Contains(chatLabelsScript(somePeer), "ls.map(String)") {
		t.Fatal("the script does not normalise label ids to strings")
	}
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","ids":["1","3"]}`))
	ids, err := l.LabelsOfChat(context.Background(), somePeer, "t")
	if err != nil {
		t.Fatalf("LabelsOfChat: %v", err)
	}
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "3" {
		t.Fatalf("the ids did not survive: %v", ids)
	}
}

func TestLabelFailuresAreNamed(t *testing.T) {
	l := New(engine.NewRunner(), labelEval(`{"ok":false,"why":"boom","rows":[]}`))
	if _, err := l.ListLabels(context.Background(), "t"); !errors.Is(err, ErrLabels) {
		t.Fatalf("got %v, want ErrLabels", err)
	}
	nc := New(engine.NewRunner(), labelEval(`{"ok":false,"why":"NO_CHAT","ids":[]}`))
	if _, err := nc.LabelsOfChat(context.Background(), somePeer, "t"); !errors.Is(err, ErrNoSuchChatForLabels) {
		t.Fatalf("got %v, want ErrNoSuchChatForLabels", err)
	}
	called := false
	empty := New(engine.NewRunner(), func(context.Context, string, *string) error {
		called = true
		return nil
	})
	if _, err := empty.LabelsOfChat(context.Background(), "  ", "t"); !errors.Is(err, ErrNoSuchChatForLabels) {
		t.Fatalf("got %v, want ErrNoSuchChatForLabels", err)
	}
	if called {
		t.Fatal("the page was asked about an empty jid")
	}
	bad := New(engine.NewRunner(), labelEval(`not json`))
	if _, err := bad.ListLabels(context.Background(), "t"); err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
	if _, err := bad.LabelsOfChat(context.Background(), somePeer, "t"); err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
}

func TestCancelledContextReadsNoLabels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","rows":[]}`))
	if _, err := l.ListLabels(ctx, "t"); err == nil {
		t.Fatal("a cancelled context still read the labels")
	}
	if _, err := l.LabelsOfChat(ctx, somePeer, "t"); err == nil {
		t.Fatal("a cancelled context still read a chat's labels")
	}
}

// --- label write -------------------------------------------------------

type labelWriteDouble struct {
	ok            bool
	stage, why    string
	before, after int
	already       bool
	settleReads   int

	reads      int
	kicks      int
	lastScript string
}

func (p *labelWriteDouble) eval(ctx context.Context, expr string, out *string) error {
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
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"ALREADY","already":true,"before":%d,"after":%d}`,
				p.before, p.before)
		case p.reads <= p.settleReads:
			*out = fmt.Sprintf(`{"stage":"settling","ok":false,"why":"","before":%d,"after":%d}`,
				p.before, p.before)
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":false,"before":%d,"after":%d}`,
				p.before, p.after)
		}
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func labelWriter(p *labelWriteDouble) *Lister { return New(engine.NewRunner(), p.eval) }

// TestTheMeasuredLabelCallShapeIsUsed. [{id, type}] and [chatModel] came from
// the argument instrument with array hints — a plain recorder answered only
// ".forEach", because it answers the iteration itself and the callback never
// runs.
func TestTheMeasuredLabelCallShapeIsUsed(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 0, after: 1}
	if _, err := labelWriter(p).AddLabel(context.Background(), somePeer, "1", "t"); err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if !strings.Contains(p.lastScript, "[{ id: want, type: add ? 'add' : 'remove' }]") {
		t.Fatal("the mutation is not the measured {id, type} shape")
	}
	if !strings.Contains(p.lastScript, "B.editLabelAssociation(mutations, chats)") {
		t.Fatal("the bridge is not called with two lists")
	}
	if !strings.Contains(p.lastScript, "const chats = [chat];") {
		t.Fatal("the chat is not passed as a MODEL inside a list")
	}
}

// TestTheLocalMirrorIsCalledToo. The app calls addOrRemoveLabelsMD right after
// the bridge, and without it chat.labels does not move — which is also the only
// postcondition available, so skipping it would make every apply look failed.
func TestTheLocalMirrorIsCalledToo(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 0, after: 1}
	if _, err := labelWriter(p).AddLabel(context.Background(), somePeer, "1", "t"); err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if !strings.Contains(p.lastScript, "LC.addOrRemoveLabelsMD(mutations, chats)") {
		t.Fatal("the local mirror is not updated, so chat.labels would never move")
	}
}

// TestAnUnknownLabelIsRefusedBeforeApplying. Applying a label that does not
// exist would attach nothing and read as a change that did not take.
func TestAnUnknownLabelIsRefusedBeforeApplying(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: false, stage: "find", why: "NO_LABEL"}
	if _, err := labelWriter(p).AddLabel(context.Background(), somePeer, "999", "t"); !errors.Is(err, ErrNoLabelGiven) {
		t.Fatalf("got %v, want ErrNoLabelGiven", err)
	}
	if !strings.Contains(p.lastScript, "if (known.indexOf(want) === -1) { park(") {
		t.Fatal("the known-label check is computed but does not guard a return")
	}
}

// TestTheAwaitIsNotTheCompletionForLabels — carried from H61 rather than
// rediscovered.
func TestTheAwaitIsNotTheCompletionForLabels(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 0, after: 1, settleReads: 2}
	got, err := labelWriter(p).AddLabel(context.Background(), somePeer, "1", "t")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if got.After != 1 {
		t.Fatalf("the settled value was not picked up: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("only %d read(s): the first answer was accepted instead of waited on", p.reads)
	}
	if !strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the apply branch does not hand the settling decision to Go")
	}
}

func TestALabelThatNeverAttachesIsUnchangedNotATimeout(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 0, after: 1, settleReads: 1 << 30}
	_, err := labelWriter(p).AddLabel(context.Background(), somePeer, "1", "t")
	if !errors.Is(err, ErrLabelUnchanged) {
		t.Fatalf("got %v, want ErrLabelUnchanged", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a label that did not attach is reported as a hung page: %v", err)
	}
}

func TestARedundantLabelApplyIsANoOp(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, already: true, before: 1}
	got, err := labelWriter(p).AddLabel(context.Background(), somePeer, "1", "t")
	if err != nil {
		t.Fatalf("a redundant apply produced an error: %v", err)
	}
	if !got.NoOp {
		t.Fatalf("not reported as a no-op: %s", got)
	}
}

func TestLabelWriteRefusalsCostNoPageCall(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true}
	if _, err := labelWriter(p).AddLabel(context.Background(), "  ", "1", "t"); !errors.Is(err, ErrNoSuchChatForLabels) {
		t.Fatalf("got %v, want ErrNoSuchChatForLabels", err)
	}
	if _, err := labelWriter(p).RemoveLabel(context.Background(), somePeer, "  ", "t"); !errors.Is(err, ErrNoLabelGiven) {
		t.Fatalf("got %v, want ErrNoLabelGiven", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

func TestRemoveSendsTheOtherVerb(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 1, after: 0}
	if _, err := labelWriter(p).RemoveLabel(context.Background(), somePeer, "1", "t"); err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}
	if !strings.Contains(p.lastScript, "const add = false;") {
		t.Fatal("remove does not flip the verb")
	}
}

func TestCancelledContextWritesNoLabel(t *testing.T) {
	compressCommonClock(t)
	p := &labelWriteDouble{ok: true, before: 0, after: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := labelWriter(p).AddLabel(ctx, somePeer, "1", "t"); err == nil {
		t.Fatal("a cancelled context labelled a chat")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

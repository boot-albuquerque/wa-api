package group

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
	ok           bool
	why          string
	stage        string
	jid          string
	subject      string
	participants int
	created      bool
	missing      int

	// awaitingReads makes the double answer 'awaiting_chat' for the first N
	// polls, which is what the page does between createGroup returning and the
	// chat appearing in the collection.
	awaitingReads int

	reads      int
	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		p.reads++
		// THE CREATED-BUT-NOT-YET-VISIBLE TURN. The page parks 'awaiting_chat'
		// after createGroup returns and stops; each Go poll spends one turn
		// looking for the chat. The double answers that stage for the first
		// awaitingReads polls so the loop is actually exercised — a double that
		// answered 'done' immediately would leave the whole wait untested,
		// which is how the sleep survived in the page for as long as it did.
		if p.awaitingReads > 0 && p.reads <= p.awaitingReads {
			*out = `{"stage":"awaiting_chat","ok":false,"why":""}`
			return nil
		}
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "create"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		miss := make([]string, 0, p.missing)
		for i := 0; i < p.missing; i++ {
			miss = append(miss, `"redacted"`)
		}
		*out = fmt.Sprintf(
			`{"stage":"done","ok":true,"why":"","jid":%q,"subject":%q,`+
				`"participants":%d,"created":%t,"missing":[%s]}`,
			p.jid, p.subject, p.participants, p.created, strings.Join(miss, ","))
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func manager(p *pageDouble) *Manager { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := createBudget, createTick
	createBudget, createTick = 200*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { createBudget, createTick = ob, ot })
}

const (
	labSubject = "wa-headless-lab — nao usar"
	labGroupID = "120363000000000000@g.us"
	peer       = "5541992421234@c.us"
)

// TestAMissingParticipantIsAFailure is the postcondition, and the only outcome
// a caller could not see for themselves: a group that came back without
// somebody who was asked for would be used believing the wrong people are in
// it.
func TestAMissingParticipantIsAFailure(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 1, created: true, missing: 1}
	_, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group")
	if !errors.Is(err, ErrParticipantMissing) {
		t.Fatalf("got %v, want ErrParticipantMissing", err)
	}
	if !strings.Contains(err.Error(), "1 of 1") {
		t.Fatalf("the error must say how many are missing out of how many were asked: %v", err)
	}
	// And it must NOT say WHICH: that would be an identity in a log.
	if strings.Contains(err.Error(), peer) {
		t.Fatalf("the error named a participant: %v", err)
	}
}

// TestFindingAnExistingGroupIsNotCreatingOne. Without this distinction a caller
// cannot know whether the call just changed someone's account — and every run
// of a test needing a group would leave another one behind.
func TestFindingAnExistingGroupIsNotCreatingOne(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: false}
	got, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got.Created {
		t.Fatal("an existing group was reported as newly created")
	}
	if got.Participants != 2 || got.Subject != labSubject {
		t.Fatalf("the found group came back wrong: %s", got)
	}
	if !strings.Contains(got.String(), "created=false") {
		t.Fatalf("the rendering hides whether an account was changed: %s", got)
	}
}

func TestCreatingReportsItself(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: true}
	got, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !got.Created {
		t.Fatal("a newly created group did not say so")
	}
}

// TestAnUnnamedGroupIsRefusedBeforeThePage: an unnamed group on a real account
// is indistinguishable from junk to whoever finds it, and it cannot be found
// again by subject — so idempotency would break too.
func TestAnUnnamedGroupIsRefusedBeforeThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := manager(p).Ensure(context.Background(), "   ", []string{peer}, "t/group"); !errors.Is(err, ErrNoSubject) {
		t.Fatalf("got %v, want ErrNoSubject", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to create %d unnamed group(s)", p.kicks)
	}
}

func TestAGroupWithNobodyIsRefusedBeforeThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if _, err := manager(p).Ensure(context.Background(), labSubject, nil, "t/group"); !errors.Is(err, ErrNoParticipants) {
		t.Fatalf("got %v, want ErrNoParticipants", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to create %d empty group(s)", p.kicks)
	}
}

// TestParticipantsAreResolvedBEFORECreating. A group made with an unresolvable
// member would have to be cleaned up by a human, so the order is part of the
// contract and not an implementation detail.
func TestParticipantsAreResolvedBeforeCreating(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: true}
	if _, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	resolveAt := strings.Index(p.lastScript, "queryWidExists")
	createAt := strings.Index(p.lastScript, "createGroup(args")
	if resolveAt < 0 {
		t.Fatal("participants are not resolved at all; this build addresses people " +
			"by the lid the server returns, not by the number typed (H34)")
	}
	if createAt < 0 {
		t.Fatal("the create call is not in the script")
	}
	if resolveAt > createAt {
		t.Fatal("the group is created BEFORE the participants are resolved: an " +
			"unresolvable member would leave a group for a human to clean up")
	}
}

// TestTheUILayerIsNotUsed. WAWebCreateGroupAction opens a toast and builds
// React elements; a headless driver reaching the operation through the
// interface would be driving the UI, and would break the moment the UI moves.
func TestTheUILayerIsNotUsed(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: true}
	if _, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if strings.Contains(p.lastScript, "WAWebCreateGroupAction") {
		t.Fatal("the script calls the UI action layer, which opens a toast through " +
			"WAWebToastManager; the job underneath is the operation")
	}
	if !strings.Contains(p.lastScript, "WAWebGroupCreateJob") {
		t.Fatal("the script does not call the job layer")
	}
}

func TestAPageRefusalCarriesStageAndReason(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "resolve", why: "NOT_ON_WHATSAPP"}
	_, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t/group")
	if !errors.Is(err, ErrCreate) {
		t.Fatalf("got %v, want ErrCreate", err)
	}
	if !strings.Contains(err.Error(), "resolve") || !strings.Contains(err.Error(), "NOT_ON_WHATSAPP") {
		t.Fatalf("stage or reason dropped: %v", err)
	}
}

func TestCancelledContextNeverCreatesAGroup(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager(p).Ensure(ctx, labSubject, []string{peer}, "t/group"); err == nil {
		t.Fatal("a cancelled context created a group")
	}
	if p.kicks != 0 {
		t.Fatalf("created %d group(s) for a caller that had given up", p.kicks)
	}
}

// THE WAIT FOR THE NEW CHAT IS GO'S, and this is the test that says so.
//
// The page used to loop with setTimeout while the chat appeared in the
// collection. It worked, and it put a deadline where a caller's context could
// not reach it — during a reload, a throttled tab, a hung renderer. Now the
// page parks 'awaiting_chat' and stops; each poll here spends one turn.
func TestTheChatIsWaitedForFromGo(t *testing.T) {
	compressClock(t)
	p := &pageDouble{
		ok: true, jid: labGroupID, subject: labSubject, participants: 2, created: true,
		awaitingReads: 3,
	}
	got, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got.JID != labGroupID {
		t.Fatalf("got %+v", got)
	}
	if p.reads <= p.awaitingReads {
		t.Errorf("%d reads for %d awaiting turns; Go did not poll", p.reads, p.awaitingReads)
	}
	if p.kicks != 1 {
		t.Errorf("%d kicks; the create must run once and the polling must not re-create", p.kicks)
	}
	// And the create script must not sleep. The module-wide gate says the same
	// thing; this says it where somebody editing THIS script will see it.
	for _, banned := range []string{"setTimeout(", "setInterval("} {
		if strings.Contains(p.lastScript, banned) {
			t.Errorf("the create script contains %q; invariant 6 puts the clock on this side", banned)
		}
	}
}

// A CREATE THAT NEVER APPEARS IS NOT A PAGE THAT HUNG, and the two used to
// share one message. The group exists on the server — createGroup returned a
// wid — and the collection never showed it; a caller told "the page never
// settled" would look in the wrong place.
func TestAGroupThatNeverAppearsSaysSo(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, jid: labGroupID, awaitingReads: 1 << 30}
	_, err := manager(p).Ensure(context.Background(), labSubject, []string{peer}, "t")
	if err == nil {
		t.Fatal("a chat that never appears must not succeed")
	}
	if !strings.Contains(err.Error(), "CREATED_BUT_NOT_IN_COLLECTION") {
		t.Fatalf("err = %v, want the created-but-invisible reason", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Error("the error blames the page for hanging; it answered every time")
	}
}

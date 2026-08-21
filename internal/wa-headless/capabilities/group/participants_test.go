package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// REMOVING DOES NOT WORK AGAINST THE LIVE PAGE (H58): removeParticipantsJob
// throws "Cannot read properties of undefined (reading 'toString')" with and
// without an author. These tests cover the half that IS correct — the refusals,
// the no-op cases, the counts and the argument SHAPES read from the app's own
// source — so the measured work survives and the coverage number stays honest
// about what exists. None of them claims the capability works.

type partDouble struct {
	ok            bool
	stage         string
	why           string
	before, after int

	kicks      int
	lastScript string
}

func (p *partDouble) eval(ctx context.Context, expr string, out *string) error {
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":%q,"before":%d,"after":%d}`,
			p.why, p.before, p.after)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func parts(p *partDouble) *Manager { return New(engine.NewRunner(), p.eval) }

func compressPartClock(t *testing.T) {
	t.Helper()
	ob, ot := createBudget, createTick
	createBudget, createTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { createBudget, createTick = ob, ot })
}

const testGroupJID = "120363000000000000@g.us"

// TestTheTwoSiblingsHaveDifferentShapes is the finding, and it is the kind that
// only reading produces: addParticipantsJob takes ONE OBJECT and
// removeParticipantsJob takes SEVEN POSITIONAL ARGUMENTS, side by side in the
// same module. Assuming the second matched the first would have been another
// blind correction at the layer that has cost this module five of them.
func TestTheTwoSiblingsHaveDifferentShapes(t *testing.T) {
	compressPartClock(t)

	add := &partDouble{ok: true, before: 2, after: 3}
	if _, err := parts(add).AddParticipant(context.Background(), testGroupJID, "1@c.us", "t/add"); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}
	if !strings.Contains(add.lastScript, "addParticipantsJob({") {
		t.Fatal("add is not called with a single object")
	}

	rem := &partDouble{ok: true, before: 3, after: 2}
	if _, err := parts(rem).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if strings.Contains(rem.lastScript, "removeParticipantsJob({") {
		t.Fatal("remove is called with an object; its source takes seven positional arguments")
	}
	if !strings.Contains(rem.lastScript, "removeParticipantsJob(gwid, [r.wid]") {
		t.Fatal("remove does not use the positional shape read from its source")
	}
}

// TestTheClockStaysOnTheGoSide. Invariant 6: a page that reads its own clock is
// a page whose answers cannot be reproduced from a transcript. The timestamp
// removeParticipantsJob needs is passed in.
func TestTheClockStaysOnTheGoSide(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, before: 3, after: 2}
	if _, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if strings.Contains(p.lastScript, "Date.now()") {
		t.Fatal("the script reads the page's clock")
	}
}

// TestAlreadyAMemberIsANoOp, and so is removing somebody who is not there. Both
// are the same shape as an already-archived chat (H55): nothing to do is a
// successful no-op, not a failure.
func TestNoOpsAreSuccesses(t *testing.T) {
	compressPartClock(t)
	already := &partDouble{ok: true, why: "ALREADY_MEMBER", before: 2, after: 2}
	got, err := parts(already).AddParticipant(context.Background(), testGroupJID, "1@c.us", "t/add")
	if err != nil {
		t.Fatalf("adding an existing member produced an error: %v", err)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for a no-op: %s", got)
	}

	absent := &partDouble{ok: true, why: "NOT_A_MEMBER", before: 2, after: 2}
	got2, err := parts(absent).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem")
	if err != nil {
		t.Fatalf("removing a non-member produced an error: %v", err)
	}
	if got2.Changed() {
		t.Fatalf("Changed()=true for a no-op: %s", got2)
	}
}

// TestACountThatDoesNotMoveIsAFailure is the postcondition: removing somebody
// who is still there, or adding somebody who never arrives, is what a caller
// cannot see.
func TestACountThatDoesNotMoveIsAFailure(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, before: 3, after: 3}
	_, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem")
	if !errors.Is(err, ErrMembershipUnchanged) {
		t.Fatalf("got %v, want ErrMembershipUnchanged", err)
	}
	if !strings.Contains(err.Error(), "wanted 2") {
		t.Fatalf("the error must say what was expected: %v", err)
	}
}

// TestOnlyCountsAreReported. A group's membership is a list of people, and the
// error and the rendering carry numbers rather than names.
func TestOnlyCountsAreReported(t *testing.T) {
	m := Membership{Before: 2, After: 3}
	s := m.String()
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
	if !strings.Contains(s, "before=2") || !strings.Contains(s, "after=3") {
		t.Fatalf("the counts are missing: %s", s)
	}
}

func TestANonGroupIsRefusedBeforeThePageForParticipants(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true}
	if _, err := parts(p).AddParticipant(context.Background(), "1@c.us", "2@c.us", "t/add"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if _, err := parts(p).AddParticipant(context.Background(), testGroupJID, "  ", "t/add"); !errors.Is(err, ErrNoParticipantGiven) {
		t.Fatalf("got %v, want ErrNoParticipantGiven", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for a call that could not work", p.kicks)
	}
}

func TestNotAnAdminIsItsOwnAnswerForParticipants(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: false, stage: "find", why: "NOT_ADMIN"}
	if _, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem"); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("got %v, want ErrNotAdmin", err)
	}
}

func TestCancelledContextTouchesNoMembership(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, before: 2, after: 3}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parts(p).AddParticipant(ctx, testGroupJID, "1@c.us", "t/add"); err == nil {
		t.Fatal("a cancelled context changed a group's membership")
	}
	if p.kicks != 0 {
		t.Fatalf("changed membership %d time(s) for a caller that had given up", p.kicks)
	}
}

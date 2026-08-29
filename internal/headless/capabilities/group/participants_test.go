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
	if !strings.Contains(rem.lastScript, "removeParticipantsJob(gwid, [part(r.wid)]") {
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

// TestARealChangeIsReportedUNVERIFIED is the honest contract, and it is the
// measured one: this build shows the session that made a participant change
// nothing at all — not the metadata, not a system message. A version of this
// code that claimed otherwise reported "unchanged" for three changes that had
// all reached the server, and left the lab group with one member.
func TestARealChangeIsReportedUnverified(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, before: 3}
	got, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem")
	if err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if got.Verified {
		t.Fatalf("a real change claims to be verified: %s", got)
	}
	if got.WantedAfter != 2 {
		t.Fatalf("the requested count is not carried: %s", got)
	}
	if !strings.Contains(got.String(), "verified=false") {
		t.Fatalf("the rendering hides that nothing was confirmed: %s", got)
	}
}

// TestANoOpIsTheOneThingThisBuildCanConfirm.
func TestANoOpIsTheOneThingThisBuildCanConfirm(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, why: "NOT_A_MEMBER", before: 2}
	got, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem")
	if err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if !got.NoOp || !got.Verified {
		t.Fatalf("a no-op is not reported as confirmed: %s", got)
	}
}

// TestTheScriptDoesNotWaitOnSomethingThatNeverMoves. Three runs waited 90
// seconds on the metadata for changes that had already worked.
func TestTheScriptDoesNotWaitOnSomethingThatNeverMoves(t *testing.T) {
	compressPartClock(t)
	p := &partDouble{ok: true, before: 3}
	if _, err := parts(p).RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t/rem"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if strings.Contains(p.lastScript, "stage: 'settling'") {
		t.Fatal("the script still waits on a signal this build never sends")
	}
	if strings.Contains(participantsResultScript, "settling") {
		t.Fatal("the result script still has a settling branch")
	}
}

// TestParticipantsArePassedAsRecords is the fix that unblocked H58, and it is
// invisible in both signatures: the app's call site carries entries with .id.
func TestParticipantsArePassedAsRecords(t *testing.T) {
	compressPartClock(t)
	for _, tc := range []struct {
		name string
		run  func(*Manager) error
	}{
		{"add", func(m *Manager) error {
			_, err := m.AddParticipant(context.Background(), testGroupJID, "1@c.us", "t")
			return err
		}},
		{"remove", func(m *Manager) error {
			_, err := m.RemoveParticipant(context.Background(), testGroupJID, "1@c.us", "t")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &partDouble{ok: true, before: 3}
			if err := tc.run(parts(p)); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !strings.Contains(p.lastScript, "part(r.wid)") {
				t.Fatal("a bare wid is passed where a participant record is wanted")
			}
			if strings.Contains(p.lastScript, "[r.wid]") {
				t.Fatal("the bare-wid shape that threw is still in the script")
			}
		})
	}
}

// TestOnlyCountsAreReported. A group's membership is a list of people, and the
// error and the rendering carry numbers rather than names.
func TestOnlyCountsAreReported(t *testing.T) {
	m := Membership{Before: 2, WantedAfter: 3}
	s := m.String()
	if strings.Contains(s, "@") {
		t.Fatalf("the rendering carries something jid-shaped: %s", s)
	}
	if !strings.Contains(s, "before=2") || !strings.Contains(s, "wantedAfter=3") {
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

// TestCountIsHonestAboutWhatItReads. Count exists because cross-session is the
// only proof this build allows, and it has to fail loudly when the metadata is
// not loaded rather than reporting an empty group.
func TestCountIsHonestAboutWhatItReads(t *testing.T) {
	t.Run("counts", func(t *testing.T) {
		var script string
		m := New(engine.NewRunner(), func(_ context.Context, expr string, out *string) error {
			script = expr
			*out = "7"
			return nil
		})
		n, err := m.Count(context.Background(), testGroupJID, "t")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if n != 7 {
			t.Fatalf("got %d, want 7", n)
		}
		if !strings.Contains(script, "String(") {
			t.Fatal("the script does not return a string; a bare number is a type error at the boundary")
		}
	})

	t.Run("metadata not loaded", func(t *testing.T) {
		m := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = "-1"
			return nil
		})
		_, err := m.Count(context.Background(), testGroupJID, "t")
		if !errors.Is(err, ErrNotGroup) {
			t.Fatalf("got %v, want ErrNotGroup", err)
		}
	})

	t.Run("not a group", func(t *testing.T) {
		called := false
		m := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			called = true
			*out = "2"
			return nil
		})
		if _, err := m.Count(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNotGroup) {
			t.Fatalf("got %v, want ErrNotGroup", err)
		}
		if called {
			t.Fatal("the page was asked to count a non-group's participants")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		m := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = "not a number"
			return nil
		})
		if _, err := m.Count(context.Background(), testGroupJID, "t"); err == nil {
			t.Fatal("a non-numeric answer was accepted as a count")
		}
	})

	t.Run("evaluator error", func(t *testing.T) {
		m := New(engine.NewRunner(), func(_ context.Context, _ string, _ *string) error {
			return errors.New("boom")
		})
		if _, err := m.Count(context.Background(), testGroupJID, "t"); !errors.Is(err, ErrParticipants) {
			t.Fatalf("got %v, want ErrParticipants", err)
		}
	})
}

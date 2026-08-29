package group

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// joinDouble answers the kick and then the parked read.
//
// IT IMITATES THE PAGE'S REAL PROTOCOL: the read expression is
// `window.__headlessGroupJoin || ""`, and the page answers "" until the
// promise settles. Answering immediately would leave the polling loop — where
// the budget and the context check live — never exercised.
type joinDouble struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
}

func (d *joinDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+joinStateKey) {
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

func joiner(d *joinDouble) *Manager { return New(engine.NewRunner(), d.eval) }

// An empty code never reaches the page. A join with nothing to join is a caller
// mistake worth naming, not a page error worth discovering.
func TestAnEmptyInviteCodeIsRefused(t *testing.T) {
	for _, code := range []string{"", "   "} {
		d := &joinDouble{answer: `{"ok":true}`}
		if _, err := joiner(d).JoinByInvite(context.Background(), code, "t"); !errors.Is(err, ErrNoInviteCode) {
			t.Errorf("JoinByInvite(%q) = %v, want ErrNoInviteCode", code, err)
		}
		if _, err := joiner(d).InviteInfo(context.Background(), code, "t"); !errors.Is(err, ErrNoInviteCode) {
			t.Errorf("InviteInfo(%q) = %v, want ErrNoInviteCode", code, err)
		}
		if d.kicks != 0 {
			t.Errorf("an empty code reached the page")
		}
	}
}

// A JOIN THAT BECOMES A REQUEST IS NEITHER SUCCESS NOR FAILURE.
//
// This is the measured behaviour and the reason the error exists: the account
// is not in the group, so a caller that reads "joined" and starts sending is
// wrong; but nothing failed, so a caller that retries is wrong too.
func TestAPendingJoinIsItsOwnAnswer(t *testing.T) {
	d := &joinDouble{answer: `{"ok":true,"id":"","pending":true,` +
		`"kind":"UnexpectedJoinGroupViaInviteResponse",` +
		`"keys":["message","taalOpcodes","name","gid","membershipApprovalMode"]}`}
	got, err := joiner(d).JoinByInvite(context.Background(), "CODE123", "t")
	if !errors.Is(err, ErrJoinPending) {
		t.Fatalf("err = %v, want ErrJoinPending", err)
	}
	if !got.Pending {
		t.Error("the outcome does not say it is pending")
	}
	if got.GroupJID != "" {
		t.Error("a pending join reported a group jid; the account is not in it")
	}
	// The shape of the answer travels, in NAMES. It is the only way a
	// maintainer learns what the page handed back.
	if len(got.AnswerKeys) != 5 || got.AnswerKind == "" {
		t.Errorf("the answer shape was dropped: %s", got)
	}
}

// THE PENDING CASE IS RECOGNISED BY THE FIELD, NOT BY THE NAME.
//
// The page rejects with an object named UnexpectedJoinGroupViaInviteResponse,
// and a build that renamed it would silently turn every pending join into a
// hard error. membershipApprovalMode is the FACT; the name is a build detail.
func TestThePendingJoinIsRecognisedByTheCarriedField(t *testing.T) {
	d := &joinDouble{answer: `{"ok":true,"pending":true}`}
	if _, err := joiner(d).JoinByInvite(context.Background(), "CODE123", "t"); !errors.Is(err, ErrJoinPending) {
		t.Fatalf("err = %v", err)
	}
	// Matched with its `!== undefined` test rather than the bare word, which
	// also appears in this file's prose and in the reported key list — the
	// confusion that has cost this repository four separate mistakes.
	if !strings.Contains(d.lastScript, "e.membershipApprovalMode !== undefined") {
		t.Fatal("the join recognises the pending case by something other than the carried field")
	}
	if strings.Contains(d.lastScript, `e.name === "UnexpectedJoinGroupViaInviteResponse"`) {
		t.Error("the join keys off the response NAME, which is a build detail")
	}
}

// A join that reports success with no group id and no pending flag is refused
// rather than returned as an empty success. Both halves cannot be absent: that
// combination means the page answered something this code does not understand.
func TestAJoinWithNoIdAndNoPendingIsRefused(t *testing.T) {
	d := &joinDouble{answer: `{"ok":true,"id":"","pending":false}`}
	_, err := joiner(d).JoinByInvite(context.Background(), "CODE123", "t")
	if !errors.Is(err, ErrJoin) {
		t.Fatalf("err = %v, want ErrJoin", err)
	}
	if !strings.Contains(err.Error(), "no group id") {
		t.Errorf("the reason is unhelpful: %v", err)
	}
}

// A plain join returns the group it entered.
func TestAPlainJoinReturnsTheGroup(t *testing.T) {
	d := &joinDouble{pendingReads: 1, answer: `{"ok":true,"id":"123-456@g.us","pending":false}`}
	got, err := joiner(d).JoinByInvite(context.Background(), "CODE123", "t")
	if err != nil {
		t.Fatalf("JoinByInvite: %v", err)
	}
	if got.GroupJID != "123-456@g.us" || got.Pending {
		t.Fatalf("got %+v", got)
	}
	if d.reads != 2 {
		t.Errorf("%d reads; the double answered pending once and the loop must have polled", d.reads)
	}
}

// InviteInfo reads the group behind a link, INCLUDING whether it asks for
// approval — the field that tells a caller what a join will do before it does
// it.
func TestInviteInfoReadsTheApprovalFlag(t *testing.T) {
	d := &joinDouble{answer: `{"ok":true,"id":"123-456@g.us","subject":"lab","size":2,"approval":true}`}
	got, err := joiner(d).InviteInfo(context.Background(), "CODE123", "t")
	if err != nil {
		t.Fatalf("InviteInfo: %v", err)
	}
	if !got.ApprovalRequired {
		t.Fatal("the approval flag was dropped; a caller cannot tell what a join will do")
	}
	if got.Size != 2 || got.GroupJID == "" || got.Subject == "" {
		t.Fatalf("got %+v", got)
	}
}

// A page failure is an error on both methods, with the page's reason kept.
func TestAJoinPageFailureKeepsItsReason(t *testing.T) {
	d := &joinDouble{answer: `{"ok":false,"why":"TypeError message=nope keys=[a,b]"}`}
	if _, err := joiner(d).JoinByInvite(context.Background(), "C", "t"); !errors.Is(err, ErrJoin) ||
		!strings.Contains(err.Error(), "nope") {
		t.Errorf("join err = %v", err)
	}
	d2 := &joinDouble{answer: `{"ok":false,"why":"BAD_CODE"}`}
	if _, err := joiner(d2).InviteInfo(context.Background(), "C", "t"); !errors.Is(err, ErrJoin) ||
		!strings.Contains(err.Error(), "BAD_CODE") {
		t.Errorf("info err = %v", err)
	}
}

// THE FAILURE REPORT DESCRIBES WHAT WAS THROWN, not e.message.
//
// The first live run came back with an EMPTY reason, because this app rejects
// with plain objects as readily as with Errors. An empty reason looks like a
// silent failure while the diagnosis was in fact discarded at the boundary —
// and the whole membership-request family was invisible behind it.
func TestTheScriptDescribesWhateverWasThrown(t *testing.T) {
	d := &joinDouble{answer: `{"ok":true,"id":"1@g.us"}`}
	if _, err := joiner(d).JoinByInvite(context.Background(), "C", "t"); err != nil {
		t.Fatalf("JoinByInvite: %v", err)
	}
	for _, want := range []string{"constructor", "keys=[", "status=", "code="} {
		if !strings.Contains(d.lastScript, want) {
			t.Errorf("the failure report does not mention %q; a thrown non-Error would come back blank", want)
		}
	}
}

// Neither rendering carries what it should not: a group subject is not this
// module's to log, and a group jid identifies a real conversation.
func TestTheJoinRenderingsAreQuiet(t *testing.T) {
	j := Joined{GroupJID: "123-456@g.us", AnswerKind: "X"}
	if strings.Contains(j.String(), "123-456") {
		t.Errorf("Joined.String carries the jid: %s", j.String())
	}
	i := InviteInfo{GroupJID: "123-456@g.us", Subject: "segredo", Size: 3}
	if strings.Contains(i.String(), "segredo") || strings.Contains(i.String(), "123-456") {
		t.Errorf("InviteInfo.String carries identity: %s", i.String())
	}
}

// The parked loop is bounded by its own budget.
func TestTheJoinLoopIsBounded(t *testing.T) {
	oldB, oldT := createBudget, createTick
	createBudget, createTick = 40*time.Millisecond, 5*time.Millisecond
	defer func() { createBudget, createTick = oldB, oldT }()

	d := &joinDouble{pendingReads: 1 << 30}
	_, err := joiner(d).JoinByInvite(context.Background(), "C", "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

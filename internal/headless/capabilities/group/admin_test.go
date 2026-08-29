package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/headless/engine"
)

type adminDouble struct {
	ok         bool
	stage, why string

	kicks      int
	lastScript string
}

func (p *adminDouble) eval(ctx context.Context, expr string, out *string) error {
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":%q}`, p.why)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func admin(p *adminDouble) *Manager { return New(engine.NewRunner(), p.eval) }

// TestAThirdShapeInTheSameModule. add takes one object, remove takes seven
// positional arguments, promote takes four. Three signatures for four sibling
// operations, all exported side by side — this test fails if anyone tidies one
// into another.
func TestAThirdShapeInTheSameModule(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	if _, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !strings.Contains(p.lastScript, "J.promoteParticipantsJob(gwid, [found], md, false)") {
		t.Fatal("promote does not use the measured four-positional shape")
	}
	if strings.Contains(p.lastScript, "promoteParticipantsJob({") {
		t.Fatal("promote was turned into the object shape its sibling uses")
	}
	d := &adminDouble{ok: true}
	if _, err := admin(d).Demote(context.Background(), testGroupJID, "1@c.us", "t"); err != nil {
		t.Fatalf("Demote: %v", err)
	}
	if !strings.Contains(d.lastScript, "J.demoteParticipantsJob(gwid, [found], md, false)") {
		t.Fatal("demote does not use the measured shape")
	}
}

// TestTheParticipantRecordIsPassed — the H58 fix, carried here rather than
// rediscovered.
func TestTheParticipantRecordIsPassed(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	if _, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if strings.Contains(p.lastScript, "[r.wid]") {
		t.Fatal("a bare wid is passed where a participant record is wanted")
	}
	if !strings.Contains(p.lastScript, "[found]") {
		t.Fatal("the metadata's own participant record is not used")
	}
}

// TestARealAdminChangeIsUnverified, for the reason H58 measured.
func TestARealAdminChangeIsUnverified(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	got, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if got.Verified || got.NoOp {
		t.Fatalf("a real change claims to be confirmed: %s", got)
	}
	if !strings.Contains(got.String(), "verified=false") {
		t.Fatalf("the rendering hides that nothing was confirmed: %s", got)
	}
}

func TestAnAlreadyAdminIsANoOpAndIsConfirmable(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true, why: "ALREADY"}
	got, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t")
	if err != nil {
		t.Fatalf("promoting an existing admin produced an error: %v", err)
	}
	if !got.NoOp || !got.Verified {
		t.Fatalf("a no-op is not reported as confirmed: %s", got)
	}
	if !strings.Contains(p.lastScript, "if (!!found.isAdmin === promote) {") {
		t.Fatal("the script does not compare the current role before acting")
	}
}

func TestNotAnAdminCannotPromote(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: false, stage: "find", why: "NOT_ADMIN"}
	if _, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t"); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("got %v, want ErrNotAdmin", err)
	}
	if !strings.Contains(p.lastScript, "if (md.participants.iAmAdmin && !md.participants.iAmAdmin()) {") {
		t.Fatal("iAmAdmin is computed but does not guard a return")
	}
}

func TestPromotingANonMemberIsRefused(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: false, stage: "find", why: "NOT_A_MEMBER"}
	if _, err := admin(p).Promote(context.Background(), testGroupJID, "1@c.us", "t"); !errors.Is(err, ErrNoParticipantGiven) {
		t.Fatalf("got %v, want ErrNoParticipantGiven", err)
	}
}

// TestLeavingSomethingYouAreNotInIsRefused. A silent no-op here would leave the
// caller believing it had left a group it never joined.
func TestLeavingSomethingYouAreNotInIsRefused(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: false, stage: "find", why: "NOT_A_MEMBER"}
	if err := admin(p).Leave(context.Background(), testGroupJID, "t"); !errors.Is(err, ErrNotAMember) {
		t.Fatalf("got %v, want ErrNotAMember", err)
	}
	if !strings.Contains(p.lastScript, "!md.participants.iAmMember()") {
		t.Fatal("the script does not check membership before leaving")
	}
}

// TestLeaveTakesTheChatModel. sendExitGroup unproxies its single argument, which
// is this build's way of saying it wants a model.
func TestLeaveTakesTheChatModel(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	if err := admin(p).Leave(context.Background(), testGroupJID, "t"); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if !strings.Contains(p.lastScript, "A.sendExitGroup(chat)") {
		t.Fatal("leave does not pass the chat model")
	}
	if strings.Contains(p.lastScript, "sendExitGroup(gwid") {
		t.Fatal("leave passes a wid where a model is wanted")
	}
}

func TestAdminRefusalsCostNoPageCall(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	if _, err := admin(p).Promote(context.Background(), "1@c.us", "2@c.us", "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if _, err := admin(p).Demote(context.Background(), testGroupJID, "  ", "t"); !errors.Is(err, ErrNoParticipantGiven) {
		t.Fatalf("got %v, want ErrNoParticipantGiven", err)
	}
	if err := admin(p).Leave(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for calls that could not work", p.kicks)
	}
}

func TestCancelledContextChangesNoRole(t *testing.T) {
	compressPartClock(t)
	p := &adminDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := admin(p).Promote(ctx, testGroupJID, "1@c.us", "t"); err == nil {
		t.Fatal("a cancelled context promoted somebody")
	}
	if err := admin(p).Leave(ctx, testGroupJID, "t"); err == nil {
		t.Fatal("a cancelled context left a group")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAHungPageIsATimeout(t *testing.T) {
	compressPartClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	m := New(engine.NewRunner(), stuck)
	if _, err := m.Promote(context.Background(), testGroupJID, "1@c.us", "t"); !errors.Is(err, ErrAdminChange) {
		t.Fatalf("got %v, want ErrAdminChange", err)
	}
	if err := m.Leave(context.Background(), testGroupJID, "t"); !errors.Is(err, ErrAdminChange) {
		t.Fatalf("got %v, want ErrAdminChange", err)
	}
}

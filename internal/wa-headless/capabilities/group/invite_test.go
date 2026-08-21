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

// THE CAPABILITY DOES NOT WORK AGAINST THE LIVE PAGE (H57): queryGroupInviteCode
// reads iAmAdmin off a metadata that is not populated, and three argument
// shapes did not fix it.
//
// These tests cover the half that IS correct — the Go-side refusals, the
// credential handling and the link — so that the measured work is preserved and
// the coverage number stays honest about what exists. None of them claims the
// capability works.

type inviteDouble struct {
	ok    bool
	stage string
	why   string
	code  string

	kicks      int
	lastScript string
}

func (p *inviteDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "query"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","code":%q}`, p.code)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func inviter(p *inviteDouble) *Manager { return New(engine.NewRunner(), p.eval) }

func compressInviteClock(t *testing.T) {
	t.Helper()
	ob, ot := createBudget, createTick
	createBudget, createTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { createBudget, createTick = ob, ot })
}

// TestTheCodeIsNeverRendered. An invite code is a credential: anyone holding it
// can join the group, so a code in a log is a group anyone reading the log can
// join. Same rule as the QR code and the profile-picture url.
func TestTheCodeIsNeverRendered(t *testing.T) {
	i := Invite{Code: "ABCdef123456", Revoked: false}
	s := i.String()
	if strings.Contains(s, "ABCdef123456") {
		t.Fatalf("String() leaked the credential: %s", s)
	}
	if !strings.Contains(s, "len=12") {
		t.Fatalf("the length is the useful part: %s", s)
	}
	if empty := (Invite{}).String(); !strings.Contains(empty, "code=false") {
		t.Fatalf("an absent code must be distinguishable: %s", empty)
	}
}

// TestTheLinkIsBuiltOnDemand, not stored. A stored link and a stored code can
// drift; a method cannot. It also makes a caller ask for the dangerous form
// explicitly.
func TestTheLinkIsBuiltOnDemand(t *testing.T) {
	i := Invite{Code: "ABC123"}
	if i.Link() != "https://chat.whatsapp.com/ABC123" {
		t.Fatalf("Link()=%q", i.Link())
	}
	if (Invite{}).Link() != "" {
		t.Fatal("a group with no code produced a joinable link")
	}
}

// TestANonGroupIsRefusedBeforeThePage. Asking for the invite of a person is a
// caller mistake, and turning it into a page call would be asking WhatsApp to
// explain our bug.
func TestANonGroupIsRefusedBeforeThePage(t *testing.T) {
	compressInviteClock(t)
	p := &inviteDouble{ok: true, code: "X"}
	if _, err := inviter(p).InviteCode(context.Background(), "5541999998888@c.us", "t/inv"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if _, err := inviter(p).InviteCode(context.Background(), "  ", "t/inv"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup for a blank jid", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for a non-group", p.kicks)
	}
}

// TestNotBeingAnAdminIsItsOwnAnswer. It is not a failure of the call — it is a
// fact about who this account is in that group, and a caller can act on it.
func TestNotBeingAnAdminIsItsOwnAnswer(t *testing.T) {
	compressInviteClock(t)
	p := &inviteDouble{ok: false, stage: "query", why: "NOT_ADMIN"}
	if _, err := inviter(p).InviteCode(context.Background(), "120363@g.us", "t/inv"); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("got %v, want ErrNotAdmin", err)
	}
}

// TestReadingAndRevokingAreDifferentCalls. Revoking locks out everyone holding
// the old link, so it cannot be a flag on the read — a caller must name it.
func TestReadingAndRevokingAreDifferentCalls(t *testing.T) {
	compressInviteClock(t)
	read := &inviteDouble{ok: true, code: "AAA"}
	got, err := inviter(read).InviteCode(context.Background(), "120363@g.us", "t/read")
	if err != nil {
		t.Fatalf("InviteCode: %v", err)
	}
	if got.Revoked {
		t.Fatal("a read reported itself as a revocation")
	}
	if !strings.Contains(read.lastScript, "queryGroupInviteCode(") {
		t.Fatal("the read does not call queryGroupInviteCode")
	}
	if strings.Contains(read.lastScript, "revokeGroupInvite(") {
		t.Fatal("the READ script can revoke the code")
	}

	rev := &inviteDouble{ok: true, code: "BBB"}
	rot, err := inviter(rev).RevokeInvite(context.Background(), "120363@g.us", "t/rev")
	if err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	if !rot.Revoked {
		t.Fatal("a revocation did not report itself as one")
	}
	if !strings.Contains(rev.lastScript, "revokeGroupInvite(") {
		t.Fatal("the revoke does not call revokeGroupInvite")
	}
}

// TestTheMetadataIsQueriedFirst is the finding H57 records: the invite call
// reads iAmAdmin off a metadata that is not populated by default.
func TestTheMetadataIsQueriedFirst(t *testing.T) {
	compressInviteClock(t)
	p := &inviteDouble{ok: true, code: "AAA"}
	if _, err := inviter(p).InviteCode(context.Background(), "120363@g.us", "t/inv"); err != nil {
		t.Fatalf("InviteCode: %v", err)
	}
	meta := strings.Index(p.lastScript, "queryAndUpdateGroupMetadataById")
	query := strings.Index(p.lastScript, "queryGroupInviteCode(")
	if meta < 0 {
		t.Fatal("the metadata is never queried; the invite call reads iAmAdmin off it")
	}
	if query < 0 || meta > query {
		t.Fatal("the invite is asked for BEFORE the metadata it depends on")
	}
}

func TestASuccessWithNoCodeIsRefused(t *testing.T) {
	compressInviteClock(t)
	p := &inviteDouble{ok: true, code: ""}
	if _, err := inviter(p).InviteCode(context.Background(), "120363@g.us", "t/inv"); !errors.Is(err, ErrInvite) {
		t.Fatalf("got %v, want ErrInvite: a success with no credential is not a success", err)
	}
}

func TestCancelledContextAsksNothing(t *testing.T) {
	compressInviteClock(t)
	p := &inviteDouble{ok: true, code: "AAA"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := inviter(p).RevokeInvite(ctx, "120363@g.us", "t/rev"); err == nil {
		t.Fatal("a cancelled context revoked an invite")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAStalledPageIsNotAnInvite(t *testing.T) {
	compressInviteClock(t)
	p := &stallingInviteDouble{}
	_, err := New(engine.NewRunner(), p.eval).InviteCode(context.Background(), "120363@g.us", "t/inv")
	if !errors.Is(err, ErrInvite) {
		t.Fatalf("got %v, want ErrInvite", err)
	}
	if !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a stall must be distinguishable from a refusal: %v", err)
	}
}

type stallingInviteDouble struct{}

func (p *stallingInviteDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		*out = `{"stage":"pending","ok":false,"why":""}`
		return nil
	}
	*out = `{"started":true}`
	return nil
}

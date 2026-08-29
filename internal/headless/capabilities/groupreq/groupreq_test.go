package groupreq

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// double answers the two calls this package makes: the kick, and the poll for
// the parked result.
//
// IT IMITATES THE PAGE'S REAL SHAPE, not a convenient one. The poll expression
// is `window.__headlessGroupReq || ""`, and the page answers "" until the
// parked promise settles — so the double answers "" for the first pendingReads
// too. A double that answered immediately would never exercise the polling
// loop, which is where the deadline and the context check live.
type double struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
	err        error
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.err != nil {
		return d.err
	}
	// A LIBERACAO NAO E' LEITURA (H177): ela roda DEPOIS de a resposta ser
	// tomada, e conta-la faz um teste de numero de voltas medir uma volta que
	// nao existe.
	if strings.Contains(expr, "delete window."+stateKeyPrefix) {
		return nil
	}
	if strings.HasPrefix(expr, "window."+stateKeyPrefix) {
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

func mgr(d *double) *Manager { return New(engine.NewRunner(), d.eval) }

const someGroup = "1234567890-1600000000@g.us"

// A group jid is required, and a user jid is refused BEFORE anything is
// evaluated. Sending a membership action to a private chat is not a page error
// worth discovering; it is a caller mistake worth naming.
func TestNonGroupIsRefusedWithoutTouchingThePage(t *testing.T) {
	for _, jid := range []string{"5541999999999@c.us", "", "  ", "abc"} {
		d := &double{answer: `{"ok":true}`}
		if _, err := mgr(d).List(context.Background(), jid, "t"); !errors.Is(err, ErrNotGroup) {
			t.Errorf("List(%q) = %v, want ErrNotGroup", jid, err)
		}
		if _, err := mgr(d).Approve(context.Background(), jid, []string{"x@c.us"}, "t"); !errors.Is(err, ErrNotGroup) {
			t.Errorf("Approve(%q) = %v, want ErrNotGroup", jid, err)
		}
		if d.kicks != 0 {
			t.Errorf("%q reached the page %d times", jid, d.kicks)
		}
	}
}

// An approve with nothing to approve is an ERROR, not an empty success.
//
// "Approve everybody pending" and "approve these two" are different intentions.
// A caller that meant the second and passed an empty slice would otherwise get
// silence and believe it worked.
func TestAnEmptyRequesterListIsRefused(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"", "   "}} {
		d := &double{answer: `{"ok":true}`}
		if _, err := mgr(d).Approve(context.Background(), someGroup, in, "t"); !errors.Is(err, ErrNoRequesters) {
			t.Errorf("Approve(%v) = %v, want ErrNoRequesters", in, err)
		}
		if d.kicks != 0 {
			t.Errorf("an empty list reached the page")
		}
	}
}

// The list read parses what the page hands back, INCLUDING the field names.
//
// Fields is not decoration: the record shape could not be measured, because the
// lab group read back an empty array. The first live request is what says what
// is really there, and a read that dropped the names would throw that away.
func TestListParsesRequestsAndReportsTheFieldNames(t *testing.T) {
	d := &double{
		pendingReads: 2,
		answer: `{"ok":true,"fields":["id","addedBy","t","addedByLid"],` +
			`"requests":[{"id":"5541999999999@c.us","addedBy":"","t":1755000000,"method":"InviteLink"},` +
			`{"id":"5541988888888@c.us","addedBy":"5541977777777@c.us","t":0}]}`,
	}
	got, err := mgr(d).List(context.Background(), someGroup, "t")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Requests) != 2 {
		t.Fatalf("%d requests, want 2", len(got.Requests))
	}
	if got.Requests[0].At.IsZero() {
		t.Error("a request with a timestamp came back with a zero time")
	}
	if !got.Requests[1].At.IsZero() {
		t.Error("a request with t=0 was given a time; the epoch is not a timestamp")
	}
	if got.Requests[1].AddedByJID == "" {
		t.Error("addedBy was dropped")
	}
	if got.Requests[0].Method != "InviteLink" {
		t.Errorf("method = %q; the page's requestMethod was dropped", got.Requests[0].Method)
	}
	if len(got.Fields) != 4 {
		t.Fatalf("fields = %v, want the four the page reported", got.Fields)
	}
	if d.reads != 3 {
		t.Errorf("%d reads; the double answered pending twice and the loop must have polled", d.reads)
	}
}

// A page-level failure is an error, not an empty list. A read that returned
// zero requests for a group with three would be indistinguishable from a group
// with none — which is the worst possible answer for this particular question.
func TestAPageFailureIsNotAnEmptyList(t *testing.T) {
	d := &double{answer: `{"ok":false,"why":"NO_METADATA"}`}
	_, err := mgr(d).List(context.Background(), someGroup, "t")
	if !errors.Is(err, ErrRead) {
		t.Fatalf("err = %v, want ErrRead", err)
	}
	if !strings.Contains(err.Error(), "NO_METADATA") {
		t.Errorf("the reason was dropped: %v", err)
	}
}

// A PARTIAL outcome survives. The upstream loops one RPC per participant, so
// three approvals where the second fails is three results — and returning the
// first error would hide that the other two went through.
func TestAPartialOutcomeIsReportedPerRequester(t *testing.T) {
	d := &double{answer: `{"ok":true,"results":[` +
		`{"id":"a@c.us","ok":true},` +
		`{"id":"b@c.us","ok":false,"code":404,"why":"PARTICIPANT_ERROR"},` +
		`{"id":"c@c.us","ok":true}]}`}
	got, err := mgr(d).Approve(context.Background(), someGroup, []string{"a@c.us", "b@c.us", "c@c.us"}, "t")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("%d results, want 3", len(got))
	}
	if got[0].OK == got[1].OK {
		t.Error("a failed requester was reported the same as a successful one")
	}
	if got[1].Code != 404 {
		t.Errorf("code = %d, want the page's 404", got[1].Code)
	}
}

// Approve and Reject differ in the ARGUMENT KEY and in the response field they
// read, and nothing else. Getting that pair crossed would approve where the
// caller asked to reject — silently, because both produce a success.
func TestApproveAndRejectSendDifferentKeys(t *testing.T) {
	da := &double{answer: `{"ok":true,"results":[]}`}
	if _, err := mgr(da).Approve(context.Background(), someGroup, []string{"a@c.us"}, "t"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	dr := &double{answer: `{"ok":true,"results":[]}`}
	if _, err := mgr(dr).Reject(context.Background(), someGroup, []string{"a@c.us"}, "t"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	// Match the KEY AS SENT — `approveArgs:` with its colon — not the bare
	// word, which appears in this file's own comments and in the response
	// field names. That mistake has been made four times in this repository.
	if !strings.Contains(da.lastScript, "approveArgs: participant") {
		t.Error("Approve did not send approveArgs")
	}
	if strings.Contains(da.lastScript, "rejectArgs: participant") {
		t.Error("Approve also sent rejectArgs")
	}
	if !strings.Contains(dr.lastScript, "rejectArgs: participant") {
		t.Error("Reject did not send rejectArgs")
	}
	if strings.Contains(dr.lastScript, "approveArgs: participant") {
		t.Error("Reject also sent approveArgs")
	}
}

// THE READ REFRESHES BEFORE IT READS. A session holds the metadata it was given
// at boot; a request that arrived since is not in it. Without this the method
// answers "none pending" for a group that has three — the failure that looks
// like a correct answer.
func TestTheListRefreshesFromTheServerFirst(t *testing.T) {
	d := &double{answer: `{"ok":true,"requests":[]}`}
	if _, err := mgr(d).List(context.Background(), someGroup, "t"); err != nil {
		t.Fatalf("List: %v", err)
	}
	refresh := strings.Index(d.lastScript, "queryAndUpdateGroupMetadataById")
	read := strings.Index(d.lastScript, "getMembershipApprovalRequests")
	if refresh < 0 {
		t.Fatal("the list never refreshes the metadata")
	}
	if read < 0 {
		t.Fatal("the list never reads the store")
	}
	// THE ORDER, not merely the presence. Both calls in the wrong order pass
	// every other assertion in this file and answer stale.
	if refresh > read {
		t.Fatal("the refresh runs AFTER the read; the read would answer with boot-time metadata")
	}
}

// The parked loop gives up on its own budget rather than hanging forever, and
// it honours the caller's context.
func TestTheParkedLoopIsBounded(t *testing.T) {
	old := Budget
	Budget = 40 * time.Millisecond
	defer func() { Budget = old }()
	oldTick := Tick
	Tick = 5 * time.Millisecond
	defer func() { Tick = oldTick }()

	d := &double{pendingReads: 1 << 30} // never settles
	_, err := mgr(d).List(context.Background(), someGroup, "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

// Neither identity is rendered. This is the one place a requester's phone
// number would leak into a log, because a result is exactly what gets logged.
func TestNothingRendersAnIdentity(t *testing.T) {
	l := List{GroupJID: someGroup, Requests: []Request{{RequesterJID: "5541999999999@c.us"}}}
	if strings.Contains(l.String(), "5541") || strings.Contains(l.String(), someGroup) {
		t.Errorf("List.String carries identity: %s", l.String())
	}
	a := ActionResult{RequesterJID: "5541999999999@c.us", Code: 404}
	if strings.Contains(a.String(), "5541") {
		t.Errorf("ActionResult.String carries identity: %s", a.String())
	}
}

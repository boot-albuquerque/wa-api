// Package groupreq reads and answers the requests to JOIN a group.
//
// A group with membership approval on does not add whoever follows its invite
// link: the link produces a REQUEST, and an admin approves or rejects it. That
// is three operations upstream (list, approve, reject) and one event, and this
// package is the whole family rather than one method, because splitting it
// leaves a build that can see requests and not answer them.
//
// WHAT THE REFERENCE ANSWERED, AND WHAT IT DID NOT. whatsapp-web.js reads
// through WAWebApiMembershipApprovalRequestStore.getMembershipApprovalRequests
// and writes through WASmaxGroupsMembershipRequestsActionRPC, one participant
// per call. Both module names exist on THIS build unchanged — the first family
// where the reference's list transfers whole, against four out of four missing
// for sendText. That is worth recording precisely because the opposite has been
// the norm.
//
// WHAT THE MEASUREMENT ADDED. This build also exports
// WAWebGroupQueryJob.maybeQueryAndUpdateMembershipApprovalRequests, a targeted
// refresh the reference does not use. It is NOT used here, and the reason is
// measured rather than aesthetic: it accepted a wid, an {id} object, a bare
// string and a whole chat object, and resolved undefined for all four. A
// function that cannot reject a wrong argument cannot confirm a right one, so
// there is no way to tell it apart from a no-op. The reference's
// queryAndUpdateGroupMetadataById is used instead (H89).
package groupreq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Errors this package returns.
var (
	// ErrNotGroup is a jid that is not a group.
	ErrNotGroup = fmt.Errorf("groupreq: not a group jid")
	// ErrRead is the page refusing or failing the read.
	ErrRead = fmt.Errorf("groupreq: the page refused the membership-request read")
	// ErrAction is the page refusing or failing an approve/reject.
	ErrAction = fmt.Errorf("groupreq: the page refused the membership-request action")
	// ErrNoRequesters is an approve or reject with nothing to act on. It is an
	// error rather than an empty success because "approve everybody pending"
	// and "approve these two" are different intentions, and a caller that meant
	// the second and passed an empty slice would otherwise get silence.
	ErrNoRequesters = fmt.Errorf("groupreq: no requesters given")
)

// Budgets. Var so a test can compress them.
var (
	// Budget bounds one parked page call.
	Budget = 30 * time.Second
	// Tick is how often Go asks the parked call whether it finished.
	Tick = 250 * time.Millisecond
)

const (
	stateKeyPrefix = "__waHeadlessGroupReq"
	groupJIDSuffix = "@g.us"
)

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Request is one pending request to join.
//
// IT IS METADATA ONLY, like everything else that crosses this module's
// boundary: a requester is a phone number, so it is carried for routing and
// never rendered by String.
type Request struct {
	// RequesterJID is who asked to join.
	RequesterJID string
	// AddedByJID is who put them there, when the page says so — a request made
	// through an invite link has no adder, and the field is then empty.
	AddedByJID string
	// At is when the page says the request was made. Zero when absent.
	At time.Time
	// Method is HOW the request arrived — the page's own word, measured as
	// "InviteLink" for a request made by following a link. It distinguishes a
	// stranger with a link from somebody a member tried to add, which is the
	// difference an admin actually decides on.
	//
	// It is not in the reference's shape at all: the field names came from the
	// first real pending request this module ever produced, and were
	// [id t addedBy requestMethod parentGroupId] (H89).
	Method string
}

// List is what one read saw.
type List struct {
	GroupJID string
	Requests []Request
	// Fields is the field NAMES the page's request records carried, never the
	// values. It exists because the shape could not be measured on a group with
	// no pending requests — the lab group read back an empty array — so the
	// first live request is what says what is really there. A caller can ignore
	// it; a maintainer reading a log cannot get it any other way.
	Fields []string
}

func (l List) String() string {
	return fmt.Sprintf("groupreq.List(group=%t requests=%d fields=%v)",
		l.GroupJID != "", len(l.Requests), l.Fields)
}

// ActionResult is what one approve or reject did to ONE requester.
//
// The upstream loops one RPC per participant and so does this, which means a
// partial outcome is normal: three approvals where the second fails is three
// results, not one error. Returning a slice makes that visible; returning the
// first error would hide it.
type ActionResult struct {
	RequesterJID string
	OK           bool
	// Code is the page's own error number when OK is false: 400 not found, 401
	// not authorised, 403 forbidden, 404 request not found, 408 temporarily
	// blocked, 409 conflict, 412 linked-group constraint, 500 resource
	// constraint. Zero when OK.
	Code int
	// Why is the page's response name when the RPC itself did not succeed.
	Why string
}

func (a ActionResult) String() string {
	return fmt.Sprintf("groupreq.ActionResult(requester=%t ok=%t code=%d why=%s)",
		a.RequesterJID != "", a.OK, a.Code, a.Why)
}

// Manager reads and answers membership requests.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// List reports the requests pending on a group.
//
// IT REFRESHES FROM THE SERVER FIRST. A session that has been up for a while
// holds whatever metadata it was given at boot, and a request that arrived
// since is not in it. The reference does the same, and skipping it would make
// this method answer "none pending" for a group that has three.
func (m *Manager) List(ctx context.Context, groupJID, label string) (List, error) {
	if !isGroup(groupJID) {
		return List{}, ErrNotGroup
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, listScript(groupJID, key), key, label+"/list")
	if err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool     `json:"ok"`
		Why      string   `json:"why"`
		Fields   []string `json:"fields"`
		Requests []struct {
			ID      string `json:"id"`
			AddedBy string `json:"addedBy"`
			T       int64  `json:"t"`
			Method  string `json:"method"`
		} `json:"requests"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return List{}, fmt.Errorf("groupreq: unexpected list answer: %w", e)
	}
	if !out.OK {
		return List{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	l := List{GroupJID: groupJID, Fields: out.Fields}
	for _, r := range out.Requests {
		req := Request{RequesterJID: r.ID, AddedByJID: r.AddedBy, Method: r.Method}
		if r.T > 0 {
			req.At = time.Unix(r.T, 0)
		}
		l.Requests = append(l.Requests, req)
	}
	return l, nil
}

// Approve lets the named requesters in.
func (m *Manager) Approve(ctx context.Context, groupJID string, requesters []string, label string) ([]ActionResult, error) {
	return m.act(ctx, groupJID, requesters, true, label)
}

// Reject turns them away.
func (m *Manager) Reject(ctx context.Context, groupJID string, requesters []string, label string) ([]ActionResult, error) {
	return m.act(ctx, groupJID, requesters, false, label)
}

func (m *Manager) act(ctx context.Context, groupJID string, requesters []string, approve bool, label string) ([]ActionResult, error) {
	if !isGroup(groupJID) {
		return nil, ErrNotGroup
	}
	clean := make([]string, 0, len(requesters))
	for _, r := range requesters {
		if s := strings.TrimSpace(r); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return nil, ErrNoRequesters
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, actionScript(groupJID, clean, approve, key), key, label+"/action")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAction, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Results []struct {
			ID   string `json:"id"`
			OK   bool   `json:"ok"`
			Code int    `json:"code"`
			Why  string `json:"why"`
		} `json:"results"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, fmt.Errorf("groupreq: unexpected action answer: %w", e)
	}
	if !out.OK {
		return nil, fmt.Errorf("%w (%s)", ErrAction, out.Why)
	}
	res := make([]ActionResult, 0, len(out.Results))
	for _, r := range out.Results {
		res = append(res, ActionResult{RequesterJID: r.ID, OK: r.OK, Code: r.Code, Why: r.Why})
	}
	return res, nil
}

// parked kicks a page script that stores its answer on a global and polls for
// it, which is this module's one way of awaiting a promise: engine.Evaluate
// does not await, and invariant 6 keeps the clock on this side.
func (m *Manager) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return m.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return m.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida (H177).
			var ignored string
			_ = m.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return m.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}

func isGroup(jid string) bool {
	return strings.HasSuffix(strings.TrimSpace(jid), groupJIDSuffix)
}

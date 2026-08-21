// Package call is the calls family: create a call link, and turn a call down.
//
// IT IS SMALL BECAUSE THE UPSTREAM IS SMALL. whatsapp-web.js exposes exactly
// two things about calls — Client.createCallLink and Call.reject — plus one
// event. There is no "place a call" in it, and that shapes what can be PROVEN
// here: a link can be made and checked, and a rejection needs a call that
// somebody has to make.
//
// WHAT THE MEASUREMENT ADDED, AND WHAT IT COST. Enumerating this build's
// modules turned up a whole VOIP surface the reference has no equivalent for:
// WAWebVoipStartCall can originate a call, WAWebVoipCancelOutgoingCall can take
// one back, and WAWebVoipCreateCallLink is a call-link path of its own. The
// last of those was tried first and HUNG — createCallLink('video') never
// settled in forty seconds, which is the same class of answer as the invite
// hang in H57: it wants a VOIP stack this headless session does not bring up.
//
// So this uses what the reference uses, WAWebGenerateEventCallLink, and says
// here that the choice is measured rather than inherited (H92).
package call

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Kind is what a call link is for.
//
// The upstream refuses anything else before touching the page, and so does
// this: a typo would otherwise reach the server and come back as something
// unrecognisable.
type Kind string

const (
	KindVoice Kind = "voice"
	KindVideo Kind = "video"
)

var known = map[Kind]bool{KindVoice: true, KindVideo: true}

var (
	// ErrUnknownKind is a call kind outside {voice, video}.
	ErrUnknownKind = fmt.Errorf("call: unknown call kind")
	// ErrPastStart is a start time that has already gone by. The page would
	// take it and produce a link to a moment in the past, which is a link
	// nobody can use and no error anybody sees.
	ErrPastStart = fmt.Errorf("call: the start time is in the past")
	// ErrLink is the page refusing or failing the link creation.
	ErrLink = fmt.Errorf("call: the page refused to create the call link")
	// ErrNoLink is the page settling with no link.
	ErrNoLink = fmt.Errorf("call: the page settled and no link came back")
	// ErrReject is the page refusing or failing the rejection.
	ErrReject = fmt.Errorf("call: the page refused to reject the call")
	// ErrNoCall is a rejection with no call to reject.
	ErrNoCall = fmt.Errorf("call: a rejection needs both a caller and a call id")
	// ErrPlace is the page refusing or failing to place or cancel a call.
	ErrPlace = fmt.Errorf("call: the page refused to place the call")
	// ErrNotOnWhatsApp is a peer the server does not know.
	ErrNotOnWhatsApp = fmt.Errorf("call: that number is not on WhatsApp")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKey = "__waHeadlessCall"

// Link is a joinable call link.
//
// IT IS A CREDENTIAL, treated exactly like a group invite code: anybody holding
// it can join the call, so String never renders it and a caller has to ask for
// the URL explicitly.
type Link struct {
	// URL is the joinable link. Never rendered.
	URL string
	// Kind is what it opens.
	Kind Kind
	// StartsAt is the moment it was made for.
	StartsAt time.Time
}

func (l Link) String() string {
	return fmt.Sprintf("call.Link(url=%t len=%d kind=%s startsAt=%t)",
		l.URL != "", len(l.URL), l.Kind, !l.StartsAt.IsZero())
}

// Manager drives the calls surface.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// CreateLink makes a joinable link for a call starting at startAt.
//
// THE TIMESTAMP IS CONVERTED HERE, not in the page. The page is handed unix
// seconds and never asked what time it is — the same rule that keeps every
// deadline in this module on the Go side (invariant 6).
func (m *Manager) CreateLink(ctx context.Context, startAt time.Time, kind Kind, label string) (Link, error) {
	if !known[kind] {
		return Link{}, fmt.Errorf("%w: %q (accepted: voice, video)", ErrUnknownKind, string(kind))
	}
	if startAt.IsZero() {
		return Link{}, fmt.Errorf("%w: no start time given", ErrPastStart)
	}
	if !startAt.After(time.Now()) {
		return Link{}, fmt.Errorf("%w: %s", ErrPastStart, startAt.Format(time.RFC3339))
	}
	raw, err := m.parked(ctx, linkScript(startAt.Unix(), kind), label+"/link")
	if err != nil {
		return Link{}, fmt.Errorf("%w: %v", ErrLink, err)
	}
	var out struct {
		OK   bool   `json:"ok"`
		Why  string `json:"why"`
		Link string `json:"link"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Link{}, fmt.Errorf("call: unexpected link answer: %w", e)
	}
	if !out.OK {
		return Link{}, fmt.Errorf("%w (%s)", ErrLink, out.Why)
	}
	if strings.TrimSpace(out.Link) == "" {
		// THE UPSTREAM RETURNS `response ?? ''` AND CALLS THAT SUCCESS. An
		// empty string is not a link, and a caller handed one would publish it.
		return Link{}, ErrNoLink
	}
	return Link{URL: out.Link, Kind: kind, StartsAt: startAt}, nil
}

// Reject turns down a call that is ringing.
//
// IT HAS NO LOCAL POSTCONDITION THIS CAN CHECK. The rejection is a stanza cast
// at the server; nothing in the page confirms it, and the caller's evidence is
// that the ringing stops. That is stated rather than papered over with a read
// that would always succeed.
func (m *Manager) Reject(ctx context.Context, callerJID, callID, label string) error {
	if strings.TrimSpace(callerJID) == "" || strings.TrimSpace(callID) == "" {
		return ErrNoCall
	}
	raw, err := m.parked(ctx, rejectScript(callerJID, callID), label+"/reject")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReject, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("call: unexpected reject answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrReject, out.Why)
	}
	return nil
}

// Place ORIGINATES a call, and it is the one method in this module that makes
// hardware in somebody's pocket make noise.
//
// IT IS NOT IN THE UPSTREAM. whatsapp-web.js has no way to place a call at all;
// this build does, and the module scan found it (WAWebVoipStartCall). So this
// exists for two reasons and both are stated rather than assumed: it is a real
// capability of the platform that a product may want, and it is the ONLY way to
// prove Call.reject and the incoming-call event, which otherwise wait forever
// for somebody to dial.
//
// THE IDENTITY IS RESOLVED FIRST, exactly as the app's own call sites do:
// queryWidExists, then the wid it returns. This build files under LID (397 of
// 399 messages), and calling the phone number would be calling something the
// server does not route.
//
// A SELF-CALL IS REFUSED BY THE APP, not by this — the page redirects a call to
// your own account to a chat, silently. The refusal is reported rather than
// dressed up as a placed call.
func (m *Manager) Place(ctx context.Context, peerJID string, video bool, label string) error {
	if strings.TrimSpace(peerJID) == "" {
		return ErrNoCall
	}
	raw, err := m.parked(ctx, placeScript(peerJID, video), label+"/place")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPlace, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("call: unexpected place answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrPlace, out.Why)
	}
	return nil
}

// Cancel takes back a call this account placed and that is still ringing.
//
// It takes no arguments because the page's own function takes none: it acts on
// whatever outgoing call is pending, which means it is safe to call when there
// is none and impossible to aim at a specific one.
func (m *Manager) Cancel(ctx context.Context, label string) error {
	raw, err := m.parked(ctx, cancelScript, label+"/cancel")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPlace, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("call: unexpected cancel answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrPlace, out.Why)
	}
	return nil
}

// EnsureReady brings up this session's VOIP stack.
//
// A SESSION THAT HAS NOT DONE THIS IS NOT A CALL ENDPOINT. The application runs
// it from the UI path that precedes any call button; a headless driver has none,
// so it has to say it out loud — the same shape as presence, where a session
// that never announced availability is not one whose typing anybody is told
// about (H50).
//
// It is exported rather than folded into Place because RECEIVING needs it too,
// and the receiving side never calls Place.
func (m *Manager) EnsureReady(ctx context.Context, label string) error {
	raw, err := m.parked(ctx, ensureVoipScript, label+"/voip-init")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPlace, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("call: unexpected voip-init answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrPlace, out.Why)
	}
	return nil
}

// Pending reports the calls this session currently holds, in COUNTS and states
// — never a caller's identity.
//
// It exists because Reject needs a caller and a call id, and until a call
// arrives there is no way to see whether the collection holds anything at all.
func (m *Manager) Pending(ctx context.Context, label string) (int, error) {
	raw, err := m.parked(ctx, pendingScript, label+"/pending")
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrReject, err)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		Count int    `json:"count"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return 0, fmt.Errorf("call: unexpected pending answer: %w", e)
	}
	if !out.OK {
		return 0, fmt.Errorf("%w (%s)", ErrReject, out.Why)
	}
	return out.Count, nil
}

func (m *Manager) parked(ctx context.Context, kick, label string) (string, error) {
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
			return m.eval(c, `window.`+stateKey+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
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

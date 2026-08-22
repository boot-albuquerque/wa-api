// Package channel reads WhatsApp Channels — what the page calls newsletters.
//
// THE FAMILY UNLOCKED WITHOUT TOUCHING THE ACCOUNT. The newsletter collection is
// empty on an account that follows nothing, so the model had no instance to
// inspect, and the app's own discovery (getRecommendedNewsletters) does not
// answer — not even its own eight-second timeout (H102). The plan was to follow a
// channel and give it back.
//
// It turned out to be unnecessary: queryNewsletterMetadataByInviteCode answers
// for a channel this account does NOT follow, and carries the whole shape. The
// membership mixin comes back null, which is precisely what says "not a member"
// — so the read proves itself (H104).
//
// THE SHAPE IS MIXINS, WHICH THE REFERENCE DOES NOT DESCRIBE. There is no `name`
// field at the top level; there is newsletterNameMetadataMixin.nameElementValue.
// Reading the obvious field returns undefined forever, which is the defect class
// this repository has met four times.
package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoCode is an empty invite code.
	ErrNoCode = fmt.Errorf("channel: no invite code given")
	// ErrRead is the page refusing or failing the read.
	ErrRead = fmt.Errorf("channel: the page refused the channel read")
	// ErrNotFound is a code the server does not know.
	ErrNotFound = fmt.Errorf("channel: no channel for that invite code")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKey = "__waHeadlessChannel"

// Channel is what a channel says about itself.
//
// THE NAME AND DESCRIPTION ARE PUBLIC AND STILL CONTENT. They are carried
// because a reader exists to show them, and String renders neither. The picture
// is a count-free boolean: the url is large, it expires, and nothing here needs
// it.
type Channel struct {
	// JID identifies the channel; its domain is "newsletter".
	JID string
	// InviteCode is the code this channel was found by, as the page reports it
	// back — which is not necessarily the one that was asked for, and comparing
	// them is how a caller notices a redirect.
	InviteCode  string
	Name        string
	Description string
	// Subscribers is how many follow it. Zero is a legitimate answer.
	Subscribers int
	// State and Verification are the page's own words, carried verbatim rather
	// than mapped: a value this package has not seen must not become one it has.
	State        string
	Verification string
	// CreatedAt is when the channel was made. Zero when absent.
	CreatedAt time.Time
	// Following says this account is a member. It is derived from the
	// membership mixin being present — null means not a member, which is how
	// this package was proven without following anything.
	Following bool
	// HasPicture says the channel has one; the url is deliberately not carried.
	HasPicture bool
}

func (c Channel) String() string {
	return fmt.Sprintf("channel.Channel(jid=%t name=%t desc=%t subscribers=%d state=%s verification=%s following=%t picture=%t)",
		c.JID != "", c.Name != "", c.Description != "", c.Subscribers,
		c.State, c.Verification, c.Following, c.HasPicture)
}

// Reader reads channels.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// ByInviteCode reads a channel from its invite code.
//
// IT WORKS FOR A CHANNEL THIS ACCOUNT DOES NOT FOLLOW, which is what makes the
// whole family readable without joining anything. The code is the tail of a
// https://whatsapp.com/channel/<code> link; a full url is accepted and trimmed,
// because a caller holding the link should not have to know that.
func (r *Reader) ByInviteCode(ctx context.Context, code, label string) (Channel, error) {
	code = normalizeCode(code)
	if code == "" {
		return Channel{}, ErrNoCode
	}
	raw, err := r.parked(ctx, byInviteScript(code), label+"/by-invite")
	if err != nil {
		return Channel{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		JID      string `json:"jid"`
		Code     string `json:"code"`
		Name     string `json:"name"`
		Desc     string `json:"description"`
		Subs     int    `json:"subscribers"`
		State    string `json:"state"`
		Verif    string `json:"verification"`
		Created  int64  `json:"createdAt"`
		Member   bool   `json:"member"`
		Picture  bool   `json:"picture"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Channel{}, fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Channel{}, ErrNotFound
	}
	if !out.OK {
		return Channel{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	c := Channel{
		JID: out.JID, InviteCode: out.Code, Name: out.Name, Description: out.Desc,
		Subscribers: out.Subs, State: out.State, Verification: out.Verif,
		Following: out.Member, HasPicture: out.Picture,
	}
	if out.Created > 0 {
		c.CreatedAt = time.Unix(out.Created, 0)
	}
	return c, nil
}

// normalizeCode accepts either the bare code or the whole link.
//
// A CALLER HOLDS A LINK, not a code. Making them split it is making them know
// something this package already knows, and a link pasted whole is the shape
// that actually reaches a program.
func normalizeCode(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

func (r *Reader) parked(ctx context.Context, kick, label string) (string, error) {
	var started string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return r.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return r.eval(c, `window.`+stateKey+` || ""`, &raw)
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

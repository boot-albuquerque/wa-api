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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
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

const stateKeyPrefix = "__headlessChannel"

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

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
	// ReactionPolicyRaw is the page's OWN number for who may react, carried
	// verbatim. -1 means the metadata did not include it.
	//
	// IT IS THE RAW VALUE, not the reference's code. The upstream API takes
	// 0/1/2 and maps them to 3/1/0 before sending (Channel.js:184–197); carrying
	// the page's number keeps the two vocabularies separable, so a build that
	// changes the mapping is visible instead of silently reinterpreted.
	ReactionPolicyRaw int
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
	key := nextStateKey()
	raw, err := r.parked(ctx, byInviteScript(code, key), key, label+"/by-invite")
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
		Reaction int    `json:"reactionRaw"`
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
		ReactionPolicyRaw: out.Reaction,
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

func (r *Reader) parked(ctx context.Context, kick, key, label string) (string, error) {
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
			return r.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida (H177).
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
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

// DirectoryEntry is one result from the channel directory.
//
// IT IS A SEPARATE TYPE FROM Channel, and that is the honest shape rather than a
// convenience. A directory result is a live model carrying __x_-prefixed fields;
// a metadata query answers with mixins. They overlap but do not coincide —
// notably there is NO state on a directory result (__x_state measured undefined
// on 50 of 50), and verification is a BOOLEAN here against a string there.
// Reusing Channel would mean handing back a struct whose State and Verification
// are permanently empty, which reads as "this channel has no state" rather than
// as "this shape does not carry one".
type DirectoryEntry struct {
	// JID identifies the channel; its domain is "newsletter".
	JID string
	// Name and Description are public and still content: String renders neither.
	Name        string
	Description string
	// Subscribers is the follower count. Measured up to seven digits, so it is an
	// int and not something narrower.
	Subscribers int
	// Verified is the page's boolean. It is NOT mapped onto Channel.Verification's
	// vocabulary, because a value this package has not seen must not become one
	// it has.
	Verified bool
	// Membership is the page's own word for this account's relationship, e.g.
	// "guest". Carried verbatim.
	Membership string
	// CreatedAt is when the channel was made. Zero when absent.
	CreatedAt time.Time
}

func (d DirectoryEntry) String() string {
	return fmt.Sprintf("channel.DirectoryEntry(jid=%t name=%t desc=%t subscribers=%d verified=%t membership=%s)",
		d.JID != "", d.Name != "", d.Description != "", d.Subscribers, d.Verified, d.Membership)
}

// SearchOptions narrows a directory search.
//
// THERE IS NO LIMIT FIELD, and its absence is a decision rather than an
// omission: the reference implements one by monkey-patching a page function that
// does not exist on this build. See searchScript for the measurement.
type SearchOptions struct {
	// Query is the text to search for. Empty asks for the directory's own
	// recommendations, which is what the reference's default does.
	Query string
	// Region is an ISO country code. Empty lets the page use its own, which is
	// what a caller almost always wants and what the reference defaults to.
	Region string
	// SkipSubscribed leaves out channels this account already follows.
	SkipSubscribed bool
}

// Search asks the channel directory.
//
// IT NEEDS NO SUBSCRIPTION, which is what makes it provable here at all: this
// account follows nothing, and the neighbouring getRecommendedNewsletters is
// BLOCKED because it hangs. This one answered 50 results when measured.
//
// The count returned is the PAGE's, not a number this package chose.
func (r *Reader) Search(ctx context.Context, opts SearchOptions, label string) ([]DirectoryEntry, error) {
	key := nextStateKey()
	raw, err := r.parked(ctx, searchScript(opts.Query, opts.Region, opts.SkipSubscribed, key), key, label+"/search")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Results []struct {
			JID        string `json:"jid"`
			Name       string `json:"name"`
			Desc       string `json:"description"`
			Subs       int    `json:"subscribers"`
			Verified   bool   `json:"verified"`
			Membership string `json:"membership"`
			CreatedAt  int64  `json:"createdAt"`
		} `json:"results"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if !out.OK {
		return nil, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	// AN EMPTY DIRECTORY IS A LEGITIMATE ANSWER and is NOT an error: a search for
	// a term nobody used returns nothing, and turning that into a failure would
	// make the caller retry something that will never succeed.
	found := make([]DirectoryEntry, 0, len(out.Results))
	for _, res := range out.Results {
		entry := DirectoryEntry{
			JID: res.JID, Name: res.Name, Description: res.Desc,
			Subscribers: res.Subs, Verified: res.Verified, Membership: res.Membership,
		}
		if res.CreatedAt > 0 {
			entry.CreatedAt = time.Unix(res.CreatedAt, 0)
		}
		found = append(found, entry)
	}
	return found, nil
}

package channel

// The owner side: creating a channel, renaming it, describing it, deleting it.
//
// EVERY WRITE HERE IS VERIFIED BY READING THE CHANNEL BACK, and the oracle is
// ByInviteCode — already proven, and proven specifically against a channel this
// account does not follow, which is what makes it trustworthy as a postcondition
// rather than as a second opinion from the same code path.
//
// The reference returns a BOOLEAN from each of these, and for creation it
// returns an ERROR MESSAGE AS A STRING ('CreateChannelError: …') that the caller
// has to pattern-match. Both are silent failures under invariant 14.
//
// getSubscribers is deliberately absent: WAWebMexFetchNewsletterSubscribersJob,
// the module the reference uses, DOES NOT EXIST on this build (measured
// 2026-08-22). That row stays open in the ledger with the measurement rather
// than being filled with something that cannot work.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

var (
	// ErrNoName is an empty channel name.
	ErrNoName = fmt.Errorf("channel: no channel name given")
	// ErrNoJID is an empty channel jid.
	ErrNoJID = fmt.Errorf("channel: no channel jid given")
	// ErrCreationDisabled is the page refusing to create channels at all.
	ErrCreationDisabled = fmt.Errorf("channel: this account cannot create channels")
	// ErrWrite is the page refusing or failing a write.
	ErrWrite = fmt.Errorf("channel: the page refused the write")
	// ErrNotTaken is the postcondition: the page accepted and nothing changed.
	ErrNotTaken = fmt.Errorf("channel: the change did not take")
	// ErrStillThere is a delete the server accepted and did not perform.
	ErrStillThere = fmt.Errorf("channel: the channel is still readable after the delete")
)

// Created is a channel that now exists.
type Created struct {
	JID        string
	InviteCode string
	CreatedAt  time.Time
}

func (c Created) String() string {
	return fmt.Sprintf("channel.Created(jid=%t code=%t at=%t)",
		c.JID != "", c.InviteCode != "", !c.CreatedAt.IsZero())
}

// Manager owns channels.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
	reader *Reader
}

// NewManager builds one. It keeps a Reader because every postcondition here is
// a read.
func NewManager(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval, reader: New(runner, eval)}
}

// Create makes a channel and PROVES it exists by reading it back.
func (m *Manager) Create(ctx context.Context, name, description, label string) (Created, error) {
	if strings.TrimSpace(name) == "" {
		return Created{}, ErrNoName
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, createScript(name, description, key), key, label+"/create")
	if err != nil {
		return Created{}, fmt.Errorf("%w: %v", ErrWrite, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		Disabled bool   `json:"disabled"`
		JID      string `json:"jid"`
		Code     string `json:"code"`
		At       int64  `json:"at"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Created{}, fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if out.Disabled {
		return Created{}, ErrCreationDisabled
	}
	if !out.OK {
		return Created{}, fmt.Errorf("%w (%s)", ErrWrite, out.Why)
	}
	made := Created{JID: out.JID, InviteCode: out.Code}
	if out.At > 0 {
		made.CreatedAt = time.Unix(out.At, 0)
	}
	// THE POSTCONDITION. The reference hands back the values the create call
	// echoed; this asks the server, through the reader proven against a channel
	// this account does not follow.
	if made.InviteCode == "" {
		return made, fmt.Errorf("%w: the page reported a channel with no invite code, "+
			"so there is nothing to verify against", ErrNotTaken)
	}
	if _, err := m.reader.ByInviteCode(ctx, made.InviteCode, label+"/create-verify"); err != nil {
		return made, fmt.Errorf("%w: created but not readable back: %v", ErrNotTaken, err)
	}
	return made, nil
}

// SetName renames a channel and PROVES the new name is what the server reports.
func (m *Manager) SetName(ctx context.Context, jid, code, name, label string) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	return m.edit(ctx, jid, code, editName, name, label, func(c Channel) bool {
		return c.Name == name
	})
}

// SetDescription rewrites the description and PROVES it.
//
// An EMPTY description is allowed: clearing one is a legitimate operation, and
// refusing it would make this the only setting a caller cannot undo.
func (m *Manager) SetDescription(ctx context.Context, jid, code, desc, label string) error {
	return m.edit(ctx, jid, code, editDescription, desc, label, func(c Channel) bool {
		return c.Description == desc
	})
}

func (m *Manager) edit(ctx context.Context, jid, code, field, value, label string,
	took func(Channel) bool) error {
	if strings.TrimSpace(jid) == "" {
		return ErrNoJID
	}
	if strings.TrimSpace(code) == "" {
		return ErrNoCode
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, editScript(jid, field, value, key), key, label+"/edit")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrite, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrWrite, out.Why)
	}
	// READ IT BACK FROM THE SERVER. The reference returns true here having only
	// seen the call not throw.
	got, err := m.reader.ByInviteCode(ctx, code, label+"/edit-verify")
	if err != nil {
		return fmt.Errorf("%w: the write was accepted and the channel could not be "+
			"read back: %v", ErrNotTaken, err)
	}
	if !took(got) {
		return fmt.Errorf("%w: the page accepted the change and the server still "+
			"reports the old value", ErrNotTaken)
	}
	return nil
}

// Delete removes a channel and PROVES it is gone.
func (m *Manager) Delete(ctx context.Context, jid, code, label string) error {
	if strings.TrimSpace(jid) == "" {
		return ErrNoJID
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, deleteScript(jid, key), key, label+"/delete")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrite, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrWrite, out.Why)
	}
	// A DELETE WITHOUT A CODE CANNOT BE VERIFIED, and saying so is better than
	// reporting success. The caller keeps the code from Create for this reason.
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("%w: deleted, but no invite code was given so it could not "+
			"be checked", ErrNotTaken)
	}
	if _, err := m.reader.ByInviteCode(ctx, code, label+"/delete-verify"); err == nil {
		return ErrStillThere
	}
	return nil
}

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
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
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

var (
	// ErrOwnChannel is subscribing to a channel this account owns.
	//
	// ITS OWN ERROR because it is a caller mistake with a different repair, and
	// because the page's refusal for it is indistinguishable from a real failure.
	ErrOwnChannel = fmt.Errorf("channel: this account owns that channel")
	// ErrNotReachable is a channel the page could not fetch.
	ErrNotReachable = fmt.Errorf("channel: the page could not reach that channel")
)

// membershipGuest is what the page calls a non-member. Measured on directory
// results (H112) and used here as the postcondition for unfollowing.
const membershipGuest = "guest"

// Follow subscribes this account to a channel and PROVES the membership moved.
//
// The reference returns a bare boolean, and returns it after merely not
// throwing. Here the membership is read back: a page that accepted the call and
// left the account a guest is a failure, not a success (invariant 14).
func (m *Manager) Follow(ctx context.Context, jid, label string) (string, error) {
	return m.follow(ctx, jid, true, label)
}

// Unfollow reverses it, with the same postcondition in the other direction.
func (m *Manager) Unfollow(ctx context.Context, jid, label string) (string, error) {
	return m.follow(ctx, jid, false, label)
}

func (m *Manager) follow(ctx context.Context, jid string, subscribe bool, label string) (string, error) {
	if strings.TrimSpace(jid) == "" {
		return "", ErrNoJID
	}
	verb := "unfollow"
	if subscribe {
		verb = "follow"
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, followScript(jid, subscribe, key), key, label+"/"+verb)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWrite, err)
	}
	var out struct {
		OK         bool   `json:"ok"`
		Why        string `json:"why"`
		Owner      bool   `json:"owner"`
		Membership string `json:"membership"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return "", fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if out.Owner {
		return "", ErrOwnChannel
	}
	if !out.OK {
		if strings.Contains(out.Why, "not reachable") {
			// THE PAGE'S OWN REASON TRAVELS. Swallowing it cost a run: the first
			// version returned a bare ErrNotReachable and the probe could not tell
			// "no such channel" from "find rejected the argument shape".
			return "", fmt.Errorf("%w (%s)", ErrNotReachable, out.Why)
		}
		return "", fmt.Errorf("%w (%s)", ErrWrite, out.Why)
	}
	// THE POSTCONDITION. Following must leave a membership that is NOT guest;
	// unfollowing must leave one that IS.
	isGuest := out.Membership == membershipGuest || out.Membership == ""
	if subscribe && isGuest {
		return out.Membership, fmt.Errorf("%w: the page accepted the subscription and "+
			"the account is still a guest", ErrNotTaken)
	}
	if !subscribe && !isGuest {
		return out.Membership, fmt.Errorf("%w: the page accepted the unsubscription and "+
			"the membership is still %q", ErrNotTaken, out.Membership)
	}
	return out.Membership, nil
}

// Followed lists the channels this account follows.
//
// AN EMPTY LIST IS A LEGITIMATE ANSWER, and it is the answer this account gave
// for the whole of Phase 1 until something was followed — which is exactly why
// the ledger row for it sat open rather than being filled with a reader nobody
// had seen return anything (H93).
//
// IT READS THE CLIENT'S CACHE, NOT THE SERVER, and that is a measured caveat
// rather than an implementation detail (H139). When ANOTHER account deletes a
// channel this one is an admin or subscriber of, the local model STAYS: six such
// leftovers were found across the two lab accounts, every one of them reporting
// serverAlive:false when asked by invite code. A caller that treats this list as
// "what exists" will count channels that do not.
func (m *Manager) Followed(ctx context.Context, label string) ([]DirectoryEntry, error) {
	key := nextStateKey()
	raw, err := m.parked(ctx, followedScript(key), key, label+"/followed")
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
	found := make([]DirectoryEntry, 0, len(out.Results))
	for _, r := range out.Results {
		e := DirectoryEntry{
			JID: r.JID, Name: r.Name, Description: r.Desc, Subscribers: r.Subs,
			Verified: r.Verified, Membership: r.Membership,
		}
		if r.CreatedAt > 0 {
			e.CreatedAt = time.Unix(r.CreatedAt, 0)
		}
		found = append(found, e)
	}
	return found, nil
}

// ReactionPolicy is who may react to a channel's posts, in the REFERENCE's
// vocabulary — the three values its API accepts.
type ReactionPolicy int

const (
	// ReactionsAll lets anyone react with any emoji.
	ReactionsAll ReactionPolicy = 0
	// ReactionsBasic limits reactions to a basic set.
	ReactionsBasic ReactionPolicy = 1
	// ReactionsNone turns reactions off.
	ReactionsNone ReactionPolicy = 2
)

// reactionWire maps the reference's code to the number the page actually wants.
//
// THE TWO VOCABULARIES ARE NOT THE SAME, and the mapping is not ours: it is
// literally the reference's own table (Channel.js:186–190), where 0→3, 1→1 and
// 2→0. Carrying it here rather than inventing one keeps parity honest, and
// keeping it in ONE place is what stops a caller from passing a raw page number
// by accident.
var reactionWire = map[ReactionPolicy]int{
	ReactionsAll:   3,
	ReactionsBasic: 1,
	ReactionsNone:  0,
}

// ErrBadReactionPolicy is a value outside the three the reference accepts.
var ErrBadReactionPolicy = fmt.Errorf("channel: unknown reaction policy")

// ErrUnverifiable is a write whose postcondition has nothing to read.
//
// IT IS NOT A FAILURE OF THE WRITE. The page may well have stored the value; we
// simply cannot say. Reporting that as ErrNotTaken would claim knowledge nobody
// has, which is the opposite of what invariant 14 is for.
var ErrUnverifiable = fmt.Errorf("channel: the change cannot be verified on this channel")

// reactionAbsent is what the reader returns when the metadata carries no
// reaction mixin at all.
const reactionAbsent = -1

// SetReactionPolicy changes who may react, and PROVES the server took it.
//
// The reference returns a bare boolean computed from the absence of an
// exception. Here the metadata is read back and compared against the WIRE value,
// because that is what the server stores — comparing against the reference's
// code would compare two different vocabularies and pass by accident.
func (m *Manager) SetReactionPolicy(ctx context.Context, jid, code string,
	policy ReactionPolicy, label string) (int, error) {
	wire, ok := reactionWire[policy]
	if !ok {
		return -1, fmt.Errorf("%w: %d", ErrBadReactionPolicy, policy)
	}
	if strings.TrimSpace(jid) == "" {
		return -1, ErrNoJID
	}
	if strings.TrimSpace(code) == "" {
		return -1, ErrNoCode
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, reactionScript(jid, wire, key), key, label+"/reaction")
	if err != nil {
		return -1, fmt.Errorf("%w: %v", ErrWrite, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return -1, fmt.Errorf("channel: unexpected answer: %w", e)
	}
	if !out.OK {
		return -1, fmt.Errorf("%w (%s)", ErrWrite, out.Why)
	}
	got, err := m.reader.ByInviteCode(ctx, code, label+"/reaction-verify")
	if err != nil {
		return -1, fmt.Errorf("%w: the write was accepted and the channel could not "+
			"be read back: %v", ErrNotTaken, err)
	}
	// ABSENT AND WRONG ARE DIFFERENT ANSWERS, and conflating them is the mistake
	// this module has now made twice (H108 with acks, H126 with descriptions).
	// A freshly created channel's metadata does NOT carry the reaction mixin at
	// all — measured (H133) — so the postcondition cannot run there, and saying
	// "the server reports -1" as if it were a value would be reporting our own
	// sentinel back as the world's answer.
	if got.ReactionPolicyRaw == reactionAbsent {
		return got.ReactionPolicyRaw, fmt.Errorf("%w: the channel's metadata does not "+
			"carry a reaction setting, so the write cannot be verified", ErrUnverifiable)
	}
	if got.ReactionPolicyRaw != wire {
		return got.ReactionPolicyRaw, fmt.Errorf("%w: asked for wire value %d and the "+
			"server reports %d", ErrNotTaken, wire, got.ReactionPolicyRaw)
	}
	return got.ReactionPolicyRaw, nil
}

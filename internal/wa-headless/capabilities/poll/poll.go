// Package poll reads and casts votes on a poll.
//
// This module could already SEND a poll (H69) and could never see what anybody
// answered. That is the half a product actually needs: a poll nobody can read is
// a message with buttons.
//
// WHERE THE VOTES LIVE, AND WHY IT IS UNUSUAL. Every other read in this module
// goes through a collection. Votes do not: they sit in a SCHEMA TABLE, queried
// by the parent message's key —
// WAWebPollsVotesSchema.getTable().equals(['parentMsgKey'], key). That is a
// storage layer nothing else here touches, and it is the reference's route too.
//
// WHERE THE REFERENCE CANNOT BE FOLLOWED. whatsapp-web.js builds the key with
// MsgKey.fromString(msg.id._serialized). On this build that throws
// "MsgKey.fromString error: str is null or not a string" — the same
// null-identifier fact that shaped capabilities/messagemeta.
//
// The message's `id` IS ALREADY A KEY, and its toString gives the 47-character
// form the table indexes on. So the conversion the reference needs is one this
// module skips: use what the page holds instead of rebuilding it from a string
// that is not there (H97). Same shape as H34.
package poll

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

var (
	// ErrNoMessage is an empty message id.
	ErrNoMessage = fmt.Errorf("poll: no message id given")
	// ErrNotFound is a message this session has not LOADED.
	//
	// IT IS NOT "THAT MESSAGE DOES NOT EXIST", and the difference is the whole
	// contract. MsgCollection holds what the session has looked at, not
	// everything the account has: a session that booted and never opened a
	// conversation has none of its messages. The first live round trip died
	// here — conta-A found its own poll instantly and conta-B could not find it
	// at all, because conta-B had never looked at that chat.
	//
	// LOADING IS THE CALLER'S JOB, deliberately. The alternative was a scan that
	// loads every chat until the message turns up, which on the lab account
	// means nine hundred loads for one lookup and is a heuristic wearing a
	// convenience's clothes. capabilities/fetchmessages loads a chat; this
	// package reads a poll.
	ErrNotFound = fmt.Errorf("poll: this session has not loaded that message")
	// ErrNotAPoll is a message that is not a poll. It is separate from
	// ErrNotFound because the repairs differ: one is a stale id and the other
	// is a caller pointing at the wrong thing.
	ErrNotAPoll = fmt.Errorf("poll: that message is not a poll")
	// ErrRead is the page refusing or failing the vote read.
	ErrRead = fmt.Errorf("poll: the page refused the vote read")
	// ErrVote is the page refusing or failing the vote.
	ErrVote = fmt.Errorf("poll: the page refused the vote")
	// ErrUnknownOption is an answer that is not on the poll. The page would
	// silently send an empty selection, which reads as "voted for nothing".
	ErrUnknownOption = fmt.Errorf("poll: that option is not on this poll")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKeyPrefix = "__waHeadlessPoll"

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Tally is who voted for what.
//
// IT COUNTS BY OPTION AND NEVER NAMES A VOTER. A poll's voters are identities
// and its options are content; what crosses this boundary is the option's local
// id and how many chose it. A caller that needs the names has the raw ids in
// Voters, which is a list of jids carried for routing and never rendered.
type Tally struct {
	// MessageID is the poll this tallies.
	MessageID string
	// Options maps a poll option's local id to how many voters chose it.
	// Options with no votes are present with zero, so a caller can tell an
	// unvoted option from one this build did not report.
	Options map[int]int
	// Voters is how many distinct voters answered at all.
	Voters int
	// Rows is how many vote records the table held, which is not the same as
	// Voters: a voter who changes their mind leaves more than one.
	Rows int
}

func (t Tally) String() string {
	return fmt.Sprintf("poll.Tally(msg=%t options=%d voters=%d rows=%d)",
		t.MessageID != "", len(t.Options), t.Voters, t.Rows)
}

// Manager reads and casts votes.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// Votes reports what a poll has collected.
func (m *Manager) Votes(ctx context.Context, messageID, label string) (Tally, error) {
	if strings.TrimSpace(messageID) == "" {
		return Tally{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, votesScript(messageID, key), key, label+"/votes")
	if err != nil {
		return Tally{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK      bool           `json:"ok"`
		Why     string         `json:"why"`
		Options map[string]int `json:"options"`
		Voters  int            `json:"voters"`
		Rows    int            `json:"rows"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Tally{}, fmt.Errorf("poll: unexpected votes answer: %w", e)
	}
	if !out.OK {
		return Tally{}, classify(out.Why, ErrRead)
	}
	t := Tally{MessageID: messageID, Options: map[int]int{}, Voters: out.Voters, Rows: out.Rows}
	for k, v := range out.Options {
		var id int
		if _, e := fmt.Sscanf(k, "%d", &id); e == nil {
			t.Options[id] = v
		}
	}
	return t, nil
}

// Vote answers a poll with the named options.
//
// THE NAMES ARE MAPPED IN THE PAGE, and refused there when they do not match.
// The page's own send takes a set of LOCAL IDS, and an unmatched name would
// produce an empty set — a vote for nothing, sent successfully.
func (m *Manager) Vote(ctx context.Context, messageID string, options []string, label string) error {
	if strings.TrimSpace(messageID) == "" {
		return ErrNoMessage
	}
	clean := make([]string, 0, len(options))
	for _, o := range options {
		if s := strings.TrimSpace(o); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		// AN EMPTY VOTE IS A REAL OPERATION UPSTREAM — it withdraws a previous
		// answer — but it is indistinguishable from a caller whose option names
		// all failed to match. Until withdrawing has its own method, this
		// refuses rather than guessing which was meant.
		return fmt.Errorf("%w: no options given; withdrawing a vote is not implemented", ErrUnknownOption)
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, voteScript(messageID, clean, key), key, label+"/vote")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVote, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Matched int    `json:"matched"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("poll: unexpected vote answer: %w", e)
	}
	if !out.OK {
		return classify(out.Why, ErrVote)
	}
	if out.Matched != len(clean) {
		return fmt.Errorf("%w: %d of %d option(s) matched", ErrUnknownOption, out.Matched, len(clean))
	}
	return nil
}

// classify turns the page's reason into this package's vocabulary, so a caller
// can react without parsing strings.
func classify(why string, fallback error) error {
	switch why {
	case "NO_MESSAGE":
		return ErrNotFound
	case "NOT_A_POLL":
		return ErrNotAPoll
	case "NO_OPTION_MATCHED":
		return ErrUnknownOption
	}
	return fmt.Errorf("%w (%s)", fallback, why)
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

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

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
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
	raw, err := m.parked(ctx, createScript(name, description), label+"/create")
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
	raw, err := m.parked(ctx, editScript(jid, field, value), label+"/edit")
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
	raw, err := m.parked(ctx, deleteScript(jid), label+"/delete")
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
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
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

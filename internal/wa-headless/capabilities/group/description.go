package group

// Setting a group's description, and proving the server took it.
//
// IT IS NOT THE SAME CALL AS THE SUBJECT. The subject goes through
// setGroupSubject; the description goes through setGroupDescription, which takes
// FOUR arguments — the group wid, the text, a NEW message key, and the id of the
// description being replaced. That fourth argument is the part a caller cannot
// invent: it comes from the group's own metadata and is undefined the first time
// (measured on the lab group, H126).
//
// THE POSTCONDITION IS A READ, not the call not throwing. The reference returns
// a boolean it computes from the absence of an exception, which is the silent
// success invariant 14 forbids — and this module has already caught a page that
// accepts a description and never stores it (H113, on channels).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
)

var (
	// ErrDescriptionNotTaken is the postcondition: the page accepted the change
	// and the server still reports the old text.
	ErrDescriptionNotTaken = fmt.Errorf("group: the description did not take")
	// ErrDescribe is the page refusing or failing.
	ErrDescribe = fmt.Errorf("group: the page refused the description change")
)

// MaxDescriptionBytes is a ceiling this module imposes so that a caller's
// mistake is refused here rather than becoming a server round trip. It is a
// BORROWED number — WhatsApp's own documented limit — and is written as such
// rather than presented as something measured here.
const MaxDescriptionBytes = 2048

// Described is what a description change did.
type Described struct {
	// Text is what the server reports AFTER the change, read back.
	Text string
	// Source names the field the text was read from, because this build keeps it
	// in two places and which one answered is worth knowing (H105).
	Source string
	// Took is how long the round trip plus the verification cost.
	Took time.Duration
}

func (d Described) String() string {
	return fmt.Sprintf("group.Described(text=%t source=%s took=%s)",
		d.Text != "", d.Source, d.Took.Round(time.Millisecond))
}

// SetDescription rewrites a group's description and PROVES the server took it.
//
// An EMPTY description is allowed: clearing one is a legitimate act, and
// refusing it would make this the only group setting a caller cannot undo.
func (m *Manager) SetDescription(ctx context.Context, groupJID, description, label string) (Described, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return Described{}, ErrNotGroup
	}
	if len(description) > MaxDescriptionBytes {
		return Described{}, fmt.Errorf("%w (%d bytes, ceiling %d)",
			ErrDescriptionNotTaken, len(description), MaxDescriptionBytes)
	}
	start := time.Now()
	raw, err := m.parkedDescription(ctx, describeScript(groupJID, description), label+"/describe")
	if err != nil {
		return Described{}, fmt.Errorf("%w: %v", ErrDescribe, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Described{}, fmt.Errorf("group: unexpected answer: %w", e)
	}
	if !out.OK {
		return Described{}, fmt.Errorf("%w (%s)", ErrDescribe, out.Why)
	}
	// READ IT BACK THROUGH THE PROVEN READER, which knows that this build keeps
	// the description in two fields and reports which one answered (H105).
	md, err := m.Metadata(ctx, groupJID, label+"/describe-verify")
	if err != nil {
		return Described{}, fmt.Errorf("%w: the write was accepted and the group could "+
			"not be read back: %v", ErrDescriptionNotTaken, err)
	}
	got := Described{Text: md.Description, Source: md.DescriptionSource, Took: time.Since(start)}
	if md.Description != description {
		return got, fmt.Errorf("%w: asked for %d bytes and the server reports %d",
			ErrDescriptionNotTaken, len(description), len(md.Description))
	}
	return got, nil
}

func (m *Manager) parkedDescription(ctx context.Context, kick, label string) (string, error) {
	var started string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return m.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(describeBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
			return m.eval(c, `window.`+describeKey+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", describeBudget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(describeTick):
		}
	}
}

// Budgets. Var so a test can compress them.
var (
	describeBudget = 30 * time.Second
	describeTick   = 250 * time.Millisecond
)

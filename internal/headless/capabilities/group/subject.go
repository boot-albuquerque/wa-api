package group

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// MaxSubjectBytes is this package's ceiling. WhatsApp's own is smaller, and the
// page will refuse first; this exists so an accidental novel is refused here
// with a message that says what happened.
const MaxSubjectBytes = 512

var (
	// ErrSubject is the page refusing or throwing a rename.
	ErrSubject = fmt.Errorf("group: the page refused the rename")
	// ErrEmptySubject is a caller trying to clear a group's name. The app's own
	// signature defaults the subject to "", which would do exactly that; this
	// package will not do it by accident.
	ErrEmptySubject = fmt.Errorf("group: empty subject; clearing a group's name is not what rename means")
	// ErrSubjectTooLong is the ceiling above.
	ErrSubjectTooLong = fmt.Errorf("group: subject is longer than this package will send")
	// ErrSubjectUnchanged is the postcondition.
	ErrSubjectUnchanged = fmt.Errorf("group: the page accepted the rename and the subject did not change")
)

// Rename is what a rename did. It carries LENGTHS rather than the names: a
// group's subject is content chosen by people, and this module reports shapes.
type Rename struct {
	FromLen, ToLen int
	// Field names WHICH model field carried the change. It is reported because
	// this build was never asked before, and a capability that cannot say where
	// it looked cannot be checked by the next reader.
	Field string
	// AlreadyInState is true when the group already had that name.
	AlreadyInState bool
	Waited         time.Duration
}

// Changed reports whether the subject moved.
func (r Rename) Changed() bool { return !r.AlreadyInState }

func (r Rename) String() string {
	return fmt.Sprintf("group.Rename(fromLen=%d toLen=%d field=%s already=%t waited=%s)",
		r.FromLen, r.ToLen, r.Field, r.AlreadyInState, r.Waited.Round(time.Millisecond))
}

const subjectStateKey = "__headlessGroupSubject"

// SetSubject renames a group.
//
// IT DOES NOT CHECK FOR ADMIN. Unlike adding participants, renaming is governed
// by a per-group setting that may allow every member, and refusing here on a
// guess would deny an act the group permits. The page's own refusal is passed
// through instead.
func (m *Manager) SetSubject(ctx context.Context, groupJID, subject, label string) (Rename, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return Rename{}, ErrNotGroup
	}
	if strings.TrimSpace(subject) == "" {
		return Rename{}, ErrEmptySubject
	}
	if len(subject) > MaxSubjectBytes {
		return Rename{}, fmt.Errorf("%w (%d bytes, ceiling %d)", ErrSubjectTooLong, len(subject), MaxSubjectBytes)
	}
	start := time.Now()

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, subjectScript(groupJID, subject), &kicked)
	}); err != nil {
		return Rename{}, fmt.Errorf("%w: %v", ErrSubject, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		FromLen int    `json:"fromLen"`
		ToLen   int    `json:"toLen"`
		Field   string `json:"field"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, subjectResultScript, &raw)
		}); err != nil {
			return Rename{}, fmt.Errorf("%w: %v", ErrSubject, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Rename{}, fmt.Errorf("group: unexpected answer to rename: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			if out.Stage == "settling" {
				return Rename{}, fmt.Errorf("%w within %s", ErrSubjectUnchanged, createBudget)
			}
			return Rename{}, fmt.Errorf("%w: the page never settled within %s", ErrSubject, createBudget)
		}
		time.Sleep(createTick)
	}

	switch {
	case out.Why == "NO_CHAT":
		return Rename{}, ErrNotGroup
	case !out.OK:
		return Rename{}, fmt.Errorf("%w at %s (%s)", ErrSubject, out.Stage, out.Why)
	}
	return Rename{FromLen: out.FromLen, ToLen: out.ToLen, Field: out.Field,
		AlreadyInState: out.Already, Waited: time.Since(start)}, nil
}

func subjectScript(groupJID, subject string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(subjectStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(subjectStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const want = ` + strconv.Quote(subject) + `;
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(groupJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			// WHICH FIELD holds a group's subject on this build has never been
			// measured, so the candidates are named once here and the one that
			// moves is REPORTED rather than assumed.
			const read = (c) => {
				for (const f of ['subject', 'name', 'formattedTitle']) {
					const v = c && c[f];
					if (typeof v === 'string' && v.length) { return { field: f, value: v }; }
				}
				return { field: '', value: '' };
			};
			const before = read(chat);
			if (before.value === want) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					fromLen: before.value.length, toLen: before.value.length,
					field: before.field });
				return;
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleSetSubjectGroupAction) + `');
			// TWO POSITIONAL, and the subject is passed explicitly: the app's
			// own default is the empty string, which would clear the name.
			await A.setGroupSubject(chat, want);

			stage = 'verify';
			park({ stage: 'settling', ok: true, why: '', already: false,
				fromLen: before.value.length, want: want, chat: chat });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// subjectResultScript reads the candidate fields off the parked chat and never
// serialises it.
const subjectResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + subjectStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		for (const f of ['subject', 'name', 'formattedTitle']) {
			const v = s.chat && s.chat[f];
			if (v === s.want) {
				return { stage: 'done', ok: true, why: '', already: false,
					fromLen: s.fromLen, toLen: v.length, field: f };
			}
		}
		return { stage: 'settling', ok: false, why: '', fromLen: s.fromLen };
	}
	return s;
})())`

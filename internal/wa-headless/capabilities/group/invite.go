package group

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// The invite code.
//
// AN INVITE CODE IS A CREDENTIAL. Anyone holding it can join the group, so it
// is treated like the QR code and the profile-picture url: carried in the
// value, never rendered by String(), never logged.
//
// THE METADATA HAS TO BE QUERIED FIRST, and that is the finding.
// queryGroupInviteCode reads iAmAdmin off the group's metadata, and the
// metadata row exists WITHOUT it — the call throws "Cannot read properties of
// undefined (reading 'iAmAdmin')" until WAWebGroupQueryJob's
// queryAndUpdateGroupMetadataById has filled it in. Fifth appearance of that
// error shape, and the first where the fix was not a different argument but a
// missing preparation step.

var (
	// ErrInvite is the page refusing or throwing.
	ErrInvite = fmt.Errorf("group: the page refused the invite operation")
	// ErrNotGroup is a jid that is not a group.
	ErrNotGroup = fmt.Errorf("group: not a group")
	// ErrNotAdmin is this account lacking the right to see or change the
	// invite. It is its own error because it is not a failure of the call — it
	// is an answer about who the account is in that group.
	ErrNotAdmin = fmt.Errorf("group: this account is not an admin of that group")
)

// Invite is a group's invite code.
type Invite struct {
	// Code is the credential. It is never rendered.
	Code string
	// Revoked says the code was replaced by this call — the previous one stops
	// working, and anyone holding it is locked out.
	Revoked bool
}

// Link builds the joinable url. It is a method rather than a stored field so
// that the code and its url never diverge, and so a caller has to ask for the
// dangerous form explicitly.
func (i Invite) Link() string {
	if i.Code == "" {
		return ""
	}
	return "https://chat.whatsapp.com/" + i.Code
}

// String redacts. A code in a log is a group anyone who reads the log can join.
func (i Invite) String() string {
	return fmt.Sprintf("group.Invite(code=%t len=%d revoked=%t)", i.Code != "", len(i.Code), i.Revoked)
}

const inviteStateKey = "__waHeadlessGroupInvite"

// InviteCode returns the group's current invite code.
//
// It queries the group's metadata first, because the page needs iAmAdmin to
// answer and does not populate it on its own.
func (m *Manager) InviteCode(ctx context.Context, groupJID, label string) (Invite, error) {
	return m.invite(ctx, groupJID, false, label)
}

// RevokeInvite replaces the code, locking out everyone holding the old one.
//
// It is separate from InviteCode rather than a flag because it is destructive
// in a way reading is not: the previous link stops working for people who may
// be relying on it.
func (m *Manager) RevokeInvite(ctx context.Context, groupJID, label string) (Invite, error) {
	return m.invite(ctx, groupJID, true, label)
}

func (m *Manager) invite(ctx context.Context, groupJID string, revoke bool, label string) (Invite, error) {
	if strings.TrimSpace(groupJID) == "" {
		return Invite{}, ErrNotGroup
	}
	if !strings.HasSuffix(groupJID, "@g.us") {
		return Invite{}, fmt.Errorf("%w: %s does not end in @g.us", ErrNotGroup, "the jid")
	}

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, inviteScript(groupJID, revoke), &kicked)
	}); err != nil {
		return Invite{}, fmt.Errorf("%w: %v", ErrInvite, err)
	}

	var out struct {
		Stage string `json:"stage"`
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		Code  string `json:"code"`
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, inviteResultScript, &raw)
		}); err != nil {
			return Invite{}, fmt.Errorf("%w: %v", ErrInvite, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Invite{}, fmt.Errorf("group: unexpected invite answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Invite{}, fmt.Errorf("%w: the page never settled within %s", ErrInvite, createBudget)
		}
		time.Sleep(createTick)
	}
	switch {
	case out.Why == "NOT_ADMIN":
		return Invite{}, ErrNotAdmin
	case out.Stage == "find" && !out.OK:
		return Invite{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case !out.OK:
		return Invite{}, fmt.Errorf("%w at %s (%s)", ErrInvite, out.Stage, out.Why)
	}
	if out.Code == "" {
		return Invite{}, fmt.Errorf("%w: the page reported success with no code", ErrInvite)
	}
	return Invite{Code: out.Code, Revoked: revoke}, nil
}

func inviteScript(groupJID string, revoke bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(inviteStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(inviteStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const WidFactory = window.require('` + string(spa.ModuleWidFactory) + `');
			const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const wid = WidFactory.createWid(` + strconv.Quote(groupJID) + `);
			if (!wid) { park({ stage, ok: false, why: 'WID_NULL' }); return; }
			const chat = Chats.get(wid);
			if (!chat) { park({ stage, ok: false, why: 'NO_GROUP' }); return; }

			stage = 'metadata';
			// WITHOUT THIS THE NEXT CALL THROWS. queryGroupInviteCode reads
			// iAmAdmin off the group's metadata, and the metadata row exists
			// without it until this job fills it in.
			try {
				const Job = window.require('` + string(spa.ModuleGroupQueryJob) + `');
				// "ById": the argument is the id, and passing the WID threw
				// "Cannot read properties of undefined (reading 'toString')" —
				// something inside reaches for a field the wid does not carry.
				// Both shapes are tried because the name says id and the sibling
				// calls in this file take chats.
				try { await Job.queryAndUpdateGroupMetadataById(chat.id); }
				catch (e1) { await Job.queryAndUpdateGroupMetadataById(chat); }
			} catch (e) {
				park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 120) });
				return;
			}

			// Only an admin may see or change the invite. Asking anyway is
			// asking for a refusal the page can already predict.
			const md = chat.groupMetadata;
			if (md && md.iAmAdmin === false) { park({ stage, ok: false, why: 'NOT_ADMIN' }); return; }

			stage = ` + strconv.Quote(map[bool]string{true: "revoke", false: "query"}[revoke]) + `;
			const A = window.require('` + string(spa.ModuleGroupInviteAction) + `');
			// THE CHAT, not the wid — it is the chat whose metadata carries
			// iAmAdmin.
			const code = ` + map[bool]string{
		true:  "await A.revokeGroupInvite(chat)",
		false: "await A.queryGroupInviteCode(chat)",
	}[revoke] + `;
			const asString = (typeof code === 'string') ? code
				: (code && typeof code.code === 'string' ? code.code : '');
			park({ stage: 'done', ok: true, why: '', code: asString });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const inviteResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + inviteStateKey + `"` + `];
	if (!s) { return { stage: 'query', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

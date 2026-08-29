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
	// ErrNoCode is the call settling with no code on the model. It is distinct
	// from ErrInvite because the repairs differ: a page that hung is a page
	// problem, and a code that never landed is a group or permission one.
	ErrNoCode = fmt.Errorf("group: the invite query settled and no code landed on the group")
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

const inviteStateKey = "__headlessGroupInvite"

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
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			// A settling stage that ran out of budget is a code that never
			// landed on the model, not a page that hung — and this entry has
			// already spent a day on the difference.
			if out.Stage == "settling" {
				return Invite{}, fmt.Errorf("%w within %s", ErrNoCode, createBudget)
			}
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
		// MEASURED, NOT SPECULATED. With the metadata the call does not throw
		// and returns undefined, which means the group has no CACHED code. The
		// fetch that would populate it — WAWebGroupQueryJob.queryGroupInvite —
		// does not return on this build: a probe calling it never settled in 90
		// seconds.
		//
		// So the error says where the next attempt starts rather than blaming
		// the caller for asking.
		return Invite{}, fmt.Errorf("%w: the group has no cached invite code, and the "+
			"fetch that would populate it does not return on this build (H57)", ErrInvite)
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
			// THE METADATA IS THE ARGUMENT, not the chat and not the wid.
			//
			// Measured across four attempts: chat and wid both throw "Cannot
			// read properties of undefined (reading 'iAmAdmin')" — and that
			// error does NOT mean a missing field. iAmAdmin is a METHOD on the
			// participants collection:
			//
			//	chat.iAmAdmin = function(){ return this.groupMetadata
			//	    ? this.groupMetadata.participants.iAmAdmin() : false }
			//
			// so the invite call's body is participants.iAmAdmin(), and handing
			// it a chat makes it look for participants on a chat. With the
			// metadata it does not throw.
			//
			// The metadata query job is NOT called here: measured, it throws on
			// its own argument, and the participants were already present and
			// iAmAdmin() already answered true without it.
			const md = chat.groupMetadata;
			if (!md) { park({ stage, ok: false, why: 'NO_METADATA' }); return; }

			// ADMIN IS A METHOD. Asking it correctly turns a refusal the page
			// would produce anyway into an answer about who this account is.
			try {
				if (md.participants && typeof md.participants.iAmAdmin === 'function'
					&& md.participants.iAmAdmin() === false) {
					park({ stage, ok: false, why: 'NOT_ADMIN' });
					return;
				}
			} catch (e) { /* the page still decides */ }

			stage = ` + strconv.Quote(map[bool]string{true: "revoke", false: "query"}[revoke]) + `;
			const A = window.require('` + string(spa.ModuleGroupInviteAction) + `');
			// THE RETURN VALUE IS NOT THE CODE. Measured: the call settles and
			// resolves to undefined, and the code lands on the MODEL. Reading
			// the return is what made this look like it never produced anything
			// — and, before that, like it hung.
			` + map[bool]string{
		true:  "await A.revokeGroupInvite(md);",
		false: "await A.queryGroupInviteCode(md);",
	}[revoke] + `

			stage = 'read';
			// WHERE THE CODE LANDS was measured on both the metadata and the
			// chat, because it was never established which owns it; the first
			// string wins and both are tried on every read.
			const readCode = () => {
				for (const obj of [md, chat]) {
					try {
						const v = obj && obj.inviteCode;
						if (typeof v === 'string' && v.length) { return v; }
					} catch (e) {}
				}
				return '';
			};
			const code = readCode();
			if (code) {
				park({ stage: 'done', ok: true, why: '', code: code });
			} else {
				// THE MODEL IS PARKED and Go polls it: the await settles before
				// the field lands, which is the same lesson as H61 and the third
				// capability in this module to need it.
				park({ stage: 'settling', ok: true, why: '', gjid: ` + strconv.Quote(groupJID) + ` });
			}
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// inviteResultScript re-reads the code off the live models each round rather
// than holding a value the kick captured too early.
const inviteResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + inviteStateKey + `"` + `];
	if (!s) { return { stage: 'query', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		try {
			const W = window.require('WAWebWidFactory');
			const C = window.require('WAWebChatCollection').ChatCollection;
			const chat = C.get(W.createWid(s.gjid));
			for (const obj of [chat && chat.groupMetadata, chat]) {
				const v = obj && obj.inviteCode;
				if (typeof v === 'string' && v.length) {
					return { stage: 'done', ok: true, why: '', code: v };
				}
			}
		} catch (e) {}
		return { stage: 'settling', ok: false, why: '' };
	}
	return s;
})())`

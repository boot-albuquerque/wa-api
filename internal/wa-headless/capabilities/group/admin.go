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

// Promoting and demoting group admins, and leaving a group.
//
// A THIRD CALL SHAPE IN THE SAME MODULE. The participant jobs now have three
// different signatures between them, all exported side by side:
//
//	addParticipantsJob({group, participants, isOffline, reason})       // 1 object
//	removeParticipantsJob(group, participants, timestamp, author,
//	                      reason, groupMetadata, isOffline)            // 7 positional
//	promoteParticipantsJob(group, participants, groupMetadata,
//	                       isOffline)                                  // 4 positional
//
// The promote body is legible because it is synchronous, and it shows the
// object it assembles: {group, participants, groupMetadata, isOffline}. Three
// shapes for four sibling operations is no longer a surprise in this codebase —
// ARMADILHAS.md carries it as a property of this build.
//
// PARTICIPANTS ARE RECORDS HERE TOO, for the same reason and with the same
// fallback. That was the blocker H58 spent two blind attempts on.
//
// VERIFIED IS FALSE, for the same measured reason as membership: this build
// shows the session that made an admin change nothing to observe.

var (
	// ErrAdminChange is the page refusing or throwing a promote or demote.
	ErrAdminChange = fmt.Errorf("group: the page refused the admin change")
	// ErrLeave is the page refusing to leave.
	ErrLeave = fmt.Errorf("group: the page refused to leave")
	// ErrNotAMember is a group this account is not in.
	ErrNotAMember = fmt.Errorf("group: this account is not a member of that group")
)

// AdminChange is what a promote or demote did.
type AdminChange struct {
	// Promoted says which direction was asked for.
	Promoted bool
	// NoOp is true when the participant already held that role.
	NoOp bool
	// Verified is true only for a no-op — see Membership.Verified.
	Verified bool
	Waited   time.Duration
}

func (a AdminChange) String() string {
	return fmt.Sprintf("group.AdminChange(promoted=%t noop=%t verified=%t waited=%s)",
		a.Promoted, a.NoOp, a.Verified, a.Waited.Round(time.Millisecond))
}

const adminStateKey = "__waHeadlessGroupAdmin"

// Promote makes a participant an admin.
func (m *Manager) Promote(ctx context.Context, groupJID, participantJID, label string) (AdminChange, error) {
	return m.setAdmin(ctx, groupJID, participantJID, true, label)
}

// Demote takes it away.
func (m *Manager) Demote(ctx context.Context, groupJID, participantJID, label string) (AdminChange, error) {
	return m.setAdmin(ctx, groupJID, participantJID, false, label)
}

func (m *Manager) setAdmin(ctx context.Context, groupJID, participantJID string, promote bool, label string) (AdminChange, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return AdminChange{}, ErrNotGroup
	}
	if strings.TrimSpace(participantJID) == "" {
		return AdminChange{}, ErrNoParticipantGiven
	}
	start := time.Now()

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, adminScript(groupJID, participantJID, promote), &kicked)
	}); err != nil {
		return AdminChange{}, fmt.Errorf("%w: %v", ErrAdminChange, err)
	}

	out, err := m.awaitAdmin(ctx, label)
	if err != nil {
		return AdminChange{}, err
	}
	switch {
	case out.Why == "NOT_ADMIN":
		return AdminChange{}, ErrNotAdmin
	case out.Why == "NOT_A_MEMBER":
		return AdminChange{}, ErrNoParticipantGiven
	case out.Why == "ALREADY":
		return AdminChange{Promoted: promote, NoOp: true, Verified: true,
			Waited: time.Since(start)}, nil
	case out.Stage == "find" && !out.OK:
		return AdminChange{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case !out.OK:
		return AdminChange{}, fmt.Errorf("%w at %s (%s)", ErrAdminChange, out.Stage, out.Why)
	}
	return AdminChange{Promoted: promote, NoOp: false, Verified: false,
		Waited: time.Since(start)}, nil
}

// Leave exits a group.
//
// THERE IS NO LIVE PROOF OF THIS ONE, and the omission is deliberate rather
// than an oversight: an account that leaves a group it created cannot rejoin
// without an invite from somebody still inside, and the lab has two accounts.
// Running it once would cost the group every other live group test depends on.
func (m *Manager) Leave(ctx context.Context, groupJID, label string) error {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return ErrNotGroup
	}
	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, leaveScript(groupJID), &kicked)
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLeave, err)
	}
	out, err := m.awaitAdmin(ctx, label)
	if err != nil {
		return err
	}
	switch {
	case out.Why == "NOT_A_MEMBER":
		return ErrNotAMember
	case out.Stage == "find" && !out.OK:
		return fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case !out.OK:
		return fmt.Errorf("%w at %s (%s)", ErrLeave, out.Stage, out.Why)
	}
	return nil
}

type adminOut struct {
	Stage string `json:"stage"`
	OK    bool   `json:"ok"`
	Why   string `json:"why"`
}

func (m *Manager) awaitAdmin(ctx context.Context, label string) (adminOut, error) {
	var out adminOut
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, adminResultScript, &raw)
		}); err != nil {
			return out, fmt.Errorf("%w: %v", ErrAdminChange, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return out, fmt.Errorf("group: unexpected admin answer: %w", err)
		}
		if out.Stage != "pending" {
			return out, nil
		}
		if !time.Now().Before(deadline) {
			return out, fmt.Errorf("%w: the page never settled within %s", ErrAdminChange, createBudget)
		}
		time.Sleep(createTick)
	}
}

func adminScript(groupJID, participantJID string, promote bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(adminStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(adminStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const promote = ` + strconv.FormatBool(promote) + `;
			const W = window.require('` + string(spa.ModuleWidFactory) + `');
			const C = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const gwid = W.createWid(` + strconv.Quote(groupJID) + `);
			if (!gwid) { park({ stage, ok: false, why: 'WID_NULL' }); return; }
			const chat = C.get(gwid);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }
			const md = chat.groupMetadata;
			if (!md) { park({ stage, ok: false, why: 'NO_METADATA' }); return; }
			if (md.participants.iAmAdmin && !md.participants.iAmAdmin()) {
				park({ stage, ok: false, why: 'NOT_ADMIN' }); return;
			}

			const r = await (` + spa.ResolveIdentityExpr + `)(` + strconv.Quote(participantJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			const list = () => md.participants.getModelsArray
				? md.participants.getModelsArray() : (md.participants || []);
			let found = null;
			for (const p of list()) {
				try { if (p.id && p.id._serialized === r.jid) { found = p; break; } } catch (e) {}
			}
			if (!found) { park({ stage, ok: false, why: 'NOT_A_MEMBER' }); return; }
			if (!!found.isAdmin === promote) {
				park({ stage: 'done', ok: true, why: 'ALREADY' }); return;
			}

			stage = 'apply';
			const J = window.require('` + string(spa.ModuleGroupParticipantsJob) + `');
			// FOUR POSITIONAL — a THIRD shape in this module, read from the
			// synchronous body: {group, participants, groupMetadata, isOffline}.
			// The participant is the RECORD from the metadata, which is the fix
			// H58 paid two blind attempts for.
			if (promote) { await J.promoteParticipantsJob(gwid, [found], md, false); }
			else { await J.demoteParticipantsJob(gwid, [found], md, false); }

			// NO POSTCONDITION, for the reason measured in H58: this build shows
			// the session that made the change nothing.
			park({ stage: 'done', ok: true, why: '' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func leaveScript(groupJID string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(adminStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(adminStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const W = window.require('` + string(spa.ModuleWidFactory) + `');
			const C = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = C.get(W.createWid(` + strconv.Quote(groupJID) + `));
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }
			const md = chat.groupMetadata;
			// LEAVING A GROUP THIS ACCOUNT IS NOT IN would be a no-op that reads
			// like a success, and the caller would believe it had left something
			// it never joined.
			if (md && md.participants && md.participants.iAmMember && !md.participants.iAmMember()) {
				park({ stage, ok: false, why: 'NOT_A_MEMBER' }); return;
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleExitGroupAction) + `');
			// ONE ARGUMENT, and it is the CHAT MODEL: the body unproxies it.
			await A.sendExitGroup(chat);
			park({ stage: 'done', ok: true, why: '' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const adminResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + adminStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

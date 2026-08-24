package group

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// Joining a group from the OTHER side of the link.
//
// InviteCode and RevokeInvite answer "what is my group's link". These two
// answer "what is behind somebody else's link" and "take me in", which is the
// half of the invite family this module did not have — and without it the
// membership-request family cannot be proven at all: a pending request only
// exists because somebody followed a link into a group that asks for approval.

var (
	// ErrJoin is the page refusing or failing the join.
	ErrJoin = fmt.Errorf("group: the page refused to join by invite")
	// ErrNoInviteCode is an empty code.
	ErrNoInviteCode = fmt.Errorf("group: no invite code given")
	// ErrJoinPending is a join that the group turned into a REQUEST because it
	// requires admin approval.
	//
	// IT IS NOT AN ERROR IN THE CALLER'S SENSE and it is deliberately not
	// reported as success either. The account is not in the group, so a caller
	// that treats "joined" as "can now send" would be wrong; but nothing failed,
	// so a caller that retries would be wrong too. Naming the state is the only
	// answer that leads to the right reaction, which is to wait.
	ErrJoinPending = fmt.Errorf("group: the group requires approval; the join became a pending request")
)

// Joined is the outcome of following an invite.
type Joined struct {
	// GroupJID is the group entered. Empty when the join became a request.
	GroupJID string
	// Pending says the group asks for admin approval and this became a
	// membership request instead of a membership.
	Pending bool
	// AnswerKind and AnswerKeys describe what the page handed back, in NAMES
	// only. They exist because the answer's shape for an approval group could
	// not be measured any other way — the lab group had to be armed first — and
	// a maintainer reading a log has no other way to learn it.
	AnswerKind string
	AnswerKeys []string
}

func (j Joined) String() string {
	return fmt.Sprintf("group.Joined(group=%t pending=%t kind=%s keys=%v)",
		j.GroupJID != "", j.Pending, j.AnswerKind, j.AnswerKeys)
}

// InviteInfo is what a link says about a group WITHOUT joining it.
type InviteInfo struct {
	GroupJID string
	Subject  string
	Size     int
	// ApprovalRequired says following this link produces a request rather than
	// a membership.
	ApprovalRequired bool
}

// String keeps the subject out. A group's name is not this module's to log.
func (i InviteInfo) String() string {
	return fmt.Sprintf("group.InviteInfo(group=%t subject=%t size=%d approval=%t)",
		i.GroupJID != "", i.Subject != "", i.Size, i.ApprovalRequired)
}

const joinStateKey = "__waHeadlessGroupJoin"

// InviteInfo reads a group behind an invite code without joining it.
func (m *Manager) InviteInfo(ctx context.Context, code, label string) (InviteInfo, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return InviteInfo{}, ErrNoInviteCode
	}
	raw, err := m.parkedJoin(ctx, inviteInfoScript(code), label+"/invite-info")
	if err != nil {
		return InviteInfo{}, fmt.Errorf("%w: %v", ErrJoin, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		ID       string `json:"id"`
		Subject  string `json:"subject"`
		Size     int    `json:"size"`
		Approval bool   `json:"approval"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return InviteInfo{}, fmt.Errorf("group: unexpected invite-info answer: %w", e)
	}
	if !out.OK {
		return InviteInfo{}, fmt.Errorf("%w (%s)", ErrJoin, out.Why)
	}
	return InviteInfo{GroupJID: out.ID, Subject: out.Subject, Size: out.Size,
		ApprovalRequired: out.Approval}, nil
}

// JoinByInvite follows an invite code.
//
// A group that requires approval answers with ErrJoinPending and a Joined whose
// Pending is set — see that error for why neither success nor failure is the
// honest word for it.
func (m *Manager) JoinByInvite(ctx context.Context, code, label string) (Joined, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Joined{}, ErrNoInviteCode
	}
	raw, err := m.parkedJoin(ctx, joinScript(code), label+"/join")
	if err != nil {
		return Joined{}, fmt.Errorf("%w: %v", ErrJoin, err)
	}
	var out struct {
		OK      bool     `json:"ok"`
		Why     string   `json:"why"`
		ID      string   `json:"id"`
		Pending bool     `json:"pending"`
		Keys    []string `json:"keys"`
		Kind    string   `json:"kind"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Joined{}, fmt.Errorf("group: unexpected join answer: %w", e)
	}
	if !out.OK {
		return Joined{}, fmt.Errorf("%w (%s)", ErrJoin, out.Why)
	}
	if out.Pending {
		return Joined{Pending: true, AnswerKeys: out.Keys, AnswerKind: out.Kind}, ErrJoinPending
	}
	if out.ID == "" {
		return Joined{}, fmt.Errorf("%w: the join settled with no group id and no pending flag", ErrJoin)
	}
	return Joined{GroupJID: out.ID}, nil
}

func (m *Manager) parkedJoin(ctx context.Context, kick, label string) (string, error) {
	var started string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return m.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return m.eval(c, `window.`+joinStateKey+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", createBudget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(createTick):
		}
	}
}

const joinPrelude = `
	window.` + joinStateKey + ` = null;
	const park = v => { window.` + joinStateKey + ` = JSON.stringify(v); };
	const jidOf = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	// WHY THE FAILURE REPORT IS THIS ELABORATE. The first live run of the join
	// came back with an EMPTY reason, because it reported e.message and the
	// page threw something that has no message — this app rejects with plain
	// objects and with strings as readily as with Errors. An empty reason is
	// worse than a wrong one: it looks like the call failed silently when in
	// fact the diagnosis was thrown away at the boundary.
	const describe = e => {
		if (e === null || e === undefined) return "threw " + String(e);
		if (typeof e === "string") return "string: " + e;
		const name = (e.constructor && e.constructor.name) || typeof e;
		const parts = [name];
		if (e.message) parts.push("message=" + e.message);
		if (e.name && e.name !== name) parts.push("name=" + e.name);
		if (e.status !== undefined) parts.push("status=" + e.status);
		if (e.code !== undefined) parts.push("code=" + e.code);
		if (e.reason !== undefined) parts.push("reason=" + String(e.reason));
		// Field NAMES of whatever else it carries, never their values.
		try { parts.push("keys=[" + Object.keys(e).join(",") + "]"); } catch (_) {}
		return parts.join(" ");
	};
`

func inviteInfoScript(code string) string {
	return `(() => {` + joinPrelude + `
	(async () => {
		try {
			const info = await window.require("WAWebGroupQueryJob").queryGroupInvite(` + strconv.Quote(code) + `);
			park({
				ok: true,
				id: jidOf(info && (info.id || info.gid)),
				subject: (info && info.subject) || "",
				size: (info && (info.size || info.participantCount)) || 0,
				approval: !!(info && (info.membershipApprovalMode || info.isMembershipApprovalRequired)),
			});
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func joinScript(code string) string {
	return `(() => {` + joinPrelude + `
	(async () => {
		try {
			const res = await window.require("WAWebGroupInviteJob").joinGroupViaInvite(` + strconv.Quote(code) + `);
			// The SHAPE of a successful answer is unmeasured for a group that
			// asks for approval, so the key names come back with it. Names, not
			// values: one of those fields is a group id.
			let keys = [];
			try { keys = res && typeof res === "object" ? Object.keys(res) : []; } catch (_) {}
			// A GROUP THAT ASKS FOR APPROVAL DOES NOT RETURN A GROUP ID. What
			// exactly it returns is what the live proof measures; anything
			// without an id is reported as pending rather than guessed at, and
			// the raw shape is never invented into a success.
			const id = jidOf(res && (res.gid || res.id));
			park({ ok: true, id: id, pending: id === "", keys: keys, kind: typeof res });
		} catch (e) {
			// THE REJECTION IS THE ANSWER, MEASURED.
			//
			// A group that asks for approval does not resolve this call: it
			// REJECTS with an object named UnexpectedJoinGroupViaInviteResponse
			// carrying gid and membershipApprovalMode. The reference reads
			// res.gid._serialized off the resolved value and would crash here,
			// which is a reminder that copying its shape is not the same as
			// copying its understanding.
			//
			// Recognised by the CARRIED FIELD rather than by the name: the name
			// is a build detail, and membershipApprovalMode is the fact.
			const approval = e && typeof e === "object" && e.membershipApprovalMode !== undefined;
			if (approval) {
				let keys = [];
				try { keys = Object.keys(e); } catch (_) {}
				park({ ok: true, id: "", pending: true, keys: keys,
					kind: (e.name || "rejected") });
				return;
			}
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

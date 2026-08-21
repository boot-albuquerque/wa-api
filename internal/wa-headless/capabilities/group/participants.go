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

// Adding and removing group members.
//
// THE TWO SIBLINGS HAVE DIFFERENT SHAPES, and that is the finding rather than
// an inconvenience. Read from their own sources, side by side in one module:
//
//	addParticipantsJob({group, participants, isOffline, reason})
//	removeParticipantsJob(group, participants, timestamp, author, reason,
//	                      groupMetadata, isOffline)
//
// One takes an object, the other seven positional arguments. Assuming the
// second matched the first would have been another blind correction at exactly
// the layer that has cost this module five of them.
//
// REMOVING SOMEBODY IS VISIBLE TO THEM and to everyone else in the group. Like
// revoking a message, it is not undone by simply doing the opposite — the group
// keeps the system notice. So both directions verify, and both refuse before
// asking when the request makes no sense.

var (
	// ErrParticipants is the page refusing or throwing.
	ErrParticipants = fmt.Errorf("group: the page refused the participant change")
	// ErrNoParticipantGiven is a call with nobody to act on.
	ErrNoParticipantGiven = fmt.Errorf("group: no participant given")
	// ErrMembershipUnchanged is the postcondition: the call returned and the
	// member list is what it was.
	ErrMembershipUnchanged = fmt.Errorf("group: the page accepted the change and the membership did not change")
)

// Membership is what a participant change did.
type Membership struct {
	// Before and After are counts, not identities: a caller needs to know the
	// group changed, and the log must not name who is in it.
	Before, After int
	Waited        time.Duration
}

// Changed reports whether the member list moved.
func (m Membership) Changed() bool { return m.Before != m.After }

func (m Membership) String() string {
	return fmt.Sprintf("group.Membership(before=%d after=%d changed=%t waited=%s)",
		m.Before, m.After, m.Changed(), m.Waited.Round(time.Millisecond))
}

const participantsStateKey = "__waHeadlessGroupParticipants"

// AddParticipant adds one person to a group.
func (m *Manager) AddParticipant(ctx context.Context, groupJID, participantJID, label string) (Membership, error) {
	return m.changeMembership(ctx, groupJID, participantJID, true, label)
}

// RemoveParticipant removes one person from a group.
//
// It is a separate method rather than a flag because the two are not
// symmetrical acts: being removed from a group is visible to the person and
// leaves a notice everyone can see, and adding them back does not erase it.
func (m *Manager) RemoveParticipant(ctx context.Context, groupJID, participantJID, label string) (Membership, error) {
	return m.changeMembership(ctx, groupJID, participantJID, false, label)
}

func (m *Manager) changeMembership(ctx context.Context, groupJID, participantJID string, add bool, label string) (Membership, error) {
	if strings.TrimSpace(groupJID) == "" || !strings.HasSuffix(groupJID, "@g.us") {
		return Membership{}, ErrNotGroup
	}
	if strings.TrimSpace(participantJID) == "" {
		return Membership{}, ErrNoParticipantGiven
	}
	start := time.Now()

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, participantsScript(groupJID, participantJID, add, start.Unix()), &kicked)
	}); err != nil {
		return Membership{}, fmt.Errorf("%w: %v", ErrParticipants, err)
	}

	var out struct {
		Stage  string `json:"stage"`
		OK     bool   `json:"ok"`
		Why    string `json:"why"`
		Before int    `json:"before"`
		After  int    `json:"after"`
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, participantsResultScript, &raw)
		}); err != nil {
			return Membership{}, fmt.Errorf("%w: %v", ErrParticipants, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Membership{}, fmt.Errorf("group: unexpected participant answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Membership{}, fmt.Errorf("%w: the page never settled within %s", ErrParticipants, createBudget)
		}
		time.Sleep(createTick)
	}
	switch {
	case out.Why == "NOT_ADMIN":
		return Membership{}, ErrNotAdmin
	case out.Why == "ALREADY_MEMBER" || out.Why == "NOT_A_MEMBER":
		// Nothing to do is a successful no-op, not a failure — the same shape
		// as a conversation with nothing unread (H52) and a chat already in the
		// requested state (H55).
		return Membership{Before: out.Before, After: out.Before, Waited: time.Since(start)}, nil
	case out.Stage == "find" && !out.OK:
		return Membership{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case !out.OK:
		return Membership{}, fmt.Errorf("%w at %s (%s)", ErrParticipants, out.Stage, out.Why)
	}

	// THE POSTCONDITION. Removing somebody who is still there, or adding
	// somebody who never arrives, is the failure a caller cannot see.
	want := out.Before + 1
	if !add {
		want = out.Before - 1
	}
	if out.After != want {
		return Membership{}, fmt.Errorf("%w: %d before, %d after, wanted %d",
			ErrMembershipUnchanged, out.Before, out.After, want)
	}
	return Membership{Before: out.Before, After: out.After, Waited: time.Since(start)}, nil
}

func participantsScript(groupJID, participantJID string, add bool, now int64) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(participantsStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(participantsStateKey) + `] = v; };
		const resolveIdentity = ` + spa.ResolveIdentityExpr + `;
		(async () => {
		let stage = 'find';
		try {
			const WidFactory = window.require('` + string(spa.ModuleWidFactory) + `');
			const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const gwid = WidFactory.createWid(` + strconv.Quote(groupJID) + `);
			if (!gwid) { park({ stage, ok: false, why: 'WID_NULL' }); return; }
			const chat = Chats.get(gwid);
			if (!chat) { park({ stage, ok: false, why: 'NO_GROUP' }); return; }
			const md = chat.groupMetadata;
			if (!md || !md.participants) { park({ stage, ok: false, why: 'NO_METADATA' }); return; }

			// ADMIN IS A METHOD on the participants collection (H57), not a
			// field on the metadata.
			try {
				if (typeof md.participants.iAmAdmin === 'function' && md.participants.iAmAdmin() === false) {
					park({ stage, ok: false, why: 'NOT_ADMIN' });
					return;
				}
			} catch (e) {}

			stage = 'resolve';
			// The person is addressed by the identity the SERVER assigns, like
			// everywhere else in this build (H34).
			const r = await resolveIdentity(` + strconv.Quote(participantJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			const list = () => {
				try { return md.participants.getModelsArray(); } catch (e) { return []; }
			};
			const isMember = () => list().some(p => {
				try { return p.id && p.id._serialized === r.jid; } catch (e) { return false; }
			});
			const before = list().length;
			const member = isMember();
			const adding = ` + strconv.FormatBool(add) + `;
			if (adding && member) { park({ stage: 'done', ok: true, why: 'ALREADY_MEMBER', before: before, after: before }); return; }
			if (!adding && !member) { park({ stage: 'done', ok: true, why: 'NOT_A_MEMBER', before: before, after: before }); return; }

			stage = 'apply';
			const J = window.require('` + string(spa.ModuleGroupParticipantsJob) + `');
			` + map[bool]string{
		true: `// ONE OBJECT. Read from addParticipantsJob's own source.
			await J.addParticipantsJob({ group: gwid, participants: [r.wid], isOffline: false, reason: 'addParticipants' });`,
		false: `// SEVEN POSITIONAL ARGUMENTS, unlike its sibling one line above in
			// the same module: (group, participants, timestamp, author, reason,
			// groupMetadata, isOffline).
			// THE TIMESTAMP COMES FROM GO. Invariant 6 keeps the clock on this
			// side, and a page that reads its own is a page whose answers cannot
			// be reproduced from a transcript.
			// THE AUTHOR IS REQUIRED. Passing null threw "Cannot read properties
			// of undefined (reading 'toString')" — something inside renders it.
			// The author is this account, addressed the way THIS GROUP is
			// addressed: getMeUserMatchingAddressingModeOrThrow, which the app
			// itself uses when it needs the same thing.
			const Me = window.require('WAWebUserPrefsMeUser');
			const author = Me.getMeUserMatchingAddressingModeOrThrow(gwid);
			await J.removeParticipantsJob(gwid, [r.wid], ` + strconv.FormatInt(now, 10) + `, author,
				'removeParticipants', md, false);`,
	}[add] + `

			stage = 'verify';
			park({ stage: 'done', ok: true, why: '', before: before, after: list().length });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const participantsResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + participantsStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

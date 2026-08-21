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
// PARTICIPANTS ARE RECORDS, NOT WIDS, and that was the actual blocker. Passing
// [wid] threw "Cannot read properties of undefined (reading 'toString')" with a
// null author AND with the author resolved — identical both times, which was the
// evidence that the author was never the problem. The app's own call site
// settled it:
//
//	removeParticipantsJob(n, a.participants, x, t.author, a.reason, r, i)
//
// with the line above reading a.participants.some(e => e.id).
//
// THIS BUILD CANNOT CONFIRM A PARTICIPANT CHANGE IN THE SAME SESSION, and that
// is measured, not assumed. Three changes were proven to reach the server —
// each one visible from a FRESH session — while within the session that made
// them:
//
//	chat.groupMetadata.participants   stayed at the old count for 90 seconds
//	GroupMetadataCollection.get(...)  is the SAME OBJECT, so it stays too
//	MsgCollection                     received NO gp2/add or gp2/remove notice
//	                                  (a gp2/subject from a rename did arrive,
//	                                  so the channel itself works)
//
// So Membership reports Verified: false for every real change. That is the
// asymmetric contract this module already uses for reaction removal (H53): the
// honest answer is "sent, not confirmable here", and pretending otherwise is
// how the lab group spent an afternoon with one member while three test runs
// reported "unchanged".
//
// THE PROOF IS ACROSS SESSIONS. Count(), which reads the same metadata, is
// correct at session start; the live test removes in one session and counts in
// the next.
//
// REMOVING SOMEBODY IS VISIBLE TO THEM and to everyone else in the group. Like
// revoking a message, it is not undone by simply doing the opposite — the group
// keeps the system notice.

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
	// Before is the count this session saw, and WantedAfter is what it should
	// become. Counts, not identities: a caller needs to know the group changed,
	// and the log must not name who is in it.
	Before, WantedAfter int
	// NoOp is true when there was nothing to do — already a member, or not one.
	NoOp bool
	// Verified says whether the change was CONFIRMED. On this build it is true
	// only for a no-op, because a real change is invisible to the session that
	// made it: the metadata does not refresh and no system notice arrives.
	// Reporting a change as confirmed here would be a lie the caller cannot
	// detect, and one that already cost this repository a broken lab group.
	Verified bool
	Waited   time.Duration
}

// Changed reports whether a change was requested at all.
func (m Membership) Changed() bool { return !m.NoOp }

func (m Membership) String() string {
	return fmt.Sprintf("group.Membership(before=%d wantedAfter=%d noop=%t verified=%t waited=%s)",
		m.Before, m.WantedAfter, m.NoOp, m.Verified, m.Waited.Round(time.Millisecond))
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
		// A no-op is the ONE outcome this build can confirm in-session: nothing
		// had to move, so nothing had to be observed moving.
		return Membership{Before: out.Before, WantedAfter: out.Before, NoOp: true,
			Verified: true, Waited: time.Since(start)}, nil
	case out.Stage == "find" && !out.OK:
		return Membership{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case !out.OK:
		return Membership{}, fmt.Errorf("%w at %s (%s)", ErrParticipants, out.Stage, out.Why)
	}

	// NO POSTCONDITION, AND IT SAYS SO. Verified is false because this build
	// gives the session that made the change nothing to observe: see the file
	// comment for the three signals that were tried and what each reported.
	// Returning Verified: true here would be a claim the caller cannot check —
	// and the version of this code that did exactly that reported "unchanged"
	// three runs in a row for changes that had all worked.
	want := out.Before + 1
	if !add {
		want = out.Before - 1
	}
	return Membership{Before: out.Before, WantedAfter: want, NoOp: false,
		Verified: false, Waited: time.Since(start)}, nil
}

// Count reports how many participants a group has, as this session sees it.
//
// IT IS CORRECT AT SESSION START AND STALE AFTER A CHANGE THIS SESSION MADE.
// That is not a defect in Count — it is the build's behaviour, measured — and it
// is what makes cross-session verification the only honest proof available:
// change in one session, Count in the next.
func (m *Manager) Count(ctx context.Context, groupJID, label string) (int, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return 0, ErrNotGroup
	}
	var raw string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/count", func(ctx context.Context) error {
		return m.eval(ctx, countScript(groupJID), &raw)
	}); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrParticipants, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("group: unexpected count answer: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("%w: the group's metadata is not loaded", ErrNotGroup)
	}
	return n, nil
}

func countScript(groupJID string) string {
	return `(() => {
		try {
			const W = window.require('` + string(spa.ModuleWidFactory) + `');
			const C = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = C.get(W.createWid(` + strconv.Quote(groupJID) + `));
			const p = chat && chat.groupMetadata && chat.groupMetadata.participants;
			if (!p) { return '-1'; }
			// STRING, because engine.Tab.Evaluate unmarshals into one and a bare
			// number is a type error rather than a count.
			return String(p.getModelsArray ? p.getModelsArray().length : (p.length || -1));
		} catch (e) { return '-1'; }
	})()`
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
			// part() produces what the jobs actually take: the group's own
			// participant record when it has one, and the minimal {id} shape
			// otherwise. Adding somebody has no record yet, which is why the
			// fallback is not dead code.
			const part = (wid) => {
				try {
					const found = md.participants.get ? md.participants.get(wid) : null;
					if (found) { return found; }
				} catch (e) {}
				return { id: wid };
			};
			` + map[bool]string{
		true: `// ONE OBJECT. Read from addParticipantsJob's own source.
			// PARTICIPANT RECORDS, not bare wids — see the note on the sibling.
			await J.addParticipantsJob({ group: gwid, participants: [part(r.wid)],
				isOffline: false, reason: 'addParticipants' });`,
		false: `// SEVEN POSITIONAL ARGUMENTS, unlike its sibling one line above in
			// the same module: (group, participants, timestamp, author, reason,
			// groupMetadata, isOffline).
			// THE TIMESTAMP COMES FROM GO. Invariant 6 keeps the clock on this
			// side, and a page that reads its own is a page whose answers cannot
			// be reproduced from a transcript.
			// PARTICIPANTS ARE RECORDS, NOT WIDS. This is what the two blind
			// attempts got wrong, and neither the signature nor the error said
			// so: passing [wid] threw "Cannot read properties of undefined
			// (reading 'toString')" with author null AND with the author
			// resolved, which is exactly the evidence that the author was never
			// the problem.
			//
			// The app's own call site settled it:
			//
			//	removeParticipantsJob(n, a.participants, x, t.author, a.reason, r, i)
			//
			// and the line above it in the same function reads
			// a.participants.some(e => e.id) — so each entry carries .id. The
			// real record from the group's metadata is preferred; part() falls
			// back to the minimal shape.
			//
			// THE TIMESTAMP COMES FROM GO. Invariant 6 keeps the clock on this
			// side, and a page that reads its own is a page whose answers cannot
			// be reproduced from a transcript.
			const Me = window.require('WAWebUserPrefsMeUser');
			const author = Me.getMeUserMatchingAddressingModeOrThrow(gwid);
			await J.removeParticipantsJob(gwid, [part(r.wid)], ` + strconv.FormatInt(now, 10) + `, author,
				'removeParticipants', md, false);`,
	}[add] + `

			stage = 'verify';
			// THE AWAIT IS NOT THE COMPLETION (H61, measured at 696ms on a
			// sibling capability). Reading list().length here returned the OLD
			// count and reported a removal that had in fact worked as
			// "membership did not change" — the second wrong diagnosis this
			// entry produced. The METADATA is parked and Go polls it.
			// NO POSTCONDITION HERE, and that is the measured conclusion rather
			// than a shortcut. Neither the metadata nor the message collection
			// reflects a participant change in the session that made it — see
			// the file comment for the three things that were tried and what
			// each of them reported. Waiting on any of them produced a wrong
			// answer for 90 seconds, three runs in a row.
			park({ stage: 'done', ok: true, why: '', before: before,
				want: adding ? before + 1 : before - 1 });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// participantsResultScript returns what the kick parked. There is no settling
// branch: nothing this session can read reflects a participant change it made.
const participantsResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + participantsStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

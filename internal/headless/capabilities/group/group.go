// Package group creates and inspects groups.
//
// IT EXISTS TO MAKE A PROOF POSSIBLE. Sending to a group could not be proven
// against a real account, because a real group has real members and a test that
// messages them is a test nobody may run (HOUSEKEEP H48). A group created
// between two lab accounts is the only target where the dispatch can be
// exercised honestly.
//
// EVERY CALL SHAPE HERE WAS READ FROM THE APP'S OWN CODE. The bundles were
// searched for how WhatsApp Web itself calls the job, and the answer was:
//
//	const args = {title, thumb: null, full: null, restrict: false,
//	              announce: false, membershipApprovalMode: false,
//	              memberAddMode: false, memberShareGroupHistoryMode: false};
//	const res = await GroupCreateJob.createGroup(args, participants, outContacts);
//	const gid = WidFactory.asGroupWidOrThrow(res.wid);
//
// THE PARTICIPANT ARGUMENT CARRIES AN ID; IT IS NOT ONE. Measured the hard way:
// getGroupMutationParticipant(wid, …) throws "Cannot read properties of
// undefined (reading 'isLid')", while the same call with the CONTACT MODEL
// returns {lid, phoneNumber}. Its source reads t.id.isLid(), t.phoneNumber and
// t.username — all fields of a contact.
//
// This is the THIRD time this exact shape appeared in one day: the avatar
// bridge died on 'isNewsletter' for the same reason, and the media path's
// sendToChat takes one object rather than positional arguments. When a page
// function dies reading a field off undefined, the argument is almost always a
// MODEL and not the identity inside it.
//
// TWO DOORS, AND THE OBVIOUS ONE IS WRONG. WAWebCreateGroupAction is the UI
// layer: it opens a toast through WAWebToastManager and builds React elements.
// A headless driver calling it would be driving the interface to reach the
// operation. The job below is what that action calls underneath.
//
// CREATION IS A PERSISTENT ARTEFACT on someone's account, so this package is
// deliberately conservative: it is IDEMPOTENT by subject, it never invents
// participants, and the caller is expected to name the group so a human seeing
// it understands what it is. Removing one is a human action in the app; nothing
// here deletes.
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

// Bounds. Var, not const, so tests can compress the clock.
var (
	createBudget = 90 * time.Second
	createTick   = time.Second
)

var (
	// ErrCreate is the page refusing or throwing.
	ErrCreate = fmt.Errorf("group: the page refused to create the group")
	// ErrNoSubject is a group with no name. It is refused here rather than
	// sent, because an unnamed group on a real account is indistinguishable
	// from junk to the human who finds it.
	ErrNoSubject = fmt.Errorf("group: a group must be named")
	// ErrNoParticipants is a group with nobody in it.
	ErrNoParticipants = fmt.Errorf("group: a group needs at least one participant")
	// ErrParticipantMissing is the postcondition: the group came back without
	// somebody who was asked for. It is the failure a caller cannot see, and
	// the reason this package verifies at all.
	ErrParticipantMissing = fmt.Errorf("group: the created group is missing a requested participant")
)

// Group is one group, identified the way everything else here is.
type Group struct {
	// JID is the group's identity. A group has no lid counterpart — it IS its
	// own identity (H48).
	JID string
	// Subject is the group's name. It is carried because the caller chose it
	// and needs it back to recognise the group, and because it is the key this
	// package uses for idempotency.
	Subject string
	// Participants is how many members the page reports.
	Participants int
	// Created says whether this call made the group or found it already there.
	// A caller that cannot tell the two apart cannot know whether it just
	// changed someone's account.
	Created bool
}

// String redacts the identity but keeps the subject: the subject is chosen by
// the caller for a test group, not personal data taken from an account.
func (g Group) String() string {
	return fmt.Sprintf("group.Group(jid=<redacted> subject=%q participants=%d created=%t)",
		g.Subject, g.Participants, g.Created)
}

// Manager creates and finds groups on one session.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Manager.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// Ensure returns a group with this subject, creating it only if it is not
// already there.
//
// IDEMPOTENCY IS NOT AN OPTIMISATION HERE. Without it, every run of a test that
// needs a group would leave another one behind on a real account, and the
// person who owns it would find a pile of near-identical groups nobody can
// tell apart.
//
// participants are jids as a caller would write them; each is resolved through
// the same path a send uses, because this build addresses people by the lid the
// server returns and not by the number that was typed (H34).
func (m *Manager) Ensure(ctx context.Context, subject string, participants []string, label string) (Group, error) {
	if strings.TrimSpace(subject) == "" {
		return Group{}, ErrNoSubject
	}
	if len(participants) == 0 {
		return Group{}, ErrNoParticipants
	}

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, ensureScript(subject, participants), &kicked)
	}); err != nil {
		return Group{}, fmt.Errorf("%w: %v", ErrCreate, err)
	}

	var out struct {
		Stage        string   `json:"stage"`
		OK           bool     `json:"ok"`
		Why          string   `json:"why"`
		JID          string   `json:"jid"`
		Subject      string   `json:"subject"`
		Participants int      `json:"participants"`
		Created      bool     `json:"created"`
		Missing      []string `json:"missing"`
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		// ONE SCRIPT FOR BOTH WAITS. ensureVerifyScript returns the parked
		// state unchanged unless the state is 'awaiting_chat', in which case it
		// spends this turn looking for the chat. Reading and re-checking are
		// therefore the same evaluation, and the number of turns is decided
		// here rather than in the page (invariant 6).
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, ensureVerifyScript, &raw)
		}); err != nil {
			return Group{}, fmt.Errorf("%w: %v", ErrCreate, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Group{}, fmt.Errorf("group: unexpected answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "awaiting_chat" {
			break
		}
		if !time.Now().Before(deadline) {
			// A CREATE THAT NEVER APPEARED IS NOT A PAGE THAT HUNG, and the
			// two used to share one message. The group exists on the server —
			// createGroup returned a wid — and the collection never showed it.
			if out.Stage == "awaiting_chat" {
				return Group{}, fmt.Errorf("%w at verify (CREATED_BUT_NOT_IN_COLLECTION within %s)",
					ErrCreate, createBudget)
			}
			return Group{}, fmt.Errorf("%w: the page never settled within %s", ErrCreate, createBudget)
		}
		time.Sleep(createTick)
	}
	if !out.OK {
		return Group{}, fmt.Errorf("%w at %s (%s)", ErrCreate, out.Stage, out.Why)
	}
	if out.JID == "" {
		return Group{}, fmt.Errorf("%w: the page reported success without a group identity", ErrCreate)
	}
	// THE POSTCONDITION. A group that came back without somebody who was asked
	// for is a group the caller would use believing the wrong people are in it,
	// and nothing else would tell them. The count of missing participants is
	// reported; WHICH ones is not, because that would be an identity in a log.
	if len(out.Missing) > 0 {
		return Group{}, fmt.Errorf("%w: %d of %d requested participant(s) are not in it",
			ErrParticipantMissing, len(out.Missing), len(participants))
	}
	return Group{
		JID:          out.JID,
		Subject:      out.Subject,
		Participants: out.Participants,
		Created:      out.Created,
	}, nil
}

const ensureStateKey = "__headlessGroupEnsure"

func ensureScript(subject string, participants []string) string {
	quoted := make([]string, 0, len(participants))
	for _, p := range participants {
		quoted = append(quoted, strconv.Quote(p))
	}
	return `JSON.stringify((() => {` + verifyFnJS + `
		window[` + strconv.Quote(ensureStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(ensureStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const WidFactory = window.require('` + string(spa.ModuleWidFactory) + `');
			const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const GMGetters = window.require('` + string(spa.ModuleGroupMetadataGetters) + `');
			const want = ` + strconv.Quote(subject) + `;

			// Read a group's subject through the page's getter, falling back to
			// the chat's formattedTitle. Two sources because a freshly created
			// group can have metadata before the chat is fully named.
			const subjectOf = (chat) => {
				try {
					const s = GMGetters.getSubject && GMGetters.getSubject(chat.groupMetadata || chat);
					if (typeof s === 'string' && s) { return s; }
				} catch (e) {}
				try { if (typeof chat.formattedTitle === 'string') { return chat.formattedTitle; } } catch (e) {}
				return '';
			};

			const findExisting = () => {
				for (const c of Chats.getModelsArray()) {
					try {
						if (!c.id || c.id.server !== 'g.us') { continue; }
						if (subjectOf(c) === want) { return c; }
					} catch (e) {}
				}
				return null;
			};

			// RESOLVE THE PARTICIPANTS FIRST, and do it before creating
			// anything: a group made with an unresolvable member would have to
			// be cleaned up by a human.
			stage = 'resolve';
			const Query = window.require('` + string(spa.ModuleQueryExistsJob) + `');
			const Contacts = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			const wanted = [` + strings.Join(quoted, ", ") + `];
			const resolved = [];
			const contacts = [];
			for (const jid of wanted) {
				const local = WidFactory.createWid(jid);
				if (!local) { park({ stage, ok: false, why: 'WID_NULL' }); return; }
				const ex = await Query.queryWidExists(local);
				if (!ex || !ex.wid) { park({ stage, ok: false, why: 'NOT_ON_WHATSAPP' }); return; }
				resolved.push(ex.wid);
				// THE PARTICIPANT HELPER WANTS A CONTACT, NOT A WID. Its source
				// reads t.id.isLid(), t.phoneNumber and t.username — fields of a
				// contact model, none of which a wid has. Passing the wid throws
				// "Cannot read properties of undefined (reading 'isLid')".
				const contact = Contacts.get(ex.wid);
				if (!contact) { park({ stage, ok: false, why: 'NO_CONTACT_FOR_PARTICIPANT' }); return; }
				contacts.push(contact);
			}

			stage = 'find';
			let chat = findExisting();
			let created = false;

			if (!chat) {
				stage = 'create';
				const Job = window.require('` + string(spa.ModuleGroupCreateJob) + `');
				const PU = window.require('` + string(spa.ModuleGroupMutationParticipantUtils) + `');
				// The exact argument set the app itself passes.
				const args = {
					title: want, thumb: null, full: null, restrict: false, announce: false,
					membershipApprovalMode: false, memberAddMode: false,
					memberShareGroupHistoryMode: false
				};
				const members = contacts.map(c => PU.getGroupMutationParticipant(c, true, 'createGroup'));
				const res = await Job.createGroup(args, members, []);
				if (!res || !res.wid) { park({ stage, ok: false, why: 'NO_WID_RETURNED' }); return; }
				created = true;
				// THE CHAT TAKES A MOMENT TO APPEAR, AND THE WAITING IS GO'S.
				//
				// This used to loop here with setTimeout, and the comment above
				// it claimed the Go side owned the clock — which was false where
				// it was written: a page that sleeps decides its own timeout
				// during a reload, a throttled tab and a hung renderer, where Go
				// can neither see the decision nor cancel it (invariant 6).
				//
				// So it parks and stops. Go's existing poll loop sees
				// 'awaiting_chat' and evaluates ensureVerifyScript, which does
				// the same lookup once per turn under the caller's budget.
				const gid = WidFactory.asGroupWidOrThrow(res.wid);
				park({
					stage: 'awaiting_chat', ok: false, why: '',
					gid: (gid && gid._serialized) || '',
					want: want,
					resolved: resolved.map(w => w._serialized),
				});
				return;
			}

			stage = 'verify';
			park(verify(chat, created, resolved.map(w => w._serialized)));
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 180) });
		}
		})();
		return { started: true };
	})())`
}

// verifyFnJS is the participant read-back, defined ONCE and injected into both
// the create script and the poll script.
//
// It exists as a shared string rather than as two copies because the two paths
// ask the identical question — "who is actually in this group?" — and a copy
// that drifts would let one path enforce the postcondition while the other
// quietly stopped.
const verifyFnJS = `
	const subjectOfChat = (c) => {
		try { return (c.groupMetadata && c.groupMetadata.subject) || c.formattedTitle || ''; }
		catch (e) { return ''; }
	};
	const verify = (chat, created, wantedSerialized) => {
		// WHO IS ACTUALLY IN IT. The participant list is read back from the
		// page rather than assumed from what was asked for.
		let present = [];
		try {
			const md = chat.groupMetadata;
			const parts = md && md.participants;
			const arr = parts && (typeof parts.getModelsArray === 'function'
				? parts.getModelsArray() : (parts.toArray ? parts.toArray() : []));
			present = (arr || []).map(p => {
				try { return (p.id && p.id._serialized) || ''; } catch (e) { return ''; }
			}).filter(Boolean);
		} catch (e) {}
		const missing = [];
		for (const w of wantedSerialized || []) {
			if (w && present.indexOf(w) === -1) { missing.push('redacted'); }
		}
		return {
			stage: 'done', ok: true, why: '',
			jid: (chat.id && chat.id._serialized) || '',
			subject: subjectOfChat(chat),
			participants: present.length,
			created: created,
			missing: missing
		};
	};
`

// ensureVerifyScript is one TURN of the wait that used to happen in the page.
//
// Synchronous by design: it asks the model what it holds right now and answers
// in a single evaluation, which is what keeps the deciding — how many turns,
// how long — on the Go side where the caller's context can reach it.
const ensureVerifyScript = `JSON.stringify((() => {
	const KEY = ` + `"` + ensureStateKey + `"` + `;
	const s = window[KEY];
	if (!s) { return { stage: 'create', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage !== 'awaiting_chat') { return s; }
	try {` + verifyFnJS + `
		const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
		let chat = s.gid ? Chats.get(s.gid) : null;
		if (!chat) {
			// The subject fallback, same as the create path's findExisting.
			const all = typeof Chats.getModelsArray === 'function' ? Chats.getModelsArray() : [];
			for (const c of all) {
				try {
					if (c.id && c.id.server === 'g.us' && subjectOfChat(c) === s.want) { chat = c; break; }
				} catch (e) {}
			}
		}
		if (!chat) { return s; }
		const done = verify(chat, true, s.resolved);
		window[KEY] = done;
		return done;
	} catch (e) {
		return { stage: 'verify', ok: false, why: String((e && e.message) || e).slice(0, 180) };
	}
})())`

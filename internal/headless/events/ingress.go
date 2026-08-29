package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// The page side: one installer, one buffer, one sequence.

// Bounds. Var, not const, so tests can compress the clock.
var (
	// PollInterval is how often Go asks the page for what happened. It is a
	// compromise nobody escapes: shorter means more round trips, longer means
	// the page's buffer holds more and drops sooner.
	PollInterval = 500 * time.Millisecond
	// BufferSize is how many events the PAGE holds between polls. Past it the
	// page drops and counts, which is the honest failure: the alternative is
	// unbounded growth in somebody else's process.
	BufferSize = 512
)

const stateKey = "__headlessEventBus"

// Pump moves events from the page to a Hub.
type Pump struct {
	runner *engine.Runner
	eval   spa.Evaluator
	hub    *Hub
}

// NewPump builds one. It does not touch the page until Run.
func NewPump(runner *engine.Runner, eval spa.Evaluator, hub *Hub) *Pump {
	return &Pump{runner: runner, eval: eval, hub: hub}
}

// Run pumps until the context is cancelled.
//
// IT REINSTALLS ON EVERY CYCLE, and that is the design rather than an
// inefficiency. A page reload wipes the handlers and the buffer, and detecting
// a reload needs a signal this module does not have. An idempotent install
// answers "already" in the ordinary case and repairs itself in the other, so
// recovery costs one cheap call per poll and needs no detection.
//
// EVENTS FROM THE FIRST DRAIN AFTER AN INSTALL ARE MARKED Replay. After a
// reload the page refills its collections from history, and the same 'add' that
// means "a message arrived" fires for every message that ever arrived. A
// consumer that counts those as new double-counts its whole history.
func (p *Pump) Run(ctx context.Context) error {
	firstAfterInstall := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		installed, fresh, err := p.install(ctx)
		if err != nil {
			return err
		}
		if !installed {
			// The page is not ready to be subscribed to yet — during boot, or
			// mid-reload. Waiting is right; failing would make a normal moment
			// look like a defect.
			if !sleepCtx(ctx, PollInterval) {
				return ctx.Err()
			}
			continue
		}
		if fresh {
			p.hub.mu.Lock()
			// The outgoing incarnation's counters are folded into the base
			// before the page starts counting again from zero.
			p.hub.droppedBase += p.hub.lastPageDropped
			p.hub.seenBase += p.hub.lastPageSeen
			p.hub.lastPageDropped, p.hub.lastPageSeen = 0, 0
			p.hub.stats.Reinstalls++
			// THE SEQUENCE RESTARTS WITH THE PAGE. Keeping the old high-water
			// mark would make every event after a reload look like a gap, and
			// the gap counter would report drops that never happened.
			p.hub.lastSeq = 0
			p.hub.mu.Unlock()
			firstAfterInstall = true
		}

		batch, err := p.drain(ctx)
		if err != nil {
			return err
		}
		for _, e := range batch {
			e.Fresh = classify(e, firstAfterInstall)
			// CONSERVATIVE BY CONSTRUCTION: anything not PROVEN live reads as
			// replay for every consumer written before Freshness existed.
			e.Replay = e.Fresh != FreshnessLive
			p.hub.deliver(e)
		}
		// THE WINDOW CLOSES AFTER THE FIRST DRAIN, EMPTY OR NOT. It used to
		// close only on a non-empty one, and that is wrong on a QUIET page: the
		// replay burst, if there is one, is already buffered when the first
		// drain runs. If that drain comes back empty there was no replay, and
		// keeping the flag set marks the next real event — possibly minutes
		// later — as history.
		//
		// It showed up as a flaky live test: the same send produced
		// message.added on one run and nothing on the next, because a
		// subscriber that skips replay skipped it.
		firstAfterInstall = false
		if !sleepCtx(ctx, PollInterval) {
			return ctx.Err()
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (p *Pump) install(ctx context.Context) (installed, fresh bool, err error) {
	var raw string
	if e := p.runner.Do(ctx, engine.OpStateProbe, "events/install", func(c context.Context) error {
		return p.eval(c, installScript(), &raw)
	}); e != nil {
		return false, false, fmt.Errorf("events: installing the ingress: %w", e)
	}
	var out struct {
		Installed bool   `json:"installed"`
		Already   bool   `json:"already"`
		Reason    string `json:"reason"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return false, false, fmt.Errorf("events: unexpected install answer: %w", e)
	}
	return out.Installed, out.Installed && !out.Already, nil
}

func (p *Pump) drain(ctx context.Context) ([]Event, error) {
	var raw string
	if e := p.runner.Do(ctx, engine.OpStateProbe, "events/drain", func(c context.Context) error {
		return p.eval(c, drainScript, &raw)
	}); e != nil {
		return nil, fmt.Errorf("events: draining: %w", e)
	}
	var out struct {
		OK      bool `json:"ok"`
		Dropped int64
		Seen    int64
		Rows    []struct {
			Type    string `json:"type"`
			Seq     int64  `json:"seq"`
			At      int64  `json:"at"`
			Chat    string `json:"chat"`
			Msg     string `json:"msg"`
			FromMe  bool   `json:"fromMe"`
			Kind    string `json:"kind"`
			Ack     int    `json:"ack"`
			BodyLen int    `json:"bodyLen"`

			MsgT    int64  `json:"msgT"`
			Subtype string `json:"subtype"`

			Call     string `json:"call"`
			Peer     string `json:"peer"`
			Video    bool   `json:"video"`
			Outgoing bool   `json:"outgoing"`
			Group    bool   `json:"group"`
		} `json:"rows"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, fmt.Errorf("events: unexpected drain answer: %w", e)
	}
	p.hub.mu.Lock()
	// ACCUMULATED, NOT OVERWRITTEN. The page's counters live in the page, so a
	// reload resets them to zero — and a stat that goes BACKWARDS is worse than
	// no stat, because the number people quote is the one after the last
	// reload. The base carries what previous incarnations reported.
	p.hub.stats.DroppedInPage = p.hub.droppedBase + out.Dropped
	p.hub.stats.SeenInPage = p.hub.seenBase + out.Seen
	p.hub.lastPageDropped = out.Dropped
	p.hub.lastPageSeen = out.Seen
	p.hub.mu.Unlock()

	evs := make([]Event, 0, len(out.Rows))
	for _, r := range out.Rows {
		t := Type(r.Type)
		// AN UNKNOWN TYPE IS DROPPED HERE, not forwarded. The page and this
		// file are versioned together; a name that does not match means one of
		// them changed alone, and forwarding it would let a subscriber match on
		// a string that means nothing.
		//
		// The list is PageTypes, not KnownTypes, and that is deliberate: a
		// session.* name arriving from the page would mean the page is claiming
		// to know something only this process can know, which is exactly the
		// forgery this filter exists to refuse.
		known := false
		for _, k := range PageTypes {
			if k == t {
				known = true
				break
			}
		}
		if !known {
			continue
		}
		// A GROUP SYSTEM MESSAGE IS RECLASSIFIED HERE, IN GO.
		//
		// The page announces it as an ordinary added message of kind "gp2"; the
		// reference turns that into one of four distinct events by looking at the
		// subtype. Doing the same in the page would be deciding in the page,
		// which invariant 6 forbids, so the subtype travels and the decision is
		// made here.
		if t == MessageAdded && r.Kind == kindGroupSystem {
			if g, ok := GroupTypeFor(r.Subtype); ok {
				t = g
			}
		}
		evs = append(evs, Event{
			Type: t, Origin: SourcePage, Seq: r.Seq, At: time.UnixMilli(r.At),
			Subtype: r.Subtype,
			ChatJID: r.Chat, MessageID: r.Msg, FromMe: r.FromMe,
			Kind: r.Kind, Ack: r.Ack, BodyLen: r.BodyLen,
			Aged:       r.MsgT > 0,
			AgeSeconds: ageSeconds(r.MsgT, r.At),
			CallID:     r.Call, CallerJID: r.Peer, Video: r.Video,
			// A call the account PLACED is not an incoming call, and the two
			// arrive through the same collection. FromMe carries the difference
			// so a subscriber does not have to know that.
			OutgoingCall: r.Outgoing, GroupCall: r.Group,
		})
	}
	return evs, nil
}

// Uninstall removes the page-side handlers.
//
// A session teardown should call it, and it is safe when nothing is installed.
// Leaving handlers behind is not merely untidy: a later Pump would install a
// second set, and the page would push every event twice.
func (p *Pump) Uninstall(ctx context.Context) error {
	var raw string
	return p.runner.Do(ctx, engine.OpStateProbe, "events/uninstall", func(c context.Context) error {
		return p.eval(c, uninstallScript, &raw)
	})
}

func installScript() string {
	return `JSON.stringify((() => {
		const KEY = ` + strconv.Quote(stateKey) + `;
		const st = window[KEY];
		if (st && st.installed) { return { installed: true, already: true }; }

		let MC, CC;
		try {
			MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
		} catch (e) { return { installed: false, reason: 'NO_MODULES' }; }
		if (!MC || typeof MC.on !== 'function' || !CC || typeof CC.on !== 'function') {
			return { installed: false, reason: 'NO_COLLECTIONS' };
		}

		const s = { buf: [], dropped: 0, seen: 0, seq: 0, installed: false,
			cap: ` + strconv.Itoa(BufferSize) + `, handlers: [] };

		// EVERY HANDLER PUSHES INTO THE SAME BUFFER, so the sequence orders
		// events across types and one ceiling governs them all.
		const push = (row) => {
			s.seen++;
			if (s.buf.length >= s.cap) { s.dropped++; return; }
			s.seq++;
			row.seq = s.seq;
			row.at = Date.now();
			s.buf.push(row);
		};

		// NO CONTENT CROSSES. A body becomes its length; everything else is an
		// identity a caller must route on, or a small enum.
		const msgRow = (type, m) => {
			const id = m && m.id;
			return {
				type: type,
				chat: (id && id.remote && id.remote._serialized) || '',
				msg: (id && id.id) || '',
				fromMe: !!(id && id.fromMe),
				kind: (typeof m.type === 'string') ? m.type : '',
				ack: (typeof m.ack === 'number') ? m.ack : 0,
				bodyLen: (m && typeof m.body === 'string') ? m.body.length : 0,
				// THE MESSAGE'S OWN TIMESTAMP, which is the only causal
				// discriminator this bus has. A message created two days ago and
				// announced now is hydration; one created a second ago is news.
				// It is the message's field, not a clock read for a decision —
				// invariant 6 is about DECIDING in the page, and this only
				// reports.
				msgT: (m && typeof m.t === 'number') ? m.t : 0,
				// THE GROUP-NOTIFICATION SUBTYPE. A system message about a group
				// arrives as an ordinary added message of kind "gp2"; what KIND of
				// group event it is lives only in this field. It travels here
				// because the classification belongs in Go — deciding in the page
				// is what invariant 6 forbids — and because it costs one string on
				// a row that already exists.
				//
				// Measured 2026-08-22: 1 gp2 message in a store of 395, subtype
				// "membership_approval_mode". The field is present and populated.
				subtype: (m && typeof m.subtype === 'string') ? m.subtype : ''
			};
		};

		const onAdd = (m) => { try { push(msgRow('` + string(MessageAdded) + `', m)); } catch (e) { s.dropped++; } };
		const onAck = (m) => { try { push(msgRow('` + string(MessageAck) + `', m)); } catch (e) { s.dropped++; } };
		// A REVOKED MESSAGE IS RECOGNISED BY THE SAME PREDICATE THE REVOKE
		// CAPABILITY USES, not by one field. Listening to change:isRevokedMsg
		// alone was measured NOT firing; the capability's own postcondition
		// checks three signals because this build does not agree with itself
		// about which one moves. The handler runs on all three and emits only
		// when the message actually reads as revoked, so change:type — which
		// fires for other reasons — cannot produce a false one.
		const looksRevoked = (m) => !!(m && (m.isRevokedMsg || m.type === 'revoked' || m.revokeSender));
		const onRevoke = (m) => {
			try { if (looksRevoked(m)) { push(msgRow('` + string(MessageRevoked) + `', m)); } }
			catch (e) { s.dropped++; }
		};
		// A REMOCAO E' FILTRADA POR isNewMsg, e o filtro nao e' cosmetico: a
		// colecao remove modelos por conta propria quando descarrega conversa
		// antiga, e sem o filtro cada despejo viraria "alguem apagou isto".
		// A referencia filtra no mesmo ponto, pelo mesmo motivo.
		const onRemove = (m) => {
			try { if (m && m.isNewMsg) { push(msgRow('` + string(MessageRemoved) + `', m)); } }
			catch (e) { s.dropped++; }
		};
		const onEdit = (m) => { try { push(msgRow('` + string(MessageEdited) + `', m)); } catch (e) { s.dropped++; } };
		const onReaction = (m) => { try { push(msgRow('` + string(MessageReaction) + `', m)); } catch (e) { s.dropped++; } };
		const onContact = (c) => {
			try {
				push({ type: '` + string(ContactChanged) + `',
					// A CONTACT HAS NO CHAT AND NO MESSAGE. Its identity goes in
					// chat because that is the field callers route on, and the
					// alternative — a second identity field used by one type —
					// is worse than one field whose doc says what it holds.
					chat: (c && c.id && c.id._serialized) || '',
					msg: '', fromMe: false, kind: 'contact', ack: 0, bodyLen: 0 });
			} catch (e) { s.dropped++; }
		};
		const onChat = (c) => {
			try {
				push({ type: '` + string(ChatChanged) + `',
					chat: (c && c.id && c.id._serialized) || '',
					msg: '', fromMe: false, kind: '', ack: 0, bodyLen: 0 });
			} catch (e) { s.dropped++; }
		};

		// The names are the page's own collection events. 'change:ack' is the
		// narrow one — subscribing to 'change' would deliver every field of
		// every message and fill the buffer with noise.
		MC.on('add', onAdd);
		MC.on('change:ack', onAck);
		// NARROW FIELDS, not 'change'. Subscribing to every field of every
		// message would fill the buffer with noise and make a drop mean
		// nothing; these are the fields the corresponding capability moves.
		MC.on('change:isRevokedMsg', onRevoke);
		MC.on('change:revokeSender', onRevoke);
		MC.on('change:type', onRevoke);
		MC.on('remove', onRemove);
		MC.on('change:latestEditMsgKey', onEdit);
		// hasReaction is STICKY (H53), so this fires when a reaction is ADDED
		// and, on the same session, may not fire again when it is taken back.
		// That asymmetry belongs in the type's doc, not in a silent gap.
		MC.on('change:hasReaction', onReaction);
		const onCall = (c) => {
			if (!c) { return; }
			push({
				type: 'call.incoming',
				call: (c.id && c.id._serialized) || (typeof c.id === 'string' ? c.id : ''),
				peer: (c.peerJid && c.peerJid._serialized) ||
					(typeof c.peerJid === 'string' ? c.peerJid : ''),
				video: !!c.isVideo,
				outgoing: !!c.outgoing,
				group: !!c.isGroup,
			});
		};
		// A COLECAO DE CONVERSAS TEM DUAS PORTAS, e so' uma estava aberta.
		// 'change' diz que a conversa mexeu; 'remove' diz que ela SUMIU, e sem o
		// segundo um apagamento chegava como mais quatorze 'change' (H173).
		const onChatRemove = (c) => {
			try {
				push({ type: '` + string(ChatRemoved) + `',
					chat: (c && c.id && c.id._serialized) || '',
					msg: '', fromMe: false, kind: '', ack: 0, bodyLen: 0 });
			} catch (e) { s.dropped++; }
		};
		CC.on('change', onChat);
		CC.on('remove', onChatRemove);
		s.handlers = [[MC, 'add', onAdd], [MC, 'change:ack', onAck],
			[MC, 'change:isRevokedMsg', onRevoke], [MC, 'change:revokeSender', onRevoke],
			[MC, 'change:type', onRevoke], [MC, 'change:latestEditMsgKey', onEdit],
			[MC, 'change:hasReaction', onReaction],
			[CC, 'change', onChat], [CC, 'remove', onChatRemove]];

		// The contact collection is optional: a build without it should give a
		// bus with five types, not a boot failure.
		try {
			const CT = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			if (CT && typeof CT.on === 'function') {
				CT.on('change', onContact);
				s.handlers.push([CT, 'change', onContact]);
			}
		} catch (e) {}

		// THE CALL COLLECTION, and the listener is tried BEFORE the reference's
		// technique is considered.
		//
		// whatsapp-web.js observes incoming calls by finding the property of
		// WAWebCallCollection that IS a Map and wrapping its set method — a
		// patch to the running application. It presumably found no listener.
		// This build's collection DOES expose .on (measured), so the clean door
		// is tried first and, if it never fires, that is a measurement rather
		// than a reason to start patching somebody else's object.
		//
		// A CALL CARRIES THE CALLER'S NUMBER, so the row keeps the peer jid for
		// routing — the same treatment every other event's identity gets — and
		// never the participants, the display name, or anything a log would
		// render.
		try {
			const RAW = window.require('` + string(spa.ModuleCallCollection) + `');
			const CL = RAW && (RAW.CallCollection || RAW.default || RAW);
			if (CL && typeof CL.on === 'function') {
				CL.on('add', onCall);
				CL.on('change:isRinging', onCall);
				s.handlers.push([CL, 'add', onCall], [CL, 'change:isRinging', onCall]);
			}
		} catch (e) {}
		// THE POLL VOTE COLLECTION, found the same way and for the same reason.
		//
		// The reference has no listener for votes either: it patches
		// pollVoteTableMode.bulkUpsert and reads the arguments in flight. This
		// build exposes WAWebCollections.PollVote with on/off/getModelsArray
		// (measured 2026-08-22), so the clean door is used and the page is left
		// alone — H112's rule, unchanged.
		//
		// A VOTE CARRIES WHO VOTED AND FOR WHAT. Neither crosses: the row keeps
		// the poll's message id and the voter jid for routing, and never the
		// option text, which is content the poll author wrote.
		try {
			const CO = window.require('` + string(spa.ModuleCollections) + `');
			const PV = CO && CO.PollVote;
			if (PV && typeof PV.on === 'function') {
				const onVote = (v) => {
					try {
						const pk = v && (v.parentMsgKey || v.msgKey);
						push({
							type: '` + string(VoteUpdated) + `',
							chat: (pk && pk.remote && pk.remote._serialized) || '',
							msg: (pk && pk.id) || '',
							peer: (v && v.sender && v.sender._serialized) || '',
							fromMe: !!(pk && pk.fromMe)
						});
					} catch (e) { s.dropped++; }
				};
				PV.on('add', onVote);
				PV.on('change', onVote);
				s.handlers.push([PV, 'add', onVote], [PV, 'change', onVote]);
			}
		} catch (e) {}
		s.installed = true;
		window[KEY] = s;
		return { installed: true, already: false };
	})())`
}

const drainScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s || !s.installed) { return { ok: false, rows: [], dropped: 0, seen: 0 }; }
	const rows = s.buf;
	s.buf = [];
	return { ok: true, rows: rows, dropped: s.dropped, seen: s.seen };
})())`

const uninstallScript = `JSON.stringify((() => {
	const KEY = ` + `"` + stateKey + `"` + `;
	const s = window[KEY];
	if (!s || !s.installed) { return { removed: false }; }
	try {
		for (const h of (s.handlers || [])) {
			try { if (h[0] && typeof h[0].off === 'function') { h[0].off(h[1], h[2]); } } catch (e) {}
		}
	} catch (e) {}
	s.installed = false;
	s.handlers = [];
	s.buf = [];
	window[KEY] = undefined;
	return { removed: true };
})())`

// LiveWindow is how recent a message's OWN timestamp must be for its arrival to
// count as news.
//
// It is not a rate heuristic and the difference matters: a rate heuristic asks
// "have events slowed down?", which the hydration burst would answer wrong. This
// asks "was this message created at essentially the moment it was announced?",
// which is a property of the message and is false for every historical one,
// however fast or slow they arrive.
//
// Sixty seconds is generous on purpose. The cost of being too generous is that a
// message delivered after a minute of queueing reads as live, which is what it
// is; the cost of being too tight would be calling real arrivals history.
var LiveWindow int64 = 60

// ageSeconds is how old the message was when the page announced it. It can be
// NEGATIVE when the sender's clock runs ahead, which is why the window is
// applied in both directions rather than as a floor.
func ageSeconds(msgT, atMillis int64) int64 {
	if msgT <= 0 || atMillis <= 0 {
		return 0
	}
	return atMillis/1000 - msgT
}

// classify decides an event's freshness.
//
// THE ONLY WAY TO REACH LIVE IS THROUGH EVIDENCE ABOUT THE EVENT ITSELF. There
// is deliberately no branch here that reads a clock, a counter or a rate: the
// whole defect this replaces was a window that closed on a schedule the page did
// not keep to.
func classify(e Event, firstDrain bool) Freshness {
	if firstDrain {
		// The page had these before this pump existed. Whatever they are, this
		// subscriber did not cause them and cannot have been waiting for them.
		return FreshnessReplay
	}
	if e.Type == MessageAdded && e.Aged {
		if e.AgeSeconds <= LiveWindow && e.AgeSeconds >= -LiveWindow {
			return FreshnessLive
		}
		return FreshnessReplay
	}
	// NO CAUSAL DISCRIMINATOR, NO CLAIM. chat.changed, message.ack and the rest
	// carry nothing that separates hydration from news, so they say so. This is
	// the branch the instruction was about: they stay UNKNOWN until there is
	// evidence, not until enough time has passed.
	return FreshnessUnknown
}

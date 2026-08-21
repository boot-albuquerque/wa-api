package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
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

const stateKey = "__waHeadlessEventBus"

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
			e.Replay = firstAfterInstall
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
		known := false
		for _, k := range KnownTypes {
			if k == t {
				known = true
				break
			}
		}
		if !known {
			continue
		}
		evs = append(evs, Event{
			Type: t, Seq: r.Seq, At: time.UnixMilli(r.At),
			ChatJID: r.Chat, MessageID: r.Msg, FromMe: r.FromMe,
			Kind: r.Kind, Ack: r.Ack, BodyLen: r.BodyLen,
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
				bodyLen: (m && typeof m.body === 'string') ? m.body.length : 0
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
		const onEdit = (m) => { try { push(msgRow('` + string(MessageEdited) + `', m)); } catch (e) { s.dropped++; } };
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
		MC.on('change:latestEditMsgKey', onEdit);
		CC.on('change', onChat);
		s.handlers = [[MC, 'add', onAdd], [MC, 'change:ack', onAck],
			[MC, 'change:isRevokedMsg', onRevoke], [MC, 'change:revokeSender', onRevoke],
			[MC, 'change:type', onRevoke], [MC, 'change:latestEditMsgKey', onEdit],
			[CC, 'change', onChat]];

		// The contact collection is optional: a build without it should give a
		// bus with five types, not a boot failure.
		try {
			const CT = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			if (CT && typeof CT.on === 'function') {
				CT.on('change', onContact);
				s.handlers.push([CT, 'change', onContact]);
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

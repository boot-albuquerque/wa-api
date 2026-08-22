// Package messagemeta answers the product's onMessageMeta: a stream of message
// METADATA, never content.
//
// THE CONTRACT IT SERVES. HANDOFF §C5 and invariant 12: WaMessageMeta carries
// waJid, waMessageId, direction, type and waTimestamp, and NEVER a body, text
// or media. §C6 makes a raw waJid in a log a blocker. Both are enforced here
// rather than trusted to callers — the page script builds the metadata object
// field by field, so a body cannot arrive by accident, and Meta redacts itself
// when rendered.
//
// HOW EVENTS GET OUT OF THE PAGE. The page subscribes to the message collection
// once and appends metadata to a bounded array; Go drains it on its own clock.
// The alternative — a CDP binding pushing each event — is what whatsapp-web.js
// does via puppeteer's exposeFunction, and it would mean building event
// plumbing this module does not have. The pull design also keeps invariant 6
// satisfied by construction: the only clock is on the Go side.
//
// THE PRICE, PAID OUT LOUD. A bounded buffer can drop. Every drop is counted in
// the page and reported on the next drain, because a metadata stream that loses
// events silently is worse than no stream at all: it looks complete.
package messagemeta

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// stateKey is where the page keeps the subscription. Named, not spelled twice.
//
// IT CONTINUA SENDO UMA CHAVE ÚNICA, DE PROPÓSITO, e esta é a única capacidade
// do módulo que não recebeu a chave por chamada da H177.
//
// A DIFERENÇA É O QUE A CHAVE GUARDA. Nas outras, ela guarda a RESPOSTA de uma
// chamada, e duas chamadas concorrentes escreviam a mesma variável — medido a 12
// cruzamentos em 12 rodadas. Aqui ela guarda uma ASSINATURA de longa duração:
// `installScript` a instala uma vez (e sai cedo se já estiver instalada) e
// `drainScript` esvazia o buffer dela. Há UMA assinatura por sessão por desenho,
// e dar-lhe uma chave por chamada quebraria exatamente isso — o install
// escreveria uma chave e o drain leria outra.
const stateKey = "__waHeadlessMsgMeta"

// DefaultBufferSize bounds the page-side queue.
//
// It is a CAPACITY, not a rate: it decides how many events may arrive between
// two drains before the oldest are refused. There is no measurement of real
// message rate for this account behind it, so it is not presented as one —
// what makes the number safe is that exceeding it is COUNTED and reported, not
// that it was chosen well.
const DefaultBufferSize = 500

// Direction is who sent the message.
type Direction string

const (
	// DirectionIn is a message received.
	DirectionIn Direction = "in"
	// DirectionOut is a message this account sent.
	DirectionOut Direction = "out"
)

// MessageID is the identifier, IN PARTS.
//
// A CONSCIOUS DIVERGENCE from whatsapp-web.js, and one forced by measurement:
// wwebjs uses msg.id._serialized, and PARIDADE-WWEBJS.md §6.4 measured that
// accessor answering NULL in this build. Copying their code would have produced
// an empty id on every message, and empty is indistinguishable from "no id".
//
// So the parts are carried as the page hands them over, and nothing is
// concatenated here. If _serialized ever comes back it becomes one more source
// to reconcile against, not the only one.
type MessageID struct {
	// ID is the raw per-message identifier (msg.id.id).
	ID string `json:"id"`
	// FromMe is msg.id.fromMe, kept because it is part of what identifies a
	// message, not only of its direction.
	FromMe bool `json:"from_me"`
	// RemoteJID is the chat this message belongs to, serialized.
	RemoteJID string `json:"remote_jid"`
}

// Present reports whether an identifier arrived at all.
func (m MessageID) Present() bool { return m.ID != "" }

// String redacts: the remote jid is PII under §C6.
func (m MessageID) String() string {
	if !m.Present() {
		return "MessageID(absent)"
	}
	return fmt.Sprintf("MessageID(id=%s from_me=%v remote=<redacted>)", m.ID, m.FromMe)
}

// Meta is one message's metadata. No body, ever.
type Meta struct {
	// JID is the counterparty/chat identifier. PII: redacted by String().
	JID string `json:"jid"`
	// ID identifies the message.
	ID MessageID `json:"id"`
	// Direction is in or out.
	Direction Direction `json:"direction"`
	// Type is the message type as the page reports it ("chat", "image", ...).
	// It is a category, not content, so it is safe to log.
	Type string `json:"type"`
	// Timestamp is when the message is stamped, from msg.t (unix seconds).
	Timestamp time.Time `json:"timestamp"`
}

// String redacts the jid and carries no body by construction.
func (m Meta) String() string {
	return fmt.Sprintf("Meta(jid=<redacted> id=%s dir=%s type=%s at=%s)",
		m.ID.ID, m.Direction, m.Type, m.Timestamp.UTC().Format(time.RFC3339))
}

// GoString redacts too, so %#v is no leakier than %v.
func (m Meta) GoString() string { return m.String() }

// Drain is one collection of what the page buffered since the last one.
type Drain struct {
	// Events is the metadata, oldest first.
	Events []Meta
	// Dropped is how many events the page REFUSED since the last drain because
	// the buffer was full. Non-zero means this stream has a hole in it, and the
	// caller must treat the sequence as incomplete rather than merely short.
	Dropped int
	// Seen is how many events the subscription observed in total since it was
	// installed, drops included. It is the denominator that makes Dropped
	// interpretable.
	Seen int
	// Reinstalled is true when the subscription had to be put back because the
	// page no longer had it — a reload or a navigation. EVENTS WERE LOST in
	// that gap and there is no way to know how many, which is why this is a
	// distinct field and not folded into Dropped.
	Reinstalled bool
}

// Complete reports whether this drain represents an unbroken sequence.
func (d Drain) Complete() bool { return d.Dropped == 0 && !d.Reinstalled }

// ErrNotInstalled is a drain against a page that has no subscription and could
// not be given one.
var ErrNotInstalled = fmt.Errorf("messagemeta: the subscription is not installed")

// Subscription owns the page-side subscription for one session.
type Subscription struct {
	runner *engine.Runner
	eval   spa.Evaluator
	size   int
}

// New prepares a subscription. size <= 0 uses DefaultBufferSize.
func New(runner *engine.Runner, eval spa.Evaluator, size int) *Subscription {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &Subscription{runner: runner, eval: eval, size: size}
}

// metaExpr builds the metadata object, field by field.
//
// Written as an explicit allow-list rather than by copying the model: a message
// model in this build carries __x_body among ~60 own keys, and any "copy then
// delete the secrets" shape would ship the body the first time Meta adds a
// field nobody thought about. The allow-list fails closed.
const metaExpr = metaExprShared

// MetaExpr exposes the allow-list to sibling capabilities that must produce the
// SAME metadata shape — capabilities/fetchmessages, today.
//
// It is exported rather than copied because a second allow-list is how a body
// eventually ships: two lists drift, someone adds a field to one, and the
// invariant holds in the place that is tested and fails in the place that is
// not. One list, one test, one place to be wrong.
const MetaExpr = metaExprShared

// DecodeWire turns one raw metadata object from MetaExpr into a Meta. It is
// exported for the same reason as MetaExpr: the decoding must not exist twice
// either, or the two capabilities can disagree about what an absent timestamp
// or an unknown direction means.
func DecodeWire(raw []byte) ([]Meta, error) {
	var events []wireMeta
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, fmt.Errorf("messagemeta: decoding events: %w", err)
	}
	return decode(wireDrain{Events: events}).Events, nil
}

const metaExprShared = `(function (m) {
	var wid = function (w) { return (w && typeof w._serialized === 'string') ? w._serialized : ''; };
	var key = (m && m.id) || {};
	return {
		jid: wid(key.remote) || wid(m && m.to) || wid(m && m.from),
		id: {
			id: (typeof key.id === 'string') ? key.id : '',
			from_me: !!key.fromMe,
			remote_jid: wid(key.remote)
		},
		direction: key.fromMe ? 'out' : 'in',
		type: (typeof m.type === 'string') ? m.type : '',
		t: (typeof m.t === 'number') ? m.t : 0
	};
})`

func (s *Subscription) installScript() string {
	return `JSON.stringify((() => {
		const KEY = ` + strconv.Quote(stateKey) + `;
		if (window[KEY] && window[KEY].installed) {
			return { installed: true, already: true };
		}
		const c = window.require('` + string(spa.ModuleMsgCollection) + `');
		const coll = c && c.MsgCollection;
		if (!coll || typeof coll.on !== 'function') {
			return { installed: false, reason: 'NO_COLLECTION' };
		}
		const meta = ` + metaExpr + `;
		const state = { buf: [], dropped: 0, seen: 0, installed: false, cap: ` +
		strconv.Itoa(s.size) + ` };
		state.handler = function (m) {
			state.seen++;
			if (state.buf.length >= state.cap) { state.dropped++; return; }
			try { state.buf.push(meta(m)); } catch (e) { state.dropped++; }
		};
		coll.on('` + string(eventAdd) + `', state.handler);
		state.installed = true;
		window[KEY] = state;
		return { installed: true, already: false };
	})())`
}

// eventAdd is the collection event this subscribes to.
//
// TAKEN FROM whatsapp-web.js's understanding (it listens for 'add' on the
// message collection) and now VERIFIED against this build.
//
// This comment used to say the opposite — that confirming it required a human
// to send a message to the lab account, and that until then silence must not be
// read as proof. That was true when written and stopped being true on
// 2026-08-20, when a second account was paired: TestRealSPASendAndReceiveBetween
// Accounts sends from one and observes the arrival on the other, with NO human
// involved. The receiving half came through this subscription, carrying the
// SAME message id the sender returned, one second after the send.
//
// The correction is recorded rather than quietly applied, because a stale
// "unverified" is its own hazard: it invites someone to re-do work that is
// done, or to distrust a path that has evidence.
const eventAdd = "add"

var drainScript = `JSON.stringify((() => {
	const s = window[` + strconv.Quote(stateKey) + `];
	if (!s || !s.installed) { return { installed: false }; }
	const out = { installed: true, events: s.buf, dropped: s.dropped, seen: s.seen };
	s.buf = [];
	s.dropped = 0;
	return out;
})())`

type wireMeta struct {
	JID string `json:"jid"`
	ID  struct {
		ID        string `json:"id"`
		FromMe    bool   `json:"from_me"`
		RemoteJID string `json:"remote_jid"`
	} `json:"id"`
	Direction string `json:"direction"`
	Type      string `json:"type"`
	T         int64  `json:"t"`
}

type wireDrain struct {
	Installed bool       `json:"installed"`
	Events    []wireMeta `json:"events"`
	Dropped   int        `json:"dropped"`
	Seen      int        `json:"seen"`
}

type wireInstall struct {
	Installed bool   `json:"installed"`
	Already   bool   `json:"already"`
	Reason    string `json:"reason"`
}

// Install puts the subscription in the page. It is idempotent: a page that
// already has one is left alone, so re-installing after a drain cannot produce
// two handlers counting the same message twice.
func (s *Subscription) Install(ctx context.Context, label string) error {
	var raw string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return s.eval(ctx, s.installScript(), &raw)
	}); err != nil {
		return fmt.Errorf("messagemeta: installing: %w", err)
	}
	var got wireInstall
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return fmt.Errorf("messagemeta: unexpected install answer: %w", err)
	}
	if !got.Installed {
		return fmt.Errorf("%w: %s", ErrNotInstalled, got.Reason)
	}
	return nil
}

// Drain collects what the page buffered.
//
// If the subscription is gone — a reload dropped the window state — it is put
// back and the returned Drain says Reinstalled. That flag is not cosmetic: the
// events between the reload and the reinstall are gone and UNCOUNTABLE, so a
// caller must treat the sequence as broken rather than merely empty.
func (s *Subscription) Drain(ctx context.Context, label string) (Drain, error) {
	raw, err := s.readDrain(ctx, label)
	if err != nil {
		return Drain{}, err
	}
	if !raw.Installed {
		if err := s.Install(ctx, label+"/reinstall"); err != nil {
			return Drain{}, err
		}
		again, err := s.readDrain(ctx, label+"/after-reinstall")
		if err != nil {
			return Drain{}, err
		}
		out := decode(again)
		out.Reinstalled = true
		return out, nil
	}
	return decode(raw), nil
}

func (s *Subscription) readDrain(ctx context.Context, label string) (wireDrain, error) {
	var raw string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return s.eval(ctx, drainScript, &raw)
	}); err != nil {
		return wireDrain{}, fmt.Errorf("messagemeta: draining: %w", err)
	}
	var got wireDrain
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return wireDrain{}, fmt.Errorf("messagemeta: unexpected drain answer: %w", err)
	}
	return got, nil
}

func decode(w wireDrain) Drain {
	out := Drain{Dropped: w.Dropped, Seen: w.Seen}
	for _, e := range w.Events {
		m := Meta{
			JID:       e.JID,
			Type:      e.Type,
			Direction: DirectionIn,
		}
		if e.Direction == string(DirectionOut) {
			m.Direction = DirectionOut
		}
		m.ID = MessageID{ID: e.ID.ID, FromMe: e.ID.FromMe, RemoteJID: e.ID.RemoteJID}
		if e.T > 0 {
			m.Timestamp = time.Unix(e.T, 0).UTC()
		}
		out.Events = append(out.Events, m)
	}
	return out
}

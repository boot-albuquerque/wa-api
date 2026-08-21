// Package events is the one way things that happen in the page reach Go.
//
// WHY IT EXISTS AS A SINGLE MECHANISM. The upstream has 31 events and this
// module had two subscriptions, each written from scratch, each with its own
// buffer and its own idea of what a drop is. Adding the other 23 that way would
// be 23 installs racing to the same page, 23 buffers with unrelated ceilings,
// and no way to say what order anything happened in.
//
// So there is ONE installer on the page, ONE buffer, and one sequence counter.
// Ordering across event types survives the boundary because the page stamps it,
// and a drop means the same thing for every subscriber.
//
// THE PAGE IS NOT A PLACE TO KEEP STATE YOU CANNOT AFFORD TO LOSE. A reload
// wipes the installed handlers and everything buffered. Rather than detect
// reloads — which needs a signal this module does not have — the pump
// REINSTALLS on every cycle. The install is idempotent and answers "already"
// in the common case, so the cost is one cheap call per poll and the reward is
// that recovery needs no detection at all.
//
// WHAT IT DELIBERATELY DOES NOT DO: it does not let capabilities talk to each
// other. A capability may observe the Hub; none of them may depend on another
// capability having run. That keeps the dependency graph a tree with the page
// at its root, which is what makes any of this testable.
package events

import (
	"fmt"
	"sync"
	"time"
)

// Type names something that happened.
//
// The values are this module's own, not the upstream's strings: equivalence is
// semantic, and borrowing "message_create" would promise a shape this build
// does not necessarily produce.
type Type string

const (
	// MessageAdded is a message appearing in the collection, sent or received.
	MessageAdded Type = "message.added"
	// MessageAck is a delivery state moving on a message this account sent.
	MessageAck Type = "message.ack"
	// ChatChanged is a conversation's own fields moving — unread, archived,
	// pinned, muted.
	ChatChanged Type = "chat.changed"
	// MessageRevoked is a message deleted for everyone, seen from either side.
	MessageRevoked Type = "message.revoked"
	// MessageEdited is a message whose text was replaced.
	MessageEdited Type = "message.edited"
	// ContactChanged is a contact record moving — a name, a picture, a
	// presence-adjacent field.
	ContactChanged Type = "contact.changed"
	// MessageReaction is somebody reacting to a message, or taking it back.
	//
	// IT ANSWERS "SOMETHING HAPPENED", NOT "WHAT IT IS NOW. The flag it rides on
	// is sticky within a session (H53) and the aggregate that would say which
	// emoji has no source on this build (H83). A subscriber learns that a
	// message's reactions moved and has nowhere to read them from — which is
	// worth delivering anyway, because "go look" is more than silence, and is
	// stated here rather than discovered.
	MessageReaction Type = "message.reaction"
	// CallIncoming is a call appearing in this session's call collection.
	//
	// IT IS THE ONE TYPE HERE THAT HAS NEVER BEEN SEEN FIRING, and saying so is
	// the rule rather than an exception to it. ContactChanged sat in exactly
	// this position until H90 gave it a trigger; this one is waiting for the
	// same thing. The listener is installed because it costs nothing and is the
	// door that a solution would come through — but nobody should read its
	// presence as a delivered capability (H93).
	//
	// IT COVERS BOTH DIRECTIONS and says which. A call this account PLACED
	// arrives through the same collection as one it received, and a subscriber
	// that treated every row as somebody calling would answer its own calls.
	// OutgoingCall carries the difference.
	CallIncoming Type = "call.incoming"

	// SessionReady is a session reaching a VERIFIED ready: the page classified
	// APP_READY and the module inventory passed on that same boot. It is not
	// "the browser started".
	SessionReady Type = "session.ready"
	// SessionBootFailed is a boot that did not reach ready. Reason is the boot
	// STAGE, which is what says whether the repair is waiting, relaunching, or
	// a human with a phone.
	SessionBootFailed Type = "session.boot_failed"
	// SessionStopped is a session torn down through the module's one shutdown
	// path, carrying how it went (Reason is the StopVia).
	SessionStopped Type = "session.stopped"
	// SessionStateChanged is a liveness verdict that DIFFERS from the previous
	// one. Every probe produces a verdict; only a transition is an event,
	// because a bus that repeats "still alive" twice a second is a heartbeat
	// wearing an event's clothes.
	SessionStateChanged Type = "session.state_changed"
)

// Freshness says whether an event is NEWS.
//
// WHY THREE STATES AND NOT A BOOLEAN. A boolean forces every event into either
// "history" or "live", and this bus cannot honestly make that call for most of
// them. Measured (H93): a freshly booted session pushes ~1170 messages and ~2000
// chat changes through in the first eight seconds, all of them hydration, and
// the replay window that was supposed to cover it closes after the FIRST drain.
//
// The explicit signal was looked for and does not exist for this purpose (H96):
// WAWebHistorySyncProgressGetters answers inProgress=false and progress=100
// from the first second, because it tracks the SERVER's history sync for the
// account, not this session's local hydration. getInitialHistorySyncComplete is
// a persisted account flag and reads true before anything has loaded.
//
// So the third state is the honest one, and the rule that comes with it is
// absolute: UNKNOWN IS NEVER PROMOTED TO LIVE BY TIME OR BY RATE. "The events
// stopped arriving so the rest must be live" is precisely the heuristic that
// would make this bus lie again, more quietly.
type Freshness string

const (
	// FreshnessReplay is history: the page had it before this pump existed.
	FreshnessReplay Freshness = "replay"
	// FreshnessLive is news, PROVEN so by something causal about the event
	// itself — not by when it arrived.
	FreshnessLive Freshness = "live"
	// FreshnessUnknown is an event this bus cannot classify. It is not a
	// failure and it is not a maybe-live: it is the accurate answer for a type
	// with no causal discriminator, and a consumer that must not double-count
	// has to treat it as history.
	FreshnessUnknown Freshness = "unknown"
)

// Source says where an event came from, because the two origins do not share a
// clock or a sequence and a subscriber that sorts by Seq needs to know which
// events are even comparable.
type Source string

const (
	// SourcePage is the SPA's own collections, sequenced in the page.
	SourcePage Source = "page"
	// SourceLocal is this process observing itself — a boot, a stop, a
	// liveness transition. It has NO page sequence, so Seq is zero and stays
	// zero: inventing one would put lifecycle facts into an order they were
	// never measured in.
	SourceLocal Source = "local"
)

// PageTypes is every type the ingress installs a handler for. It exists so a
// test can assert the page side and the Go side agree, which is the failure
// nobody notices: a handler installed for an event nothing subscribes to, or a
// subscription for an event never installed.
var PageTypes = []Type{
	MessageAdded, MessageAck, ChatChanged,
	MessageRevoked, MessageEdited, ContactChanged, MessageReaction,
	CallIncoming,
}

// LocalTypes is every type published from Go rather than from the page.
//
// THE SPLIT IS NOT BOOKKEEPING. The page/Go agreement test asserts that every
// name in the install script has a Type and back; a lifecycle name would fail
// that test for the right reason — the page really does not emit it — and the
// only way to keep the test honest is to say which list it governs.
var LocalTypes = []Type{
	SessionReady, SessionBootFailed, SessionStopped, SessionStateChanged,
}

// KnownTypes is every type this bus can deliver, from either origin.
var KnownTypes = append(append([]Type{}, PageTypes...), LocalTypes...)

// EVERY TYPE HERE IS ONE THIS MODULE CAN TRIGGER AND HAS TRIGGERED. The upstream
// has 31 events and it would be easy to declare 31 names, install 31 listeners,
// and ship a bus whose quiet halves nobody notices. A name that has never been
// seen firing is a promise, not a capability — so a type is added when a live
// test can make it happen on demand, and not before.
//
// THE RULE COST THE LIFECYCLE FAMILY FOUR NAMES. The upstream has nine session
// events; this bus has four, because AUTHENTICATED, CODE_RECEIVED,
// LOADING_SCREEN and REMOTE_SESSION_SAVED have no observable in this build —
// there is no pairing slice, no remote store, and the settle loop measures page
// CLASSES rather than load progress. Declaring them would have cost nothing and
// bought a bus that looks complete. They are in the ledger with that reason
// written down instead (H88).

// Event is one thing that happened.
//
// IT CARRIES NO CONTENT. A message body, a contact name and a phone number are
// all things this module refuses to move around, and an event bus is exactly
// where that refusal would erode first — it is the one place every capability
// looks. Identity is a jid, which callers need to route on; bodies are lengths.
type Event struct {
	Type Type
	// Origin says whether the page produced this or this process did. It is
	// always set on delivery; a zero Origin reaching a subscriber is a bug in
	// whoever built the event, and a test asserts both producers fill it.
	Origin Source
	// Seq is assigned IN THE PAGE, before the boundary. Two events observed by
	// different subscribers can be ordered against each other because of it,
	// and a gap in it is a dropped event that nobody has to guess about.
	Seq int64
	// At is when it was seen: the PAGE's clock for SourcePage, Go's for
	// SourceLocal. Two events from different origins are therefore not
	// comparable by this field, which is why Origin exists. It is reported for
	// diagnosis and never used to decide anything — invariant 6 keeps decisions
	// on this side.
	At time.Time
	// Fresh says whether this event is news; see Freshness for why it has three
	// values rather than two.
	Fresh Freshness
	// Replay is the conservative reading of Fresh, kept as a field so that every
	// consumer written before Freshness existed keeps working — and keeps
	// working SAFELY.
	//
	// It is true for anything not PROVEN live, which means an UNKNOWN event
	// reads as replay. That is deliberate and it is the direction that cannot
	// hurt: a consumer skipping replay may now skip something that was in fact
	// new, and one that trusted the old boolean would otherwise have counted
	// eleven hundred historical messages as arrivals on every boot.
	Replay bool

	// ChatJID is which conversation it happened in. Empty when not applicable.
	ChatJID string
	// MessageID is the message it concerns. Empty when not applicable.
	MessageID string
	// FromMe says whose message it is, for the message events.
	FromMe bool
	// Kind is the message type ("chat", "image", …) for MessageAdded.
	Kind string
	// Ack is the delivery state for MessageAck, as the page's own number.
	Ack int
	// BodyLen is the message body's length in UTF-16 units — the page's own
	// measure — never the body.
	BodyLen int
	// Aged says the message carried its own timestamp, which is what makes
	// AgeSeconds meaningful.
	//
	// IT EXISTS BECAUSE ZERO IS AMBIGUOUS, and the first version of the
	// classifier was wrong about exactly that: it guarded on AgeSeconds > 0,
	// treating "no timestamp" and "created in the same second it was announced"
	// as the same thing — and the second is the FRESHEST case there is. A
	// message sent and delivered within one second read as UNKNOWN.
	Aged bool
	// AgeSeconds is how old the MESSAGE ITSELF was when the page announced it,
	// from the message's own timestamp. It is the causal discriminator behind
	// FreshnessLive for message.added: a message created two days ago and
	// announced now is hydration, whatever the clock says about the
	// announcement. Zero when the type carries no message.
	AgeSeconds int64

	// CallID and CallerJID identify a call, for the call events. A caller is a
	// phone number, carried for routing and never rendered.
	CallID, CallerJID string
	// Video says the call is a video call; OutgoingCall says this account
	// placed it; GroupCall says it has more than two people.
	Video, OutgoingCall, GroupCall bool

	// State is the lifecycle state for the session.* events: the liveness
	// signal for SessionStateChanged, empty otherwise.
	State string
	// Reason is why, for the session.* events — a boot stage, a StopVia, a
	// page class. It is a CLOSED vocabulary from this module's own constants,
	// never a formatted error: an error string is where a profile path or a
	// jid would eventually leak into the one place every capability reads.
	Reason string
}

func (e Event) String() string {
	return fmt.Sprintf("events.Event(type=%s origin=%s fresh=%s seq=%d replay=%t chat=%t msg=%t fromMe=%t kind=%s ack=%d bodyLen=%d call=%t caller=%t video=%t outgoingCall=%t state=%s reason=%s)",
		e.Type, e.Origin, e.Fresh, e.Seq, e.Replay, e.ChatJID != "", e.MessageID != "", e.FromMe, e.Kind, e.Ack, e.BodyLen,
		e.CallID != "", e.CallerJID != "", e.Video, e.OutgoingCall, e.State, e.Reason)
}

// Handler receives one event. It runs on the Hub's delivery goroutine, so a
// handler that blocks blocks every other subscriber — which is why Subscribe
// says so and why Stats counts what a slow handler costs.
type Handler func(Event)

// Stats is what the bus can say about itself.
type Stats struct {
	// Delivered is how many events reached at least one subscriber.
	Delivered int64
	// DroppedInPage is how many the page threw away because its buffer was
	// full. It is the number that matters: those events do not exist anywhere.
	DroppedInPage int64
	// SeenInPage is how many the page's handlers ever saw, dropped or not.
	SeenInPage int64
	// Gaps is how many times the sequence jumped, which is the same fact as
	// DroppedInPage seen from Go rather than from the page. They are reported
	// separately on purpose: if they disagree, one of the two is lying.
	Gaps int64
	// Reinstalls is how many times the ingress had to be put back, which is a
	// proxy for how often the page reloaded under us.
	Reinstalls int64
	// Subscribers is how many handlers are attached right now.
	Subscribers int
}

func (s Stats) String() string {
	return fmt.Sprintf("events.Stats(delivered=%d droppedInPage=%d seenInPage=%d gaps=%d reinstalls=%d subscribers=%d)",
		s.Delivered, s.DroppedInPage, s.SeenInPage, s.Gaps, s.Reinstalls, s.Subscribers)
}

type subscription struct {
	id      int64
	types   map[Type]bool
	handler Handler
}

// Hub fans page events out to subscribers.
type Hub struct {
	mu      sync.Mutex
	subs    map[int64]*subscription
	nextID  int64
	stats   Stats
	lastSeq int64
	closed  bool

	// The page's own counters reset when the page reloads, so what it reports
	// is only ever "since the last incarnation". These fold the previous ones
	// in, so Stats never goes BACKWARDS — and a stat that goes backwards is
	// worse than no stat, because the number people quote is the one after the
	// last reload.
	droppedBase, seenBase         int64
	lastPageDropped, lastPageSeen int64
}

// NewHub builds an empty Hub. It does not touch the page; Run does.
func NewHub() *Hub {
	return &Hub{subs: map[int64]*subscription{}}
}

// Subscribe registers a handler for one or more types and returns the way to
// stop it.
//
// THE RETURNED FUNCTION IS THE ONLY WAY TO UNSUBSCRIBE, and calling it twice is
// safe. There is deliberately no Unsubscribe(handler) — comparing functions is
// not something Go does, and an API that looks like it can is a trap.
//
// A HANDLER RUNS ON THE DELIVERY GOROUTINE. Blocking in one delays every other
// subscriber and lets the page's buffer fill, which shows up as DroppedInPage.
// Anything slow belongs behind the caller's own channel.
func (h *Hub) Subscribe(handler Handler, types ...Type) func() {
	if handler == nil {
		return func() {}
	}
	set := map[Type]bool{}
	for _, t := range types {
		set[t] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return func() {}
	}
	h.nextID++
	id := h.nextID
	h.subs[id] = &subscription{id: id, types: set, handler: handler}
	h.stats.Subscribers = len(h.subs)

	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subs, id)
			h.stats.Subscribers = len(h.subs)
		})
	}
}

// deliver fans one event out. It is called by the pump.
func (h *Hub) deliver(e Event) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	// A GAP IN THE SEQUENCE IS A DROP, counted here rather than trusted from
	// the page: the two counters are compared in a test, and a disagreement
	// means one of them is wrong.
	if h.lastSeq != 0 && e.Seq > h.lastSeq+1 {
		h.stats.Gaps += e.Seq - h.lastSeq - 1
	}
	if e.Seq > h.lastSeq {
		h.lastSeq = e.Seq
	}
	targets := make([]Handler, 0, len(h.subs))
	for _, s := range h.subs {
		// An empty type set means "everything", which is what a diagnostic
		// subscriber wants and what a test uses.
		if len(s.types) == 0 || s.types[e.Type] {
			targets = append(targets, s.handler)
		}
	}
	if len(targets) > 0 {
		h.stats.Delivered++
	}
	h.mu.Unlock()

	// OUTSIDE THE LOCK. A handler that calls back into Subscribe or Stats would
	// deadlock otherwise, and a handler doing exactly that is the first thing
	// somebody writes.
	for _, fn := range targets {
		fn(e)
	}
}

// PublishSessionState is how a source OUTSIDE the page reports a lifecycle
// fact: a boot that reached ready, a stop, a liveness verdict that moved.
//
// WHY THE HUB HAS A SECOND DOOR AT ALL. Everything else on this bus is
// something the SPA did, and the pump is the only one allowed to say so. But a
// session's own life is not visible from inside the page: the page cannot know
// that its process was launched by a boot that verified an inventory, nor that
// Go has decided to stop it. Those facts exist only here, so they enter here.
//
// THE DIRECTION OF THE DEPENDENCY IS THE POINT. This does not import core, and
// core does not import this. core hands its facts to a callback it declares
// itself, and the composition layer is the only place that knows both sides —
// which is what keeps "how a session is born" out of every capability that
// merely wants to hear about it.
//
// Seq stays zero: see SourceLocal.
func (h *Hub) PublishSessionState(t Type, state, reason string) {
	h.deliver(Event{
		Type: t,
		// A LIFECYCLE FACT IS ALWAYS NEWS. This process observed it happening,
		// in this process, just now — which is the strongest causal evidence
		// anything on this bus has.
		Fresh:  FreshnessLive,
		Origin: SourceLocal,
		At:     time.Now(),
		State:  state,
		Reason: reason,
	})
}

// Stats reports what the bus knows about itself.
func (h *Hub) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stats
}

// Close stops delivery and drops every subscription.
//
// It is what a session teardown calls, and it is idempotent: a Hub closed twice
// is a Hub closed once, because teardown paths run more than once more often
// than anybody plans.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.subs = map[int64]*subscription{}
	h.stats.Subscribers = 0
}

// Closed reports whether Close has run.
func (h *Hub) Closed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

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
)

// KnownTypes is every type the ingress installs a handler for. It exists so a
// test can assert the page side and the Go side agree, which is the failure
// nobody notices: a handler installed for an event nothing subscribes to, or a
// subscription for an event never installed.
var KnownTypes = []Type{
	MessageAdded, MessageAck, ChatChanged,
	MessageRevoked, MessageEdited, ContactChanged, MessageReaction,
}

// EVERY TYPE HERE IS ONE THIS MODULE CAN TRIGGER AND HAS TRIGGERED. The upstream
// has 31 events and it would be easy to declare 31 names, install 31 listeners,
// and ship a bus whose quiet halves nobody notices. A name that has never been
// seen firing is a promise, not a capability — so a type is added when a live
// test can make it happen on demand, and not before.

// Event is one thing that happened.
//
// IT CARRIES NO CONTENT. A message body, a contact name and a phone number are
// all things this module refuses to move around, and an event bus is exactly
// where that refusal would erode first — it is the one place every capability
// looks. Identity is a jid, which callers need to route on; bodies are lengths.
type Event struct {
	Type Type
	// Seq is assigned IN THE PAGE, before the boundary. Two events observed by
	// different subscribers can be ordered against each other because of it,
	// and a gap in it is a dropped event that nobody has to guess about.
	Seq int64
	// At is when the page saw it, in the page's own clock. It is reported for
	// diagnosis and never used to decide anything — invariant 6 keeps decisions
	// on this side.
	At time.Time
	// Replay marks an event the page had buffered before this subscriber
	// existed. A consumer that treats a replayed "message arrived" as new will
	// double-count, and this is the only warning it gets.
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
}

func (e Event) String() string {
	return fmt.Sprintf("events.Event(type=%s seq=%d replay=%t chat=%t msg=%t fromMe=%t kind=%s ack=%d bodyLen=%d)",
		e.Type, e.Seq, e.Replay, e.ChatJID != "", e.MessageID != "", e.FromMe, e.Kind, e.Ack, e.BodyLen)
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

// Package messenger adapts marking-read and reacting to the application's
// ChatMessenger port.
//
// # MarkRead works by HALVES, and which half matters depends on the caller
//
// H160 measured both sides. The LOCAL acknowledgement works and is proven on
// both accounts: unreadCount goes 1 → 0, changed=true, no error. The RECEIPT TO
// THE SENDER does not arrive — with a dual session, marking read in either
// direction leaves the other side's ack at 2 for 60 seconds, and the
// markAvailable hypothesis was tested and REFUTED.
//
// The port is called MarkRead and does not distinguish the two. So:
//
//	a caller clearing its own unread badge      is served
//	a caller expecting the sender to see ticks  is NOT, and it is measured
//
// This adapter satisfies the port because the operation it names does happen.
// The caveat lives here because there is nowhere in the signature to put it, and
// a caller that needs the second half must know it is not getting it.
package messenger

import (
	"context"
	"errors"
	"time"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

// errNoTarget refuses a reaction with nothing to react to. The page needs the
// message key, and an empty one would reach it as a lookup for "".
var errNoTarget = errors.New("waheadless: reaction without a target message")

const (
	markLabel  = "adapter/mark-read"
	reactLabel = "adapter/send-reaction"
)

// marker and reactor are the slices of the page capabilities this adapter uses.
type marker interface {
	MarkRead(ctx context.Context, jid, label string) (waheadless.MarkReadResult, error)
}

type reactor interface {
	Add(ctx context.Context, msgID, emoji, label string) (waheadless.ReactionResult, error)
	Remove(ctx context.Context, msgID, label string) (waheadless.ReactionResult, error)
}

// Messenger implements appport.ChatMessenger over a headless session.
type Messenger struct {
	sessions *adapter.Sessions
	// Overridable in tests. Nil uses the real page capabilities.
	newMarker  func(ctx context.Context, txtID string) (marker, error)
	newReactor func(ctx context.Context, txtID string) (reactor, error)
}

// NewMessenger builds the adapter.
func NewMessenger(sessions *adapter.Sessions) *Messenger { return &Messenger{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Messenger) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// MarkRead marks a conversation read. See the package doc for what that does and
// does not achieve on this transport.
//
// The ids and the timestamp are IGNORED, and saying so is the point: the page
// marks the CONVERSATION, not a list of messages, and there is no per-message
// seen call to route them to. Accepting them silently and marking the whole chat
// would be a wider effect than the caller asked for, dressed as the narrow one.
func (m *Messenger) MarkRead(ctx context.Context, txtID string, _ []string, _ time.Time, chat, _ domain.JID) error {
	pageJID, err := adapter.ToPageJID(chat)
	if err != nil {
		return err
	}
	mk, err := m.marker(ctx, txtID)
	if err != nil {
		return err
	}
	// The result carries the before/after unread count — the postcondition the
	// capability reads back. It is discarded because the port promises only an
	// error; "nothing to mark" is a success, exactly as an already-archived chat
	// is.
	_, err = mk.MarkRead(ctx, pageJID, markLabel)
	return err
}

// SendReaction adds or removes a reaction.
//
// An EMPTY Text removes, which is the domain's own convention — the use case
// turns the word "remove" into an empty string before it gets here. Routing both
// through Add with an empty emoji would leave the page to guess.
func (m *Messenger) SendReaction(ctx context.Context, txtID string, _ domain.JID, r domain.Reaction) (domain.MessageSendResult, error) {
	if r.TargetMessageID == "" {
		return domain.MessageSendResult{}, errNoTarget
	}
	rc, err := m.reactor(ctx, txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	if r.Text == "" {
		if _, err := rc.Remove(ctx, r.TargetMessageID, reactLabel); err != nil {
			return domain.MessageSendResult{}, err
		}
	} else if _, err := rc.Add(ctx, r.TargetMessageID, r.Text, reactLabel); err != nil {
		return domain.MessageSendResult{}, err
	}

	// The port's result carries a timestamp. The page does not report one for a
	// reaction, so this is the moment the call returned — and NOT a claim about
	// when the server recorded it. Inventing a server timestamp would be a
	// fact-shaped guess.
	return domain.MessageSendResult{Timestamp: time.Now()}, nil
}

func (m *Messenger) marker(ctx context.Context, txtID string) (marker, error) {
	if m.newMarker != nil {
		return m.newMarker(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChatLister(m.sessions.Runner(), eval), nil
}

func (m *Messenger) reactor(ctx context.Context, txtID string) (reactor, error) {
	if m.newReactor != nil {
		return m.newReactor(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewReactor(m.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.ChatMessenger = (*Messenger)(nil)

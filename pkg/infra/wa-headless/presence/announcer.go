// Package presence adapts the page's presence capability to the application's
// ports.
//
// It satisfies PresenceAnnouncer and NOT PresenceSubscriber, and the reason is
// MEASURED rather than architectural: H144 put both lab accounts awake at the
// same time and the subscription never reached `subscribed` in 45s, with the
// address-book flags reading `isMyContact:false isAddressBookContact:false`.
// The presence subscription requires the address-book link, and that link is
// created ON THE PHONE.
//
// So this is not "not implemented yet". It is a dependency on a human holding a
// device, and calling it pending would invite somebody to spend a session
// trying to code around it — which is exactly what the LEDGER exists to stop.
package presence

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

// Labels name the operations in the run's operation log. Constants, because a
// label built at runtime can come out empty and a trace with no label locates
// nothing.
const (
	globalLabel = "adapter/send-presence"
	chatLabel   = "adapter/send-chat-presence"
)

// chatStates is the CLOSED set this adapter accepts.
//
// The port's `state` is a free string — the upstream never validated it. Passing
// it through would turn a caller's typo into a call to a page function that does
// not exist; refusing names the problem where it can still be fixed.
var chatStates = map[string]waheadless.PresenceState{
	string(waheadless.PresenceComposing): waheadless.PresenceComposing,
	string(waheadless.PresencePaused):    waheadless.PresencePaused,
	string(waheadless.PresenceRecording): waheadless.PresenceRecording,
	// The socket transport's vocabulary for the same three, so a caller written
	// against it keeps working.
	"typing":  waheadless.PresenceComposing,
	"stopped": waheadless.PresencePaused,
	"pause":   waheadless.PresencePaused,
	"audio":   waheadless.PresenceRecording,
}

// announcer is the slice of the page capability this adapter uses.
type announcer interface {
	Set(ctx context.Context, toJID string, s waheadless.PresenceState, label string) error
	SetOnline(ctx context.Context, available bool, label string) error
}

// Announcer implements appport.PresenceAnnouncer over a headless session.
type Announcer struct {
	sessions *adapter.Sessions
	// newAnnouncer is overridable in tests. Nil uses the real page capability.
	newAnnouncer func(ctx context.Context, txtID string) (announcer, error)
}

// NewAnnouncer builds the adapter.
func NewAnnouncer(sessions *adapter.Sessions) *Announcer {
	return &Announcer{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (a *Announcer) EnsureSession(ctx context.Context, txtID string) error {
	return a.sessions.EnsureSession(ctx, txtID)
}

// SendPresence sets the session's global availability.
func (a *Announcer) SendPresence(ctx context.Context, txtID string, p domain.PresenceType) error {
	var available bool
	switch p {
	case domain.PresenceAvailable:
		available = true
	case domain.PresenceUnavailable:
		available = false
	default:
		return fmt.Errorf("waheadless: unknown presence %q", p)
	}

	cap, err := a.announcer(ctx, txtID)
	if err != nil {
		return err
	}
	return cap.SetOnline(ctx, available, globalLabel)
}

// SendChatPresence announces typing, recording or paused inside a chat.
func (a *Announcer) SendChatPresence(ctx context.Context, txtID string, chat domain.JID, state, _ string) error {
	st, ok := chatStates[state]
	if !ok {
		return fmt.Errorf("%w: %q", waheadless.ErrUnknownPresenceState, state)
	}
	// Identity CONVERTED, never passed through (decision 74).
	pageJID, err := adapter.ToPageJID(chat)
	if err != nil {
		return err
	}

	cap, err := a.announcer(ctx, txtID)
	if err != nil {
		return err
	}
	return cap.Set(ctx, pageJID, st, chatLabel)
}

// capability resolves the session and builds the page capability over it.
func (a *Announcer) announcer(ctx context.Context, txtID string) (announcer, error) {
	if a.newAnnouncer != nil {
		return a.newAnnouncer(ctx, txtID)
	}
	return a.capability(ctx, txtID)
}

func (a *Announcer) capability(ctx context.Context, txtID string) (*waheadless.PresenceAnnouncer, error) {
	eval, err := a.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewPresence(a.sessions.Runner(), eval), nil
}

// Compile-time proof: this adapter satisfies the announcing half, and only it.
var _ appport.PresenceAnnouncer = (*Announcer)(nil)

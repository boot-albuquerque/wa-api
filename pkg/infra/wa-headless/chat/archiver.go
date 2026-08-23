// Package chat adapts the page's chat-state capability to the application's
// ports.
//
// It satisfies ChatArchiver and NOTHING ELSE, and that is the decision 80
// pattern rather than an unfinished job: this transport drives the SPA, so it
// can archive, but RequestUnavailableMessage asks a peer to resend a message
// that could not be DECRYPTED — and a page driver decrypts nothing, the page
// hands over text already. Implementing it to return "unsupported" would
// satisfy the compiler while lying about the capability.
package chat

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

// archiveLabel names the operation in the run's operation log. It is a constant
// because a label built at runtime can come out empty, and a trace with no
// label locates nothing.
const archiveLabel = "adapter/archive-chat"

// setter is the slice of the page capability this adapter uses.
type setter interface {
	SetArchived(ctx context.Context, jid string, archived bool, label string) (waheadless.ChatStateChange, error)
}

// Archiver implements appport.ChatArchiver over a headless session.
type Archiver struct {
	sessions *registry.Registry
	// configFor answers "where does this session live", which is the one thing
	// the registry cannot know. It mirrors the socket adapter taking a client
	// getter: the lookup is injected, not invented here.
	configFor func(txtID string) (waheadless.StartConfig, error)
	runner    *waheadless.Runner
	// newSetter is overridable in tests. Nil uses the real page capability.
	newSetter func(ctx context.Context, txtID string) (setter, error)
}

// NewArchiver builds the adapter.
func NewArchiver(sessions *registry.Registry, configFor func(string) (waheadless.StartConfig, error)) *Archiver {
	return &Archiver{sessions: sessions, configFor: configFor, runner: waheadless.NewRunner()}
}

// EnsureSession reports whether this process can serve txtID.
//
// It does NOT boot: ADR-0005 D6 makes ownership and readiness two questions,
// and a guard that booted a browser to answer the first would turn a cheap
// check into a minute of work.
func (a *Archiver) EnsureSession(_ context.Context, txtID string) error {
	if !a.sessions.Holds(txtID) {
		return fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return nil
}

// ArchiveChat archives or unarchives a conversation.
func (a *Archiver) ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error {
	// The identity is CONVERTED, never passed through: a domain.JID minted by
	// the socket adapter names the right person in the wrong namespace for this
	// page, and the page would answer "no such conversation" for a chat plainly
	// present (decision 74).
	pageJID, err := adapter.ToPageJID(chat)
	if err != nil {
		return err
	}

	s, err := a.setter(ctx, txtID)
	if err != nil {
		return err
	}

	// The capability reads the state back after writing it — invariant 14, no
	// silent success — and the Change it returns IS that postcondition. It is
	// discarded here because the port promises only an error; a port that
	// wanted the confirmation would have to say so in its signature.
	_, err = s.SetArchived(ctx, pageJID, archive, archiveLabel)
	return err
}

func (a *Archiver) setter(ctx context.Context, txtID string) (setter, error) {
	if a.newSetter != nil {
		return a.newSetter(ctx, txtID)
	}
	cfg, err := a.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("waheadless: config for session: %w", err)
	}
	holder, err := a.sessions.Acquire(txtID, cfg, registry.KindOperational)
	if err != nil {
		return nil, err
	}
	sess, err := holder.Session(ctx)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChatState(a.runner, sess.Tab().Evaluate), nil
}

// Compile-time proof that this adapter satisfies the narrow port — and only it.
var _ appport.ChatArchiver = (*Archiver)(nil)

// Package blocklist adapts the page's blocking capability to the application's
// BlocklistManager port.
//
// Both halves are PROVEN in the LEDGER — block and unblock in H59 (blocklist
// 0->1->0), and reading the list in H146, which also recorded that filtering the
// roster for `isBlocked` would return empty forever: of 945 contacts, ZERO carry
// that flag. So the list comes from WAWebCollections.Blocklist, not from the
// roster.
//
// There is no capability asymmetry here. There IS a DATA asymmetry, and it is
// declared rather than papered over — see dhash below.
package blocklist

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

const (
	listLabel   = "adapter/get-blocklist"
	updateLabel = "adapter/update-blocklist"

	// dhashUnavailable is what this transport can honestly say about DHash.
	//
	// The socket versions the blocklist with a hash the server sends. The page
	// exposes no equivalent — it hands over the collection, not its version. An
	// invented value would be well-formed and false, and a caller comparing two
	// of them would conclude "unchanged" from two different lists.
	//
	// Empty is the honest answer, and this constant exists so the reason has one
	// home instead of being a bare "" somebody later mistakes for an oversight.
	dhashUnavailable = ""
)

// blocker is the slice of the page capability this adapter uses, as an
// interface so the adapter's own rules — the read-back, what DHash gets, which
// verb runs — are testable without a browser.
type blocker interface {
	Block(ctx context.Context, jid, label string) (waheadless.BlockResult, error)
	Unblock(ctx context.Context, jid, label string) (waheadless.BlockResult, error)
	List(ctx context.Context, label string) ([]string, error)
}

// Manager implements appport.BlocklistManager over a headless session.
type Manager struct {
	sessions  *registry.Registry
	configFor func(txtID string) (waheadless.StartConfig, error)
	runner    *waheadless.Runner
	// newBlocker is overridable in tests. Nil uses the real page capability.
	newBlocker func(ctx context.Context, txtID string) (blocker, error)
}

// NewManager builds the adapter.
func NewManager(sessions *registry.Registry, configFor func(string) (waheadless.StartConfig, error)) *Manager {
	return &Manager{sessions: sessions, configFor: configFor, runner: waheadless.NewRunner()}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Manager) EnsureSession(_ context.Context, txtID string) error {
	if !m.sessions.Holds(txtID) {
		return fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return nil
}

// GetBlocklist returns who this account refuses to hear from.
func (m *Manager) GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error) {
	b, err := m.blocker(ctx, txtID)
	if err != nil {
		return domain.Blocklist{}, err
	}
	jids, err := b.List(ctx, listLabel)
	if err != nil {
		return domain.Blocklist{}, err
	}
	return domain.Blocklist{JIDs: jids, DHash: dhashUnavailable}, nil
}

// UpdateBlocklist blocks or unblocks a contact and reports the resulting list.
func (m *Manager) UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, blockIt bool) (domain.BlocklistUpdate, error) {
	// Identity CONVERTED, never passed through (decision 74).
	pageJID, err := adapter.ToPageJID(target)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	b, err := m.blocker(ctx, txtID)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	if blockIt {
		_, err = b.Block(ctx, pageJID, updateLabel)
	} else {
		_, err = b.Unblock(ctx, pageJID, updateLabel)
	}
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	// The capability's Result carries SIZES and never entries, deliberately.
	// The port promises the resulting list, so it is read back — which is also
	// the postcondition invariant 14 asks for, now visible to the caller
	// instead of only to the capability.
	jids, err := b.List(ctx, updateLabel+"/readback")
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	return domain.BlocklistUpdate{
		// This build files identities under LID and the page answers in its own
		// spelling, so RequestedJID is what we asked with and ResolvedJID is
		// the same value: there is no second translation to report. Saying they
		// differ would invent a resolution step that did not happen.
		RequestedJID: domain.JID(pageJID),
		ResolvedJID:  domain.JID(pageJID),
		Entries:      jids,
		DHash:        dhashUnavailable,
	}, nil
}

func (m *Manager) blocker(ctx context.Context, txtID string) (blocker, error) {
	if m.newBlocker != nil {
		return m.newBlocker(ctx, txtID)
	}
	return m.capability(ctx, txtID)
}

func (m *Manager) capability(ctx context.Context, txtID string) (*waheadless.Blocker, error) {
	cfg, err := m.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("waheadless: config for session: %w", err)
	}
	holder, err := m.sessions.Acquire(txtID, cfg, registry.KindOperational)
	if err != nil {
		return nil, err
	}
	sess, err := holder.Session(ctx)
	if err != nil {
		return nil, err
	}
	return waheadless.NewBlocker(m.runner, sess.Tab().Evaluate), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.BlocklistManager = (*Manager)(nil)

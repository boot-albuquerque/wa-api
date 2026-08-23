// Package avatar adapts the page's avatar capability to the application's
// AvatarReader port.
package avatar

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

const fetchLabel = "adapter/get-profile-picture"

// fetcher is the slice of the page capability this adapter uses.
//
// It is an interface so the adapter's own logic — the Present branch, the
// preview choice, what becomes the ID — can be tested WITHOUT a browser. That
// logic is the part with rules in it; resolving a session is plumbing.
type fetcher interface {
	Fetch(ctx context.Context, jid, label string) (waheadless.AvatarPicture, error)
}

// Reader implements appport.AvatarReader over a headless session.
type Reader struct {
	sessions  *registry.Registry
	configFor func(txtID string) (waheadless.StartConfig, error)
	runner    *waheadless.Runner
	// newFetcher is overridable in tests. Nil uses the real page capability.
	newFetcher func(ctx context.Context, txtID string) (fetcher, error)
}

// NewReader builds the adapter.
func NewReader(sessions *registry.Registry, configFor func(string) (waheadless.StartConfig, error)) *Reader {
	return &Reader{sessions: sessions, configFor: configFor, runner: waheadless.NewRunner()}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (r *Reader) EnsureSession(_ context.Context, txtID string) error {
	if !r.sessions.Holds(txtID) {
		return fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return nil
}

// GetProfilePicture returns the picture, or nil when the person has none.
//
// NIL AND NO ERROR is the contract for "no picture", and it is the branch the
// capability's Present field decides — never `URL != ""`. Deriving absence from
// an empty URL would make a failed read indistinguishable from a person who
// simply has no photo, which is the F83 shape: an empty avatar_url from a
// network failure looked exactly like an empty one from having no picture.
func (r *Reader) GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error) {
	pageJID, err := adapter.ToPageJID(target)
	if err != nil {
		return nil, err
	}

	f, err := r.fetcher(ctx, txtID)
	if err != nil {
		return nil, err
	}
	pic, err := f.Fetch(ctx, pageJID, fetchLabel)
	if err != nil {
		return nil, err
	}
	if !pic.Present {
		return nil, nil
	}

	url := pic.URL
	if preview {
		url = pic.PreviewURL
	}
	// Tag is what changes when the picture changes, so it is the ID a caller can
	// use to skip a download it already has.
	return &domain.AvatarInfo{ID: pic.Tag, URL: url}, nil
}

func (r *Reader) fetcher(ctx context.Context, txtID string) (fetcher, error) {
	if r.newFetcher != nil {
		return r.newFetcher(ctx, txtID)
	}
	return r.capability(ctx, txtID)
}

func (r *Reader) capability(ctx context.Context, txtID string) (*waheadless.AvatarFetcher, error) {
	cfg, err := r.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("waheadless: config for session: %w", err)
	}
	holder, err := r.sessions.Acquire(txtID, cfg, registry.KindOperational)
	if err != nil {
		return nil, err
	}
	sess, err := holder.Session(ctx)
	if err != nil {
		return nil, err
	}
	return waheadless.NewAvatarFetcher(r.runner, sess.Tab().Evaluate), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.AvatarReader = (*Reader)(nil)

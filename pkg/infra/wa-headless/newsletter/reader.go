// Package newsletter adapts the page's channel capability to the application's
// NewsletterReader port.
//
// # The freshness caveat, which the caller has to know
//
// H123 proved the listing non-empty — one entry with membership=owner right
// after creating a channel, zero after deleting it. But H139 measured something
// the proof does not cover: `Followed` reflects the CLIENT'S CACHE, not the
// server. When ANOTHER account deletes a channel this one is admin of or
// subscribed to, the local model STAYS — six leftovers were found carrying
// serverAlive:false.
//
// This adapter does NOT filter them, and that is deliberate rather than lazy:
// the capability's DirectoryEntry carries no liveness field, so filtering here
// would mean inventing a judgement from data that is not there. Returning the
// cache as the cache is honest; returning a filtered list would claim a
// freshness this transport cannot deliver.
//
// It is a fourth kind of divergence, after "no meaning", "human dependency" and
// "missing datum": the answer is COMPLETE, and its FRESHNESS is not guaranteed.
// The port has nowhere to say so, so it is said here.
package newsletter

import (
	"context"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
)

const listLabel = "adapter/list-subscribed"

// follower is the slice of the page capability this adapter uses.
type follower interface {
	Followed(ctx context.Context, label string) ([]waheadless.ChannelEntry, error)
}

// Reader implements appport.NewsletterReader over a headless session.
type Reader struct {
	sessions *adapter.Sessions
	// newFollower is overridable in tests. Nil uses the real page capability.
	newFollower func(ctx context.Context, txtID string) (follower, error)
}

// NewReader builds the adapter.
func NewReader(sessions *adapter.Sessions) *Reader { return &Reader{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (r *Reader) EnsureSession(ctx context.Context, txtID string) error {
	return r.sessions.EnsureSession(ctx, txtID)
}

// ListSubscribed returns the channels this account follows, AS THE CLIENT HOLDS
// THEM. See the package doc: entries can outlive the channel on the server.
func (r *Reader) ListSubscribed(ctx context.Context, txtID string) (any, error) {
	f, err := r.follower(ctx, txtID)
	if err != nil {
		return nil, err
	}
	entries, err := f.Followed(ctx, listLabel)
	if err != nil {
		return nil, err
	}
	// An empty listing is a legitimate answer — this account follows nothing —
	// and it is returned as an empty slice rather than nil so a caller that
	// ranges over it does not have to distinguish the two.
	if entries == nil {
		entries = []waheadless.ChannelEntry{}
	}
	return entries, nil
}

func (r *Reader) follower(ctx context.Context, txtID string) (follower, error) {
	if r.newFollower != nil {
		return r.newFollower(ctx, txtID)
	}
	eval, err := r.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewChannelManager(r.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.NewsletterReader = (*Reader)(nil)

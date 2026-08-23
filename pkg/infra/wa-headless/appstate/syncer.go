// Package appstate adapts the page's roster refresh to the application's
// AppStateSyncer port.
//
// # The three modes are socket vocabulary
//
// The port names three: if_unsynced, incremental and full. They describe
// app-state PATCH semantics — cheap fetch, full re-snapshot, skip when already
// synced — and that is a protocol concept the socket transport implements.
//
// The page has one behaviour: ask it to refresh the collection and report what
// moved. There is no re-snapshot to request and no version to preserve, because
// there is no patch stream to be behind on.
//
// So all three modes run the same operation, and this file says so rather than
// pretending the distinction survives. What it does NOT do is accept a mode it
// has never heard of: a typo becoming a silent no-op would let a caller believe
// it asked for something it did not.
package appstate

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
)

const primeLabel = "adapter/sync-contact-roster"

// knownModes is the CLOSED set the port documents. The values map to one
// behaviour here, and the map exists so an unknown one is refused rather than
// silently treated as any of them.
var knownModes = map[string]bool{
	"if_unsynced": true,
	"incremental": true,
	"full":        true,
}

// primer is the slice of the page capability this adapter uses.
type primer interface {
	Prime(ctx context.Context, label string) (waheadless.RosterPrimeResult, error)
}

// Syncer implements appport.AppStateSyncer over a headless session.
type Syncer struct {
	sessions *adapter.Sessions
	// newPrimer is overridable in tests. Nil uses the real page capability.
	newPrimer func(ctx context.Context, txtID string) (primer, error)
}

// NewSyncer builds the adapter.
func NewSyncer(sessions *adapter.Sessions) *Syncer { return &Syncer{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (s *Syncer) EnsureSession(ctx context.Context, txtID string) error {
	return s.sessions.EnsureSession(ctx, txtID)
}

// SyncContactRoster asks the page to refresh the roster.
//
// "Nothing changed" is a SUCCESS, not a failure: on a roster already current
// that is the correct answer, and the measured ordinary case. The capability
// reserves its error for the page refusing, or for the roster SHRINKING — which
// is the one outcome a refresh must never produce quietly.
func (s *Syncer) SyncContactRoster(ctx context.Context, txtID string, mode string) error {
	if !knownModes[mode] {
		return fmt.Errorf("waheadless: unknown sync mode %q", mode)
	}

	p, err := s.primer(ctx, txtID)
	if err != nil {
		return err
	}
	_, err = p.Prime(ctx, primeLabel)
	return err
}

func (s *Syncer) primer(ctx context.Context, txtID string) (primer, error) {
	if s.newPrimer != nil {
		return s.newPrimer(ctx, txtID)
	}
	eval, err := s.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewContactLister(s.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.AppStateSyncer = (*Syncer)(nil)

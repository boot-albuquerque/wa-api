// Package roster adapts the page's contact listing to the application's
// ContactRoster port.
//
// H129 proved the listing with the two identity lines — phone and lid — merged
// into one person, which is why Rows is larger than the number of contacts and
// why that is the CORRECT state rather than a bug.
package roster

import (
	"context"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

const listLabel = "adapter/contact-roster"

// lister is the slice of the page capability this adapter uses, as an interface
// so the mapping rules are testable without a browser.
type lister interface {
	List(ctx context.Context, label string) (waheadless.ContactRoster, error)
}

// Roster implements appport.ContactRoster over a headless session.
type Roster struct {
	sessions *adapter.Sessions
	// newLister is overridable in tests. Nil uses the real page capability.
	newLister func(ctx context.Context, txtID string) (lister, error)
}

// NewRoster builds the adapter.
func NewRoster(sessions *adapter.Sessions) *Roster {
	return &Roster{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (r *Roster) EnsureSession(ctx context.Context, txtID string) error {
	return r.sessions.EnsureSession(ctx, txtID)
}

// GetAllContacts returns the roster and how many people it holds.
//
// The COUNT is the number of PEOPLE, not of rows. The page's collection carries
// one row per identity, so the same person appears twice — 944 rows folded into
// 390 people on the measured profile. Returning the row count would report
// roughly twice the contacts anybody has.
func (r *Roster) GetAllContacts(ctx context.Context, txtID string) (any, int, error) {
	roster, err := r.list(ctx, txtID)
	if err != nil {
		return nil, 0, err
	}
	return roster.Contacts, len(roster.Contacts), nil
}

// ContactNames returns the roster keyed by JID, typed.
//
// FullName and FirstName stay EMPTY, and that is measured rather than missing:
// getName answers for 1 of 944 models on the lab profile, because it reads the
// ADDRESS BOOK and almost nothing is saved there. It is a property of the
// profile, not of the build, and a listing that promised "name" would return
// nothing for 943 of 944 people.
//
// domain.ContactName.Melhor already falls back to PushName in that order, so
// the empty fields degrade exactly as the domain declared they should — this is
// the declared order working, not a gap.
func (r *Roster) ContactNames(ctx context.Context, txtID string) (map[domain.JID]domain.ContactName, error) {
	roster, err := r.list(ctx, txtID)
	if err != nil {
		return nil, err
	}

	out := make(map[domain.JID]domain.ContactName, len(roster.Contacts))
	for _, c := range roster.Contacts {
		name := domain.ContactName{PushName: c.Pushname, BusinessName: c.VerifiedName}
		// A person merged from two rows is indexed under BOTH identities. A
		// caller holding either half must find them, and which half it holds
		// depends on where it read the JID — the history speaks phone, the
		// message collection speaks lid.
		if c.LID != "" {
			out[domain.JID(c.LID)] = name
		}
		if c.PN != "" {
			out[domain.JID(c.PN)] = name
		}
	}
	return out, nil
}

// GetUserInfo returns the raw entries for the given JIDs.
func (r *Roster) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) (any, error) {
	names, err := r.ContactNames(ctx, txtID)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]domain.ContactName, len(jids))
	for _, j := range jids {
		if n, ok := names[j]; ok {
			out[j] = n
		}
	}
	return out, nil
}

func (r *Roster) list(ctx context.Context, txtID string) (waheadless.ContactRoster, error) {
	l, err := r.lister(ctx, txtID)
	if err != nil {
		return waheadless.ContactRoster{}, err
	}
	return l.List(ctx, listLabel)
}

func (r *Roster) lister(ctx context.Context, txtID string) (lister, error) {
	if r.newLister != nil {
		return r.newLister(ctx, txtID)
	}
	eval, err := r.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewContactLister(r.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.ContactRoster = (*Roster)(nil)

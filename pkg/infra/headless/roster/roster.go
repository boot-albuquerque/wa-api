// Package roster adapts the page's contact listing to the application's
// ContactRoster port.
//
// H129 proved the listing with the two identity lines — phone and lid — merged
// into one person, which is why Rows is larger than the number of contacts and
// why that is the CORRECT state rather than a bug.
package roster

import (
	"context"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
)

const listLabel = "adapter/contact-roster"

// lister is the slice of the page capability this adapter uses, as an interface
// so the mapping rules are testable without a browser.
type lister interface {
	List(ctx context.Context, label string) (headless.ContactRoster, error)
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
// The page's collection is already ordered deterministically (contacts.Roster
// documents it), so this adapter preserves that order instead of imposing a
// second one.
func (r *Roster) GetAllContacts(ctx context.Context, txtID string) ([]domain.Contact, int, error) {
	roster, err := r.list(ctx, txtID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]domain.Contact, 0, len(roster.Contacts))
	for _, c := range roster.Contacts {
		out = append(out, domain.Contact{
			// Identity() prefers the lid, which is what the send path
			// measured as working on this build.
			JID:          domain.JID(c.Identity()),
			PN:           domain.JID(c.PN),
			LID:          domain.JID(c.LID),
			Found:        true,
			PushName:     c.Pushname,
			BusinessName: c.VerifiedName,
			IsBusiness:   c.IsBusiness,
			// FullName and FirstName stay EMPTY, and that is measured rather
			// than missing: this build's getName answers for 1 of 944 models
			// on the lab profile because it reads the ADDRESS BOOK. See
			// ContactNames below.
		})
	}
	return out, len(out), nil
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

// GetUserInfo returns what this engine knows about the given JIDs, in the
// order they were asked for.
//
// Status, PictureID and Devices stay EMPTY here, and that is a property of the
// engine rather than a gap: this adapter reads the page's contact collection,
// which carries names and identities and nothing the server would have to be
// asked for. The wa-noise adapter fills exactly the complementary half.
func (r *Roster) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) ([]domain.UserInfo, error) {
	names, err := r.ContactNames(ctx, txtID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UserInfo, 0, len(jids))
	for _, j := range jids {
		n, ok := names[j]
		if !ok {
			continue
		}
		out = append(out, domain.UserInfo{
			JID:          j,
			PushName:     n.PushName,
			BusinessName: n.BusinessName,
			// VerifiedName stays empty even though the page carries a
			// verifiedName: that field is the LOCAL roster's copy, and it is
			// already reported as BusinessName. Writing the same value into
			// two keys would make a client believe two sources agreed.
		})
	}
	return out, nil
}

func (r *Roster) list(ctx context.Context, txtID string) (headless.ContactRoster, error) {
	l, err := r.lister(ctx, txtID)
	if err != nil {
		return headless.ContactRoster{}, err
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
	return headless.NewContactLister(r.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.ContactRoster = (*Roster)(nil)

// Package profile adapts the account's own identity to the application's
// ProfileAccessProvider port.
//
// # Why a SNAPSHOT and not live reads
//
// ProfileDataAccess is a synchronous interface: PushName, OwnJID and DeviceInfo
// take no context and return no error. That shape assumes a caller that already
// HOLDS the state — which is exactly what the socket transport has, in
// client.Store.
//
// A page transport has to ASK. Doing that inside a method with no context and
// no error would be hidden network in a reader, which decision 66 refused.
//
// So the asking happens in ProfileAccess, which DOES have a context, and what
// it returns is a snapshot. That mirrors the reference exactly: there too,
// PushName reads a store and answers "" when the store is absent — and the
// port's own doc already declares the convention, that a missing field becomes
// the zero value, which is the honest answer for "not known yet".
package profile

import (
	"context"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
)

const (
	identityLabel = "adapter/own-identity"
	pictureLabel  = "adapter/profile-picture"
	contactLabel  = "adapter/contact-info"
)

// Provider implements appport.ProfileAccessProvider over a headless session.
type Provider struct {
	sessions *adapter.Sessions
}

// NewProvider builds the adapter.
func NewProvider(sessions *adapter.Sessions) *Provider { return &Provider{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (p *Provider) EnsureSession(ctx context.Context, txtID string) error {
	return p.sessions.EnsureSession(ctx, txtID)
}

// ProfileAccess takes the snapshot. This is the one method with a context, and
// therefore the only place allowed to do the asking.
func (p *Provider) ProfileAccess(ctx context.Context, txtID string) (appport.ProfileDataAccess, error) {
	eval, err := p.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	runner := p.sessions.Runner()

	id, err := headless.RefreshOwnIdentity(ctx, runner, eval, identityLabel)
	if err != nil {
		return nil, err
	}
	return &snapshot{identity: id, runner: runner, eval: eval}, nil
}

// fetcher e lister são as fatias das capabilities que o instantâneo usa, como
// interfaces para que as REGRAS deste adaptador — foto ausente contra falha de
// leitura, contato fora do roster contra roster ilegível — sejam testáveis sem
// browser. Essa lógica é a parte com regras; construir a capability é fiação.
type fetcher interface {
	Fetch(ctx context.Context, jid, label string) (headless.AvatarPicture, error)
}

type lister interface {
	List(ctx context.Context, label string) (headless.ContactRoster, error)
}

// snapshot answers from what was captured, and asks the page only for the two
// methods that carry a context.
type snapshot struct {
	identity headless.OwnIdentity
	runner   *headless.Runner
	eval     headless.Evaluator
	// Sobrescritíveis em teste. Nil usa a capability real.
	newFetcher func() fetcher
	newLister  func() lister
}

func (s *snapshot) fetcher() fetcher {
	if s.newFetcher != nil {
		return s.newFetcher()
	}
	return headless.NewAvatarFetcher(s.runner, s.eval)
}

func (s *snapshot) lister() lister {
	if s.newLister != nil {
		return s.newLister()
	}
	return headless.NewContactLister(s.runner, s.eval)
}

// PushName is EMPTY on this build, and that is measured rather than missing.
//
// The page's own display-name getter EXISTS — it is a function, not absent —
// and returns null against the real paired profile. Two candidates on the
// neighbouring module do not exist at all, and the measurement discarded them.
// Empty is therefore the zero value the port declares as "not known", not a
// field somebody forgot to fill.
func (s *snapshot) PushName() string { return s.identity.DisplayName }

// OwnJID prefers the LID, because that is what this build files identities
// under — 397 of 399 messages on the reference account. The phone identity is
// the fallback, and the second return says whether either was materialised at
// all rather than making the caller compare against an empty string.
func (s *snapshot) OwnJID() (domain.JID, bool) {
	if s.identity.LID.Present() {
		return domain.JID(s.identity.LID.Serialized), true
	}
	if s.identity.PN.Present() {
		return domain.JID(s.identity.PN.Serialized), true
	}
	return "", false
}

// ProfilePictureURL returns the full-size URL and the tag that changes when the
// picture changes. Absence is an EMPTY pair with no error: this person has no
// photo is a normal answer, and the F83 shape — a failure looking exactly like
// an absence — is what the separate error return exists to prevent.
func (s *snapshot) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	pageJID, err := adapter.ToPageJID(jid)
	if err != nil {
		return "", "", err
	}
	pic, err := s.fetcher().Fetch(ctx, pageJID, pictureLabel)
	if err != nil {
		return "", "", err
	}
	if !pic.Present {
		return "", "", nil
	}
	return pic.URL, pic.Tag, nil
}

// ContactInfo returns the person's own name and the business name, in that
// order, from the roster.
//
// Both can be empty, and empty means the roster did not carry them — pushname
// was measured at 456 of 944 rows and verified name at 22 of 944, so a caller
// that treats empty as an error would be wrong about most people.
func (s *snapshot) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	pageJID, err := adapter.ToPageJID(jid)
	if err != nil {
		return "", "", err
	}
	roster, err := s.lister().List(ctx, contactLabel)
	if err != nil {
		return "", "", err
	}
	for _, c := range roster.Contacts {
		if c.LID == pageJID || c.PN == pageJID {
			return c.Pushname, c.VerifiedName, nil
		}
	}
	// Not in the roster is an ANSWER, not a failure: this account simply does
	// not know that person.
	return "", "", nil
}

// DeviceInfo is the ZERO VALUE, deliberately.
//
// The port's own doc declares the convention: a missing field becomes the zero
// value, which is the honest answer for "the store does not have this yet". The
// socket transport fills it from a local pairing store that a page driver has no
// equivalent of — the page holds a session, not a device registry.
//
// Returning invented values here would be worse than empty: DeviceInfo feeds a
// diagnostic surface, and a plausible-looking wrong platform is harder to
// distrust than a blank one.
func (s *snapshot) DeviceInfo() domain.SessionDeviceInfo { return domain.SessionDeviceInfo{} }

// Compile-time proofs.
var (
	_ appport.ProfileAccessProvider = (*Provider)(nil)
	_ appport.ProfileDataAccess     = (*snapshot)(nil)
)

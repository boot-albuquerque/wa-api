// Package accounttype implements appport.AccountTypeDetector over a wa-noise
// session, using the verified-name certificate signal measured in
// internal/wa-noise/capabilities/user/accounttype.go.
package accounttype

import (
	"context"
	"errors"

	waclientuser "wa-api/internal/wa-noise/capabilities/user"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	waclient "wa-api/pkg/infra/wa-noise/client"
)

// errUnsupportedClient is returned when the waclient.Client in hand is not a
// waclient.RealClient — a fake in a test, or a future transport that never
// wraps *wanoise.Client. It is a build/composition issue, not something a
// caller should read as "personal account".
var errUnsupportedClient = errors.New("accounttype: client does not expose wa-noise's concrete type")

// Detector implements appport.AccountTypeDetector over the wa-noise client
// getter, following the same seam as SessionGuardAdapter.
type Detector struct {
	getClient waclient.Getter
}

// NewDetector builds the adapter.
func NewDetector(getClient waclient.Getter) *Detector {
	return &Detector{getClient: getClient}
}

// EnsureSession reports whether a wa-noise client exists for txtID.
func (d *Detector) EnsureSession(_ context.Context, txtID string) error {
	if d.getClient(txtID) == nil {
		return errNoSession(txtID)
	}
	return nil
}

// Detect asks the connected session for its verified-name certificate via
// usync. It only calls the concrete *wanoise.Client (the narrow waclient.Client
// interface does not carry this operation, matching the ProfileDataAccess
// adapter's own precedent for capabilities the SDK exposes on the concrete
// type only) — an unexpected concrete type answers AccountTypeUnknown rather
// than panicking.
func (d *Detector) Detect(ctx context.Context, txtID string) (domain.AccountType, error) {
	c := d.getClient(txtID)
	if c == nil {
		return domain.AccountTypeUnknown, errNoSession(txtID)
	}
	real, ok := c.(waclient.RealClient)
	if !ok {
		return domain.AccountTypeUnknown, errUnsupportedClient
	}

	kind, err := real.Client.DetectOwnAccountKind(ctx)
	if err != nil {
		return domain.AccountTypeUnknown, err
	}
	switch kind {
	case waclientuser.AccountKindBusiness:
		return domain.AccountTypeBusiness, nil
	case waclientuser.AccountKindPersonal:
		return domain.AccountTypePersonal, nil
	default:
		return domain.AccountTypeUnknown, nil
	}
}

// errNoSession mirrors runtime/session.ErrNoSession's shape without importing
// it — importing runtime/session from here would be a cycle risk this
// adapter does not need to take on for one error value. A caller that needs
// the typed apperr behaviour already gets it from SessionGuardAdapter for the
// same txtID.
func errNoSession(txtID string) error {
	return &noSessionError{txtID: txtID}
}

type noSessionError struct{ txtID string }

func (e *noSessionError) Error() string {
	return "accounttype: no wa-noise session for " + e.txtID
}

var _ appport.AccountTypeDetector = (*Detector)(nil)

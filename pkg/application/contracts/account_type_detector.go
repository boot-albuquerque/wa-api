package port

import (
	"context"

	"wa-api/pkg/domain"
)

// AccountTypeDetector asks the transport whether the session's own account is
// personal or Business, USING WHATEVER REAL SIGNAL that transport has —
// wa-noise reads a protocol-level verified-name certificate, wa-headless
// reads a getter off the SPA's own connection model. See each adapter for its
// source.
//
// Detect must never turn "the signal was unreachable" into a business/
// personal guess: domain.AccountTypeUnknown alongside a non-nil error is the
// only honest answer when detection could not run, and CALLERS decide what
// unknown means for them (typically: keep whatever was already persisted).
type AccountTypeDetector interface {
	SessionGuard

	// Detect returns the session's account type, or AccountTypeUnknown with
	// a non-nil error when the underlying signal could not be read.
	Detect(ctx context.Context, txtID string) (domain.AccountType, error)
}

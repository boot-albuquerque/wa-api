package pairing

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/noise/client"
	wasession "wa-api/pkg/infra/noise/runtime/session"
)

// codeNoSessionRow is the code GetQRUseCase already answered for this exact
// condition before the read moved behind the port
// (pkg/application/usecase/session/get_qr.go). Kept byte for byte: it is public
// contract, and renaming it here would be a silent contract change dressed as a
// refactor.
const codeNoSessionRow = "no_session"

// errNoSessionRow reports a target id with no user row.
func errNoSessionRow(txtID string) error {
	return apperr.New(codeNoSessionRow, apperr.CategoryValidation, "no session", false,
		fmt.Errorf("no user row for session %q", txtID))
}

// QRReaderAdapter implements appport.PairingQRReader for the socket transport.
//
// # Why an engine adapter reads a shared table
//
// The pairing code is stored in users.qrcode, and reading it is one repository
// call — which makes this look like an adapter with nothing engine-specific in
// it. What is engine-specific is the WRITE: users.qrcode only ever holds a
// value because the wa-noise lifecycle listener puts one there, on every "code"
// event of the SDK's QR channel (pkg/bootstrap/lifecycle.go). No other engine in
// this build writes that column.
//
// So an engine-agnostic read would answer 200 with an empty code, forever, for
// any session on a transport that has no such listener — a truthful-looking
// success for something that never happened. Behind this port the same request
// gets capability_not_supported instead, because the engine has no adapter to
// resolve. See pkg/pairing for the resolution order.
//
// The read itself is ListUsers(ctx, txtID), the same call GetQRUseCase made
// directly until 2026-08-27 and the same one GetStatusUseCase still makes: this
// moved the read behind the engine, it did not open a second path to the column.
// The cached user info from the auth middleware is deliberately NOT used — it
// has a ten-minute TTL and the QR rotates inside it.
type QRReaderAdapter struct {
	*wasession.SessionGuardAdapter
	users appport.UserRepository
}

// NewQRReaderAdapter builds the adapter from the client lookup and the user
// repository.
func NewQRReaderAdapter(getClient client.Getter, users appport.UserRepository) *QRReaderAdapter {
	return &QRReaderAdapter{
		SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient),
		users:               users,
	}
}

// PairingQR returns the pairing code currently persisted for txtID.
//
// An empty string is returned WITHOUT an error when the row exists and carries
// no code: the QR rotates, and the gap between two codes is a normal state the
// dev panel polls through. A missing row is a different fact and stays an error
// — it is the caller's own use case that decides what to answer for it.
func (a *QRReaderAdapter) PairingQR(ctx context.Context, txtID string) (string, error) {
	entries, err := a.users.ListUsers(ctx, txtID)
	if err != nil {
		return "", fmt.Errorf("database error: %w", err)
	}
	if len(entries) == 0 {
		return "", errNoSessionRow(txtID)
	}
	return entries[0].QRCode, nil
}

// Compile-time assertion that the adapter implements the port.
var _ appport.PairingQRReader = (*QRReaderAdapter)(nil)

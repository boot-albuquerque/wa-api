package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- SessionGuard ------------------------------------------------------

// SessionGuardCall é uma chamada a EnsureSession.
type SessionGuardCall struct {
	Ctx   context.Context
	TxtID string
}

// SessionGuard é o fake de port.SessionGuard, e o bloco embutido em todas as
// portas de capacidade que exigem sessão. Zero-value = sessão sempre válida.
type SessionGuard struct {
	EnsureSessionFunc  func(ctx context.Context, txtID string) error
	EnsureSessionCalls []SessionGuardCall
}

var _ port.SessionGuard = (*SessionGuard)(nil)

// EnsureSession implementa port.SessionGuard.
func (f *SessionGuard) EnsureSession(ctx context.Context, txtID string) error {
	f.EnsureSessionCalls = append(f.EnsureSessionCalls, SessionGuardCall{Ctx: ctx, TxtID: txtID})
	if f.EnsureSessionFunc != nil {
		return f.EnsureSessionFunc(ctx, txtID)
	}
	return nil
}

// FailSession devolve um SessionGuard que sempre recusa com err. Atalho para
// o caso de teste mais frequente do plano.
func FailSession(err error) SessionGuard {
	return SessionGuard{EnsureSessionFunc: func(context.Context, string) error { return err }}
}

// --- SessionStatusReader -----------------------------------------------

// SessionStatusReaderSessionStatusCall é uma chamada a SessionStatus.
type SessionStatusReaderSessionStatusCall struct {
	Ctx    context.Context
	UserID string
}

// SessionStatusReader é o fake de port.SessionStatusReader. Zero-value
// reporta desconectado e deslogado.
type SessionStatusReader struct {
	SessionStatusFunc  func(ctx context.Context, userID string) (connected, loggedIn bool)
	SessionStatusCalls []SessionStatusReaderSessionStatusCall
}

var _ port.SessionStatusReader = (*SessionStatusReader)(nil)

// SessionStatus implementa port.SessionStatusReader.
func (f *SessionStatusReader) SessionStatus(ctx context.Context, userID string) (bool, bool) {
	f.SessionStatusCalls = append(f.SessionStatusCalls, SessionStatusReaderSessionStatusCall{Ctx: ctx, UserID: userID})
	if f.SessionStatusFunc != nil {
		return f.SessionStatusFunc(ctx, userID)
	}
	return false, false
}

// --- SessionController -------------------------------------------------

// SessionControllerLogoutCall é uma chamada a Logout.
type SessionControllerLogoutCall struct {
	Ctx   context.Context
	TxtID string
}

// SessionControllerDisconnectCall é uma chamada a Disconnect.
type SessionControllerDisconnectCall struct {
	Ctx   context.Context
	TxtID string
}

// SessionController é o fake de port.SessionController (SessionGuard +
// SessionStatusReader + Logout/Disconnect).
type SessionController struct {
	SessionGuard
	SessionStatusReader

	LogoutFunc  func(ctx context.Context, txtID string) error
	LogoutCalls []SessionControllerLogoutCall

	DisconnectFunc  func(ctx context.Context, txtID string) error
	DisconnectCalls []SessionControllerDisconnectCall
}

var _ port.SessionController = (*SessionController)(nil)

// Logout implementa port.SessionController.
func (f *SessionController) Logout(ctx context.Context, txtID string) error {
	f.LogoutCalls = append(f.LogoutCalls, SessionControllerLogoutCall{Ctx: ctx, TxtID: txtID})
	if f.LogoutFunc != nil {
		return f.LogoutFunc(ctx, txtID)
	}
	return nil
}

// Disconnect implementa port.SessionController.
func (f *SessionController) Disconnect(ctx context.Context, txtID string) error {
	f.DisconnectCalls = append(f.DisconnectCalls, SessionControllerDisconnectCall{Ctx: ctx, TxtID: txtID})
	if f.DisconnectFunc != nil {
		return f.DisconnectFunc(ctx, txtID)
	}
	return nil
}

// --- SessionCounter ----------------------------------------------------

// SessionCounterCountSessionsCall é uma chamada a CountSessions.
type SessionCounterCountSessionsCall struct {
	Ctx context.Context
}

// SessionCounter é o fake de port.SessionCounter. Zero-value devolve
// contagens zeradas e nil.
type SessionCounter struct {
	CountSessionsFunc  func(ctx context.Context) (domain.SessionCounts, error)
	CountSessionsCalls []SessionCounterCountSessionsCall
}

var _ port.SessionCounter = (*SessionCounter)(nil)

// CountSessions implementa port.SessionCounter.
func (f *SessionCounter) CountSessions(ctx context.Context) (domain.SessionCounts, error) {
	f.CountSessionsCalls = append(f.CountSessionsCalls, SessionCounterCountSessionsCall{Ctx: ctx})
	if f.CountSessionsFunc != nil {
		return f.CountSessionsFunc(ctx)
	}
	return domain.SessionCounts{}, nil
}

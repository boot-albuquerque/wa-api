package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain/apperr"
)

// appErrCode devolve o Code de um *AppError; usado para asserções de
// identidade sem importar diretamente o tipo concreto nos testes.
func appErrCode(err error) string {
	if e, ok := err.(*apperr.AppError); ok {
		return e.Code
	}
	return ""
}

// TestSessionGuardAdapter_EnsureSession_NoClient devolve ErrNoSession
// quando o getter devolve nil.
func TestSessionGuardAdapter_EnsureSession_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(getterWith(nil))
	err := a.EnsureSession(context.Background(), "missing-user")
	if err == nil {
		t.Fatal("EnsureSession with nil client returned nil error")
	}
	if got := appErrCode(err); got != "no_session" {
		t.Errorf("EnsureSession code = %q, want no_session", got)
	}
}

// TestSessionGuardAdapter_EnsureSession_WithClient devolve nil quando
// existe cliente.
func TestSessionGuardAdapter_EnsureSession_WithClient(t *testing.T) {
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	if err := a.EnsureSession(context.Background(), "u1"); err != nil {
		t.Errorf("EnsureSession with present client = %v, want nil", err)
	}
}

// TestSessionGuardAdapter_SessionStatus_NoClient devolve (false, false).
func TestSessionGuardAdapter_SessionStatus_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(getterWith(nil))
	connected, loggedIn := a.SessionStatus(context.Background(), "u1")
	if connected || loggedIn {
		t.Errorf("SessionStatus com nil client = (%v, %v), want (false, false)", connected, loggedIn)
	}
}

// TestSessionGuardAdapter_SessionStatus_ConnectedOnly devolve (true, false).
func TestSessionGuardAdapter_SessionStatus_ConnectedOnly(t *testing.T) {
	fake := &fakeWAClient{
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return false },
	}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if !c || l {
		t.Errorf("SessionStatus = (%v, %v), want (true, false)", c, l)
	}
}

// TestSessionGuardAdapter_SessionStatus_LoggedInOnly devolve (false, true).
func TestSessionGuardAdapter_SessionStatus_LoggedInOnly(t *testing.T) {
	fake := &fakeWAClient{
		IsConnectedFn: func() bool { return false },
		IsLoggedInFn:  func() bool { return true },
	}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if c || !l {
		t.Errorf("SessionStatus = (%v, %v), want (false, true)", c, l)
	}
}

// TestSessionGuardAdapter_SessionStatus_BothTrue devolve (true, true).
func TestSessionGuardAdapter_SessionStatus_BothTrue(t *testing.T) {
	fake := &fakeWAClient{
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return true },
	}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if !c || !l {
		t.Errorf("SessionStatus = (%v, %v), want (true, true)", c, l)
	}
}

// TestSessionGuardAdapter_Logout_NoClient devolve ErrNoSession.
func TestSessionGuardAdapter_Logout_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(getterWith(nil))
	err := a.Logout(context.Background(), "u1")
	if err == nil {
		t.Fatal("Logout with nil client returned nil error")
	}
	if appErrCode(err) != "no_session" {
		t.Errorf("Logout code = %q, want no_session", appErrCode(err))
	}
}

// TestSessionGuardAdapter_Logout_Success propaga o resultado do SDK.
func TestSessionGuardAdapter_Logout_Success(t *testing.T) {
	called := false
	fake := &fakeWAClient{
		LogoutFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.Logout(context.Background(), "u1"); err != nil {
		t.Fatalf("Logout = %v, want nil", err)
	}
	if !called {
		t.Error("Logout did not invoke client's Logout")
	}
}

// TestSessionGuardAdapter_Logout_PropagatesError propaga o erro do SDK.
func TestSessionGuardAdapter_Logout_PropagatesError(t *testing.T) {
	sdkErr := errors.New("not logged in")
	fake := &fakeWAClient{LogoutFn: func(ctx context.Context) error { return sdkErr }}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.Logout(context.Background(), "u1")
	if err == nil {
		t.Fatal("Logout did not propagate SDK error")
	}
	if err.Error() != "not logged in" {
		t.Errorf("Logout = %v, want %v", err, sdkErr)
	}
}

// TestSessionGuardAdapter_Disconnect_NoClient devolve ErrNoSession e
// NÃO chama Disconnect no client.
func TestSessionGuardAdapter_Disconnect_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(getterWith(nil))
	err := a.Disconnect(context.Background(), "u1")
	if err == nil {
		t.Fatal("Disconnect with nil client returned nil error")
	}
	if appErrCode(err) != "no_session" {
		t.Errorf("Disconnect code = %q, want no_session", appErrCode(err))
	}
}

// TestSessionGuardAdapter_Disconnect_Success chama Disconnect e devolve nil.
func TestSessionGuardAdapter_Disconnect_Success(t *testing.T) {
	called := false
	fake := &fakeWAClient{DisconnectFn: func() { called = true }}
	a := NewSessionGuardAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.Disconnect(context.Background(), "u1"); err != nil {
		t.Fatalf("Disconnect = %v, want nil", err)
	}
	if !called {
		t.Error("Disconnect did not invoke client's Disconnect")
	}
}

// TestErrNoSession_TipoEstruturado verifica o shape do erro: code, category,
// message e cause-chain.
func TestErrNoSession_TipoEstruturado(t *testing.T) {
	cause := errors.New("underlying")
	e := ErrNoSession("u1", cause)
	if e == nil {
		t.Fatal("ErrNoSession returned nil")
	}
	if e.Code != "no_session" {
		t.Errorf("code = %q, want no_session", e.Code)
	}
	if e.Category != apperr.CategoryValidation {
		t.Errorf("category = %v, want CategoryValidation", e.Category)
	}
	if e.Message != "no session" {
		t.Errorf("message = %q, want no session", e.Message)
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is não alcança a causa")
	}
}

// TestErrNoSession_NilCause preserva o caso sem causa.
func TestErrNoSession_NilCause(t *testing.T) {
	e := ErrNoSession("u1", nil)
	if e == nil {
		t.Fatal("ErrNoSession(nil cause) returned nil")
	}
}
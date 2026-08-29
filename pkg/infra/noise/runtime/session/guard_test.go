package session

import (
	"context"
	"errors"
	"testing"
	"wa-api/internal/noise"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"net/http"
	"strings"
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
	a := NewSessionGuardAdapter(testkit.GetterWith(nil))
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
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": &testkit.Fake{}}))
	if err := a.EnsureSession(context.Background(), "u1"); err != nil {
		t.Errorf("EnsureSession with present client = %v, want nil", err)
	}
}

// TestSessionGuardAdapter_SessionStatus_NoClient devolve (false, false).
func TestSessionGuardAdapter_SessionStatus_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(testkit.GetterWith(nil))
	connected, loggedIn := a.SessionStatus(context.Background(), "u1")
	if connected || loggedIn {
		t.Errorf("SessionStatus com nil client = (%v, %v), want (false, false)", connected, loggedIn)
	}
}

// TestSessionGuardAdapter_SessionStatus_ConnectedOnly devolve (true, false).
func TestSessionGuardAdapter_SessionStatus_ConnectedOnly(t *testing.T) {
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return false },
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if !c || l {
		t.Errorf("SessionStatus = (%v, %v), want (true, false)", c, l)
	}
}

// TestSessionGuardAdapter_SessionStatus_LoggedInOnly devolve (false, true).
func TestSessionGuardAdapter_SessionStatus_LoggedInOnly(t *testing.T) {
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return false },
		IsLoggedInFn:  func() bool { return true },
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if c || !l {
		t.Errorf("SessionStatus = (%v, %v), want (false, true)", c, l)
	}
}

// TestSessionGuardAdapter_SessionStatus_BothTrue devolve (true, true).
func TestSessionGuardAdapter_SessionStatus_BothTrue(t *testing.T) {
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return true },
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	c, l := a.SessionStatus(context.Background(), "u1")
	if !c || !l {
		t.Errorf("SessionStatus = (%v, %v), want (true, true)", c, l)
	}
}

// TestSessionGuardAdapter_Logout_NoClient devolve ErrNoSession.
func TestSessionGuardAdapter_Logout_NoClient(t *testing.T) {
	a := NewSessionGuardAdapter(testkit.GetterWith(nil))
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
	fake := &testkit.Fake{
		// Conectado: desde a F93 o Logout exige transporte vivo, porque sem ele
		// o IQ `remove-companion-device` nao tem para onde ir. Antes o dublê
		// nao precisava dizer isso, e o teste passava por omissao.
		IsConnectedFn: func() bool { return true },
		LogoutFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
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
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return true },
		LogoutFn:      func(ctx context.Context) error { return sdkErr },
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
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
	a := NewSessionGuardAdapter(testkit.GetterWith(nil))
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
	fake := &testkit.Fake{DisconnectFn: func() { called = true }}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
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

// TestSessionGuardAdapter_Logout_SemTransporteRecusa cobre a F93.
//
// O logout FALA com o WhatsApp: manda um IQ `remove-companion-device` antes de
// qualquer coisa. Sem transporte vivo o SDK devolvia erro CRU, que a fronteira
// HTTP transformava em 500 opaco — indistinguivel de defeito do servidor.
//
// A checagem e' de ESTADO (IsConnected) e nao do TEXTO do erro do SDK: casar a
// mensagem quebraria em silencio no dia em que ele mudasse a frase.
func TestSessionGuardAdapter_Logout_SemTransporteRecusa(t *testing.T) {
	chamou := false
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return false },
		LogoutFn: func(ctx context.Context) error {
			chamou = true
			return nil
		},
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))

	err := a.Logout(context.Background(), "u1")
	if err == nil {
		t.Fatal("Logout aceitou sessao sem transporte")
	}
	if chamou {
		t.Error("chamou o Logout do SDK sem transporte: e' a chamada que falha e produz o erro cru")
	}
	if got := appErrCode(err); got != apperr.CodeSessionNotConnected {
		t.Errorf("code = %q, quero %q", got, apperr.CodeSessionNotConnected)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		// 409, nao 500: a requisicao esta' correta, so' nao pode ser atendida
		// NESTE estado.
		if status := appErr.Category.HTTPStatus(); status != http.StatusConflict {
			t.Errorf("status = %d, quero %d", status, http.StatusConflict)
		}
		// A mensagem tem de dizer a SAIDA. A anterior nao dizia, e o remedio
		// (reconectar antes) era conhecimento de implementacao.
		if !strings.Contains(appErr.Message, "/session/connect") {
			t.Errorf("mensagem nao diz como sair do estado: %q", appErr.Message)
		}
	}
}

// TestSessionGuardAdapter_Logout_ConectadoSemPareamentoRecusa cobre a F275:
// transporte VIVO mas nunca emparelhado — o SDK devolve a sentinela crua
// noise.ErrNotLoggedIn ("the store doesn't contain a device JID"), e antes
// da correção esse erro cru subia até a fronteira HTTP como 500 opaco com
// envelope de texto simples.
//
// A checagem é por ESTADO (errors.Is contra a sentinela reexportada), pela
// MESMA razão que TestSessionGuardAdapter_Logout_SemTransporteRecusa já
// enuncia para o caso irmão — não por texto de erro.
func TestSessionGuardAdapter_Logout_ConectadoSemPareamentoRecusa(t *testing.T) {
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return true },
		LogoutFn: func(ctx context.Context) error {
			return noise.ErrNotLoggedIn
		},
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))

	err := a.Logout(context.Background(), "u1")
	if err == nil {
		t.Fatal("Logout aceitou sessao conectada mas nunca emparelhada")
	}
	if got := appErrCode(err); got != apperr.CodeSessionNotPaired {
		t.Errorf("code = %q, quero %q", got, apperr.CodeSessionNotPaired)
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro nao e' *apperr.AppError: %v (%T)", err, err)
	}
	// 409, nao 500: a sessao esta' correta (existe, tem transporte), so' nao
	// ha' o que desemparelhar NESTE estado.
	if status := appErr.Category.HTTPStatus(); status != http.StatusConflict {
		t.Errorf("status = %d, quero %d", status, http.StatusConflict)
	}
	if !errors.Is(err, noise.ErrNotLoggedIn) {
		t.Error("a cadeia de causa perdeu a sentinela original — errors.Is(err, noise.ErrNotLoggedIn) falhou")
	}
	// Distinto de CodeSessionNotConnected: sao dois estados diferentes e a
	// mensagem tem de dizer QUAL.
	if got := appErrCode(err); got == apperr.CodeSessionNotConnected {
		t.Error("F275 foi classificado como F93 (session_not_connected) — sao estados diferentes")
	}
}

// TestSessionGuardAdapter_Logout_OutroErroDeSDKContinuaCru: qualquer OUTRO
// erro do SDK (não a sentinela de "nunca emparelhado") continua a subir cru,
// exatamente como TestSessionGuardAdapter_Logout_PropagatesError já fixa —
// esta correção não pode virar um catch-all silencioso.
func TestSessionGuardAdapter_Logout_OutroErroDeSDKContinuaCru(t *testing.T) {
	sdkErr := errors.New("some other SDK failure")
	fake := &testkit.Fake{
		IsConnectedFn: func() bool { return true },
		LogoutFn:      func(ctx context.Context) error { return sdkErr },
	}
	a := NewSessionGuardAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))

	err := a.Logout(context.Background(), "u1")
	if !errors.Is(err, sdkErr) {
		t.Fatalf("Logout = %v, queria embrulhar %v sem traduzir", err, sdkErr)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		t.Fatalf("erro genérico do SDK foi tipado como %q — só ErrNotLoggedIn deve ser traduzido", appErr.Code)
	}
}

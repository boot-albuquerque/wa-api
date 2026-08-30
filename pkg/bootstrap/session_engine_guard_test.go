package bootstrap

import (
	"context"
	"errors"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// engineReaderStub is the same one-method spy pkg/pairing/registry_test.go
// uses over pairing.SessionEngineReader, duplicated here rather than
// exported from that package for a single test file.
type engineReaderStub struct {
	rows map[string]domain.Engine
	err  error
}

func (s *engineReaderStub) ListUsers(_ context.Context, id string) ([]domain.UserListEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	engine, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return []domain.UserListEntry{{ID: id, Engine: engine}}, nil
}

const (
	tgNoise    = "sess-noise"
	tgHeadless = "sess-headless"
	tgLegacy   = "sess-legacy"
	tgUnknown  = "sess-nao-cadastrada"
)

func newTestGuard(headlessDisconnector, headlessLogouter *contractsfake.SessionController) (*sessionEngineGuard, *contractsfake.SessionController) {
	users := &engineReaderStub{rows: map[string]domain.Engine{
		tgNoise:    domain.EngineNoise,
		tgHeadless: domain.EngineHeadless,
		tgLegacy:   domain.EngineLegacyUnknown,
	}}
	caps := capabilityregistry.NewCapabilityRegistry()
	noise := &contractsfake.SessionController{}
	var hd appport.SessionDisconnector
	var hl appport.SessionLogouter
	if headlessDisconnector != nil {
		hd = headlessDisconnector
	}
	if headlessLogouter != nil {
		hl = headlessLogouter
	}
	return newSessionEngineGuard(users, caps, noise, hd, hl), noise
}

// TestEnsureSessionDespachaPorEngineGravado: uma sessão gravada como
// headless tem EnsureSession respondido pelo adapter headless, não pelo
// noise — o defeito que este teste trava é a versão ANTERIOR a este
// arquivo, em que sessionGuard era sempre wasession.NewSessionGuardAdapter
// (só noise) e uma sessão headless falhava aqui incondicionalmente.
func TestEnsureSessionDespachaPorEngineGravado(t *testing.T) {
	headless := &contractsfake.SessionController{}
	guard, noise := newTestGuard(headless, nil)

	if err := guard.EnsureSession(context.Background(), tgHeadless); err != nil {
		t.Fatalf("EnsureSession(headless) = %v, want nil (adapter headless deveria responder)", err)
	}
	if len(headless.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter headless recebeu %d chamadas, want 1", len(headless.EnsureSessionCalls))
	}
	if len(noise.EnsureSessionCalls) != 0 {
		t.Fatalf("adapter noise recebeu %d chamadas para uma sessão headless, want 0", len(noise.EnsureSessionCalls))
	}

	if err := guard.EnsureSession(context.Background(), tgNoise); err != nil {
		t.Fatalf("EnsureSession(noise) = %v, want nil", err)
	}
	if len(noise.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter noise recebeu %d chamadas, want 1", len(noise.EnsureSessionCalls))
	}
}

// TestLegacyUnknownCaiEmNoise: uma linha sem engine gravado (migração
// anterior à coluna) resolve para noise, o mesmo default do backfill
// (pkg/infra/db/user_engine.go) e do pairing.Registry.TargetEngine.
func TestLegacyUnknownCaiEmNoise(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	if err := guard.EnsureSession(context.Background(), tgLegacy); err != nil {
		t.Fatalf("EnsureSession(legacy_unknown) = %v, want nil (deveria cair em noise)", err)
	}
	if len(noise.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter noise recebeu %d chamadas para legacy_unknown, want 1", len(noise.EnsureSessionCalls))
	}
}

// TestSemLinhaCaiEmNoise: sessão sem linha nenhuma também cai em
// noise, deixando o adapter produzir seu próprio erro de "sem sessão" em
// vez deste tipo inventar um segundo.
func TestSemLinhaCaiEmNoise(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	wantErr := errors.New("sem sessao")
	noise.EnsureSessionFunc = func(context.Context, string) error { return wantErr }

	if err := guard.EnsureSession(context.Background(), tgUnknown); !errors.Is(err, wantErr) {
		t.Fatalf("EnsureSession(sem linha) = %v, want %v (do adapter noise)", err, wantErr)
	}
}

// TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable: uma sessão
// gravada como headless num processo sem Chrome configurado (adapter
// nil) recusa com um erro identificável, não um nil silencioso nem um
// crash.
func TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable(t *testing.T) {
	guard, _ := newTestGuard(nil, nil)
	err := guard.Disconnect(context.Background(), tgHeadless)
	if err == nil {
		t.Fatal("Disconnect(headless, sem adapter) = nil, want erro")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != codeSessionEngineUnavailable {
		t.Fatalf("erro = %v, want AppError com código %q", err, codeSessionEngineUnavailable)
	}
}

// TestLogoutSemLogouterHeadlessRecusa: enquanto headlessLogouter continuar
// nil (Socket.logout não medido — ver
// pkg/infra/headless/session/disconnector.go) E a matriz continuar
// marcando logout_session como "unknown" para headless
// (pkg/capabilityregistry/matrix.go), Logout numa sessão headless recusa
// de forma identificável — hoje pela checagem de capacidade, que roda ANTES
// da resolução do adapter — e NÃO cai silenciosamente no noise. Se a
// matriz for atualizada para Supported sem que headlessLogouter seja
// wired, a recusa muda de código (capability_not_supported ->
// session_engine_unavailable) mas continua sendo uma recusa: é essa segunda
// garantia, não o código específico, que TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable
// já cobre para Disconnect.
func TestLogoutSemLogouterHeadlessRecusa(t *testing.T) {
	headlessDisconnector := &contractsfake.SessionController{}
	guard, noise := newTestGuard(headlessDisconnector, nil)

	err := guard.Logout(context.Background(), tgHeadless)
	if err == nil {
		t.Fatal("Logout(headless, sem logouter) = nil, want erro")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro = %v, want *apperr.AppError", err)
	}
	if appErr.Code != codeSessionCapabilityNotSupported && appErr.Code != codeSessionEngineUnavailable {
		t.Fatalf("código = %q, want %q ou %q", appErr.Code, codeSessionCapabilityNotSupported, codeSessionEngineUnavailable)
	}
	if len(noise.LogoutCalls) != 0 {
		t.Fatal("Logout(headless) não deveria ter caído no adapter noise")
	}
}

// TestSessionStatusSemLinhaDevolveFalseFalse: SessionStatus não tem erro na
// assinatura, então uma resolução impossível (sem linha, sem adapter) tem
// de devolver (false,false) honesto em vez de propagar.
func TestSessionStatusSemLinhaDevolveFalseFalse(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	noise.SessionStatusFunc = func(context.Context, string) (bool, bool) {
		t.Fatal("SessionStatus não deveria ter chegado ao adapter para uma sessão sem linha própria")
		return false, false
	}
	if c, l := guard.SessionStatus(context.Background(), tgHeadless); c || l {
		t.Fatalf("SessionStatus(headless, sem adapter) = (%v,%v), want (false,false)", c, l)
	}
}

// TestLogoutNoiseDespachaComSucesso: o caminho de sucesso de Logout —
// engine noise, capacidade suportada, logouter presente — chega ao
// adapter e devolve o que ele devolver, sem alteração.
func TestLogoutNoiseDespachaComSucesso(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	if err := guard.Logout(context.Background(), tgNoise); err != nil {
		t.Fatalf("Logout(noise) = %v, want nil", err)
	}
	if len(noise.LogoutCalls) != 1 {
		t.Fatalf("adapter noise recebeu %d chamadas a Logout, want 1", len(noise.LogoutCalls))
	}
}

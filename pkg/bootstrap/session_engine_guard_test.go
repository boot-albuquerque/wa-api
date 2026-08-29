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

func newTestGuard(waHeadlessDisconnector, waHeadlessLogouter *contractsfake.SessionController) (*sessionEngineGuard, *contractsfake.SessionController) {
	users := &engineReaderStub{rows: map[string]domain.Engine{
		tgNoise:    domain.EngineWaNoise,
		tgHeadless: domain.EngineWaHeadless,
		tgLegacy:   domain.EngineLegacyUnknown,
	}}
	caps := capabilityregistry.NewCapabilityRegistry()
	waNoise := &contractsfake.SessionController{}
	var hd appport.SessionDisconnector
	var hl appport.SessionLogouter
	if waHeadlessDisconnector != nil {
		hd = waHeadlessDisconnector
	}
	if waHeadlessLogouter != nil {
		hl = waHeadlessLogouter
	}
	return newSessionEngineGuard(users, caps, waNoise, hd, hl), waNoise
}

// TestEnsureSessionDespachaPorEngineGravado: uma sessão gravada como
// wa_headless tem EnsureSession respondido pelo adapter headless, não pelo
// wa_noise — o defeito que este teste trava é a versão ANTERIOR a este
// arquivo, em que sessionGuard era sempre wasession.NewSessionGuardAdapter
// (só wa_noise) e uma sessão wa_headless falhava aqui incondicionalmente.
func TestEnsureSessionDespachaPorEngineGravado(t *testing.T) {
	headless := &contractsfake.SessionController{}
	guard, noise := newTestGuard(headless, nil)

	if err := guard.EnsureSession(context.Background(), tgHeadless); err != nil {
		t.Fatalf("EnsureSession(wa_headless) = %v, want nil (adapter headless deveria responder)", err)
	}
	if len(headless.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter headless recebeu %d chamadas, want 1", len(headless.EnsureSessionCalls))
	}
	if len(noise.EnsureSessionCalls) != 0 {
		t.Fatalf("adapter wa_noise recebeu %d chamadas para uma sessão wa_headless, want 0", len(noise.EnsureSessionCalls))
	}

	if err := guard.EnsureSession(context.Background(), tgNoise); err != nil {
		t.Fatalf("EnsureSession(wa_noise) = %v, want nil", err)
	}
	if len(noise.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter wa_noise recebeu %d chamadas, want 1", len(noise.EnsureSessionCalls))
	}
}

// TestLegacyUnknownCaiEmWaNoise: uma linha sem engine gravado (migração
// anterior à coluna) resolve para wa_noise, o mesmo default do backfill
// (pkg/infra/db/user_engine.go) e do pairing.Registry.TargetEngine.
func TestLegacyUnknownCaiEmWaNoise(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	if err := guard.EnsureSession(context.Background(), tgLegacy); err != nil {
		t.Fatalf("EnsureSession(legacy_unknown) = %v, want nil (deveria cair em wa_noise)", err)
	}
	if len(noise.EnsureSessionCalls) != 1 {
		t.Fatalf("adapter wa_noise recebeu %d chamadas para legacy_unknown, want 1", len(noise.EnsureSessionCalls))
	}
}

// TestSemLinhaCaiEmWaNoise: sessão sem linha nenhuma também cai em
// wa_noise, deixando o adapter produzir seu próprio erro de "sem sessão" em
// vez deste tipo inventar um segundo.
func TestSemLinhaCaiEmWaNoise(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	wantErr := errors.New("sem sessao")
	noise.EnsureSessionFunc = func(context.Context, string) error { return wantErr }

	if err := guard.EnsureSession(context.Background(), tgUnknown); !errors.Is(err, wantErr) {
		t.Fatalf("EnsureSession(sem linha) = %v, want %v (do adapter wa_noise)", err, wantErr)
	}
}

// TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable: uma sessão
// gravada como wa_headless num processo sem Chrome configurado (adapter
// nil) recusa com um erro identificável, não um nil silencioso nem um
// crash.
func TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable(t *testing.T) {
	guard, _ := newTestGuard(nil, nil)
	err := guard.Disconnect(context.Background(), tgHeadless)
	if err == nil {
		t.Fatal("Disconnect(wa_headless, sem adapter) = nil, want erro")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != codeSessionEngineUnavailable {
		t.Fatalf("erro = %v, want AppError com código %q", err, codeSessionEngineUnavailable)
	}
}

// TestLogoutSemLogouterHeadlessRecusa: enquanto waHeadlessLogouter continuar
// nil (Socket.logout não medido — ver
// pkg/infra/wa-headless/session/disconnector.go) E a matriz continuar
// marcando logout_session como "unknown" para wa_headless
// (pkg/capabilityregistry/matrix.go), Logout numa sessão wa_headless recusa
// de forma identificável — hoje pela checagem de capacidade, que roda ANTES
// da resolução do adapter — e NÃO cai silenciosamente no wa_noise. Se a
// matriz for atualizada para Supported sem que waHeadlessLogouter seja
// wired, a recusa muda de código (capability_not_supported ->
// session_engine_unavailable) mas continua sendo uma recusa: é essa segunda
// garantia, não o código específico, que TestDisconnectSemAdapterHeadlessDevolveEngineUnavailable
// já cobre para Disconnect.
func TestLogoutSemLogouterHeadlessRecusa(t *testing.T) {
	headlessDisconnector := &contractsfake.SessionController{}
	guard, noise := newTestGuard(headlessDisconnector, nil)

	err := guard.Logout(context.Background(), tgHeadless)
	if err == nil {
		t.Fatal("Logout(wa_headless, sem logouter) = nil, want erro")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro = %v, want *apperr.AppError", err)
	}
	if appErr.Code != codeSessionCapabilityNotSupported && appErr.Code != codeSessionEngineUnavailable {
		t.Fatalf("código = %q, want %q ou %q", appErr.Code, codeSessionCapabilityNotSupported, codeSessionEngineUnavailable)
	}
	if len(noise.LogoutCalls) != 0 {
		t.Fatal("Logout(wa_headless) não deveria ter caído no adapter wa_noise")
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
		t.Fatalf("SessionStatus(wa_headless, sem adapter) = (%v,%v), want (false,false)", c, l)
	}
}

// TestLogoutWaNoiseDespachaComSucesso: o caminho de sucesso de Logout —
// engine wa_noise, capacidade suportada, logouter presente — chega ao
// adapter e devolve o que ele devolver, sem alteração.
func TestLogoutWaNoiseDespachaComSucesso(t *testing.T) {
	guard, noise := newTestGuard(nil, nil)
	if err := guard.Logout(context.Background(), tgNoise); err != nil {
		t.Fatalf("Logout(wa_noise) = %v, want nil", err)
	}
	if len(noise.LogoutCalls) != 1 {
		t.Fatalf("adapter wa_noise recebeu %d chamadas a Logout, want 1", len(noise.LogoutCalls))
	}
}

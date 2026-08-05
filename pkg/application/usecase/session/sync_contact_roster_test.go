// sync_contact_roster_test.go — SyncContactRosterUseCase.
//
// Capacidade nova e distinta de RequestHistorySyncUseCase: força o pull do
// patch de app-state que carrega a agenda de contatos, não histórico de
// mensagens. Por depender de uma porta própria (AppStateSyncer, não só
// SessionGuard), não entra na tabela guardCases() de session_test.go — tem
// sua própria bateria aqui.
package session_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

func TestSyncContactRoster_Success_PerMode(t *testing.T) {
	for _, mode := range []string{"if_unsynced", "incremental", "full"} {
		t.Run(mode, func(t *testing.T) {
			as := &contractsfake.AppStateSyncer{}
			log := &contractsfake.Logger{}

			res, err := session.NewSyncContactRosterUseCase(as, log).
				Execute(context.Background(), txtID, domain.SyncContactRosterRequest{Mode: mode})

			if err != nil {
				t.Fatalf("caminho feliz devolveu erro: %v", err)
			}
			if res.Mode != mode {
				t.Errorf("Result.Mode = %q, quero %q", res.Mode, mode)
			}
			if len(as.SyncContactRosterCalls) != 1 {
				t.Fatalf("SyncContactRoster chamadas = %d, quero 1", len(as.SyncContactRosterCalls))
			}
			call := as.SyncContactRosterCalls[0]
			if call.TxtID != txtID || call.Mode != mode {
				t.Errorf("SyncContactRosterCalls[0] = %+v, quero txtID=%q mode=%q", call, txtID, mode)
			}
			if got := len(log.ByLevel(contractsfake.LevelInfo)); got != 1 {
				t.Errorf("registros info = %d, quero 1: %v", got, log.Messages())
			}
			if got := len(log.ByLevel(contractsfake.LevelError)); got != 0 {
				t.Errorf("caminho feliz logou erro: %v", log.Messages())
			}
		})
	}
}

func TestSyncContactRoster_SemSessao_NaoChamaSyncENaoValidaModo(t *testing.T) {
	as := &contractsfake.AppStateSyncer{SessionGuard: contractsfake.FailSession(errNoSession)}
	log := &contractsfake.Logger{}

	_, err := session.NewSyncContactRosterUseCase(as, log).
		Execute(context.Background(), txtID, domain.SyncContactRosterRequest{Mode: "not-even-valid"})

	if !errors.Is(err, errNoSession) {
		t.Fatalf("a causa da porta se perdeu: %v", err)
	}
	if len(as.EnsureSessionCalls) != 1 || as.EnsureSessionCalls[0].TxtID != txtID {
		t.Errorf("EnsureSession: %+v", as.EnsureSessionCalls)
	}
	if len(as.SyncContactRosterCalls) != 0 {
		t.Errorf("SyncContactRoster foi chamada apesar da sessao recusada: %+v", as.SyncContactRosterCalls)
	}

	rec, found := log.FindLevel(contractsfake.LevelError, "no whatsmeow session")
	if !found {
		t.Fatalf("recusa de sessao nao foi logada em nivel error: %v", log.Messages())
	}
	if v, ok := rec.Keyval("txtID"); !ok || v != txtID {
		t.Errorf(`Keyval("txtID") = %v, %v; quero %q`, v, ok, txtID)
	}
}

func TestSyncContactRoster_ModoInvalido_NaoChamaSync(t *testing.T) {
	as := &contractsfake.AppStateSyncer{}
	log := &contractsfake.Logger{}

	_, err := session.NewSyncContactRosterUseCase(as, log).
		Execute(context.Background(), txtID, domain.SyncContactRosterRequest{Mode: "bogus"})

	if err == nil {
		t.Fatal("modo invalido devia ser recusado")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro nao e' um *apperr.AppError: %v", err)
	}
	if appErr.Category != apperr.CategoryValidation {
		t.Errorf("Category = %q, quero %q", appErr.Category, apperr.CategoryValidation)
	}
	if len(as.SyncContactRosterCalls) != 0 {
		t.Errorf("SyncContactRoster foi chamada apesar do modo invalido: %+v", as.SyncContactRosterCalls)
	}
	// EnsureSession roda ANTES da validacao de modo (ordem pedida pelo
	// plano), entao ela E' chamada aqui — diferente do modo invalido, que
	// nao chega a acionar o SDK.
	if len(as.EnsureSessionCalls) != 1 {
		t.Errorf("EnsureSession: %+v", as.EnsureSessionCalls)
	}
}

func TestSyncContactRoster_ErroDoSDKPropaga(t *testing.T) {
	sdkErr := errors.New("fetch app state: boom")
	as := &contractsfake.AppStateSyncer{
		SyncContactRosterFunc: func(context.Context, string, string) error { return sdkErr },
	}
	log := &contractsfake.Logger{}

	_, err := session.NewSyncContactRosterUseCase(as, log).
		Execute(context.Background(), txtID, domain.SyncContactRosterRequest{Mode: "incremental"})

	if !errors.Is(err, sdkErr) {
		t.Fatalf("erro do SDK nao propagou: %v", err)
	}
	if _, found := log.FindLevel(contractsfake.LevelError, "contact roster sync failed"); !found {
		t.Errorf("falha do SDK nao foi logada em nivel error: %v", log.Messages())
	}
}

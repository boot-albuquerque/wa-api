package pairing

import (
	"context"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

// TestStarterSatisfazAPorta garante que o adapter satisfaz a porta que o
// pairing.Registry vai resolver para GET /session/connect.
func TestStarterSatisfazAPorta(t *testing.T) {
	var s any = NewStarter(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := s.(appport.SessionStarter); !ok {
		t.Fatal("não satisfaz appport.SessionStarter")
	}
}

// TestCheckOwnership_SempreAceita: wa_headless não tem lease entre réplicas
// hoje — ver o comentário do próprio método sobre por quê. A checagem nunca
// recusa.
func TestCheckOwnership_SempreAceita(t *testing.T) {
	s := NewStarter(adapter.NewSessions(registry.New(1), cfgFor))
	if err := s.CheckOwnership(context.Background(), "qualquer-sessao"); err != nil {
		t.Fatalf("CheckOwnership = %v, want nil", err)
	}
}

// TestStartSession_NaoBloqueiaOChamador: StartSession é fire-and-forget por
// contrato (appport.SessionStarter) — a chamada tem de devolver antes do
// boot terminar, mesmo apontando para um binário que nunca vai subir.
func TestStartSession_NaoBloqueiaOChamador(t *testing.T) {
	s := NewStarter(adapter.NewSessions(registry.New(1), cfgFor))
	returned := make(chan struct{})
	go func() {
		s.StartSession(context.Background(), "sessao-fire-and-forget", "token")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("StartSession bloqueou o chamador por mais de 2s")
	}
}

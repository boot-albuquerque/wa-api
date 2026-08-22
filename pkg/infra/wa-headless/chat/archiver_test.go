package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// TestOAdaptadorNaoFingeSuportarOQueNaoTem é a decisão 80 travada em teste.
//
// Com uma interface única, este adaptador teria de implementar
// RequestUnavailableMessage para compilar, devolvendo "não suportado" — o tipo
// satisfeito e a capacidade mentida. Com portas por capacidade, a assimetria
// fica VISÍVEL no tipo, e este teste garante que continue visível: se alguém
// acrescentar os métodos que faltam só para "completar" o adaptador, ele passa
// a satisfazer a composição e o teste morde.
func TestOAdaptadorNaoFingeSuportarOQueNaoTem(t *testing.T) {
	var a any = NewArchiver(registry.New(1), cfgFor)

	if _, ok := a.(appport.ChatArchiver); !ok {
		t.Fatal("o adaptador não satisfaz ChatArchiver, que é o que ele existe para fazer")
	}
	if _, ok := a.(appport.ChatOperations); ok {
		t.Fatal("o adaptador satisfaz ChatOperations INTEIRA: alguém implementou " +
			"RequestUnavailableMessage num transporte que não decifra nada, ou " +
			"RejectCall num build onde o LEDGER regista reject como PARTIAL")
	}
	if _, ok := a.(appport.UnavailableMessageRequester); ok {
		t.Fatal("pedir reenvio de mensagem indecifrável não faz sentido num " +
			"driver de página: a página já entrega texto")
	}
}

// EnsureSession responde POSSE e não boota. A ADR-0005 D6 separa as duas, e uma
// guarda que subisse um browser para responder transformaria uma checagem
// barata num minuto de trabalho.
func TestEnsureSessionRespondePosseSemBootar(t *testing.T) {
	reg := registry.New(2)
	a := NewArchiver(reg, cfgFor)

	if err := a.EnsureSession(context.Background(), "desconhecida"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("got %v, want ErrUnknownSession", err)
	}

	if _, err := reg.Acquire("conhecida", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Nenhum browser subiu — o Holder é preguiçoso — e mesmo assim a posse é sim.
	if err := a.EnsureSession(context.Background(), "conhecida"); err != nil {
		t.Fatalf("EnsureSession de sessão detida falhou: %v", err)
	}
}

// A identidade é recusada ANTES de qualquer trabalho de sessão. A ordem importa:
// validar depois de adquirir slot gastaria capacidade limitada com um pedido que
// nunca poderia funcionar.
func TestIdentidadeInvalidaERecusadaAntesDeTocarNoRegistry(t *testing.T) {
	reg := registry.New(1)
	a := NewArchiver(reg, cfgFor)

	err := a.ArchiveChat(context.Background(), "s1", domain.JID("status@broadcast"), true)
	if err == nil {
		t.Fatal("um JID de broadcast foi aceito como conversa")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("o erro não nomeia a causa: %v", err)
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa de identidade consumiu um slot de sessão", reg.Len())
	}
}

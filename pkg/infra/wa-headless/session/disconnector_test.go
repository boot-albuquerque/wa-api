package session

import (
	"context"
	"errors"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// TestDesconectaMasNaoSAI trava a recusa por POLÍTICA, que é diferente das
// outras quatro categorias: aqui a capacidade FUNCIONA.
//
// A H122 mediu que Socket.logout existe e funciona neste build. Chamá-lo
// desempareia a conta, e restaurar exige um humano com o telefone. Implementar
// a porta para satisfazer o compilador significaria chamar a operação que
// funciona — apagando um pareamento que ninguém pediu para apagar.
func TestDesconectaMasNaoSAI(t *testing.T) {
	var d any = NewDisconnector(adapter.NewSessions(registry.New(1), cfgFor))

	if _, ok := d.(appport.SessionDisconnector); !ok {
		t.Fatal("não satisfaz SessionDisconnector, que é o que ele existe para fazer")
	}
	if _, ok := d.(appport.SessionLogouter); ok {
		t.Fatal("passou a satisfazer SessionLogouter: alguém implementou o logout, " +
			"que DESEMPAREIA a conta e exige um humano com o telefone para " +
			"restaurar — se isso foi deliberado, a H122 precisa de ser revista antes")
	}
	if _, ok := d.(appport.SessionController); ok {
		t.Fatal("satisfaz a composição inteira; ver acima")
	}
}

// Desconectar LIBERTA o slot, senão o teto vira uma contagem que só sobe.
func TestDesconectarLibertaOSlot(t *testing.T) {
	reg := registry.New(1)
	s := adapter.NewSessions(reg, cfgFor)
	d := NewDisconnector(s)

	if _, err := reg.Acquire("s1", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := d.Disconnect(context.Background(), "s1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d depois de desconectar: o slot não voltou", reg.Len())
	}
}

// Desconectar uma sessão que este processo não detém é ERRO, e não um silêncio
// simpático: quem chamou acha que derrubou algo, e o que derrubou era de outro.
func TestDesconectarSessaoAlheiaEErro(t *testing.T) {
	d := NewDisconnector(adapter.NewSessions(registry.New(1), cfgFor))
	if err := d.Disconnect(context.Background(), "fantasma"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("got %v, want ErrUnknownSession", err)
	}
}

// TestSessionStatusRelataPOSSEENaoAdivinha: a ADR-0005 D6 separa intenção de
// estado observado, e responder "conectado" sem olhar a página seria mentir —
// mas olhar exigiria subir um browser para responder a uma consulta de estado.
// Relatar posse é a resposta honesta disponível.
func TestSessionStatusRelataPOSSEENaoAdivinha(t *testing.T) {
	reg := registry.New(1)
	d := NewDisconnector(adapter.NewSessions(reg, cfgFor))

	if c, l := d.SessionStatus(context.Background(), "s1"); c || l {
		t.Fatalf("sessão não detida reportada como conectada=%v autenticada=%v", c, l)
	}
	if _, err := reg.Acquire("s1", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if c, l := d.SessionStatus(context.Background(), "s1"); !c || !l {
		t.Fatalf("sessão detida reportada como conectada=%v autenticada=%v", c, l)
	}
}

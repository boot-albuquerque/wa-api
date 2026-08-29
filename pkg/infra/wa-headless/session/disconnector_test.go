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

// TestSessionStatusNaoDetidaNaoSobe: sem posse, SessionStatus devolve
// (false, false) SEM tentar subir um browser — GET /session/status não pode
// custar um boot só para responder a um poll de sessão que nem existe aqui.
func TestSessionStatusNaoDetidaNaoSobe(t *testing.T) {
	d := NewDisconnector(adapter.NewSessions(registry.New(1), cfgFor))
	if c, l := d.SessionStatus(context.Background(), "s1"); c || l {
		t.Fatalf("sessão não detida reportada como conectada=%v autenticada=%v", c, l)
	}
}

// TestClassifyIdentity_NuncaPareouEConectadaNaoAutenticada: uma sessão que
// nunca confirmou identidade nenhuma é o caso normal de "esperando o
// escaneio" — connected=true, loggedIn=false, e NÃO marca everIdentity.
func TestClassifyIdentity_NuncaPareouEConectadaNaoAutenticada(t *testing.T) {
	connected, loggedIn, markSeen := classifyIdentity(false, false)
	if !connected || loggedIn || markSeen {
		t.Fatalf("classifyIdentity(everHad=false, present=false) = (%v,%v,%v), quer (true,false,false)",
			connected, loggedIn, markSeen)
	}
}

// TestClassifyIdentity_IdentidadePresenteMarcaEReportaPareada: identidade
// presente é sempre (true,true), e marca everIdentity — não importa se já
// tinha sido vista antes.
func TestClassifyIdentity_IdentidadePresenteMarcaEReportaPareada(t *testing.T) {
	for _, everHad := range []bool{false, true} {
		connected, loggedIn, markSeen := classifyIdentity(everHad, true)
		if !connected || !loggedIn || !markSeen {
			t.Fatalf("classifyIdentity(everHad=%v, present=true) = (%v,%v,%v), quer (true,true,true)",
				everHad, connected, loggedIn, markSeen)
		}
	}
}

// TestClassifyIdentity_PareouEDeslogouRemotoReportaDesconectada é o achado
// da F378: o usuário conectou no wa_headless, desconectou pelo APARELHO
// (desvincular no celular), e o painel mostrou "conectada, não
// autenticada" em vez de "desconectada" — o MESMO evento físico que no
// wa_noise produz "desconectada" (client.IsConnected() cai). MEDIDO ao
// vivo: a página se recupera sozinha para um QR novo e funcional
// (GET /session/pair/qr respondeu code_age_seconds=0 imediatamente), então
// RefreshOwnIdentity falhando (ou devolvendo ausente) é EXATAMENTE a mesma
// forma que uma sessão nunca pareada — só o histórico (everHad) distingue.
func TestClassifyIdentity_PareouEDeslogouRemotoReportaDesconectada(t *testing.T) {
	connected, loggedIn, markSeen := classifyIdentity(true, false)
	if connected || loggedIn || markSeen {
		t.Fatalf("classifyIdentity(everHad=true, present=false) = (%v,%v,%v), quer (false,false,false) — "+
			"F378: sessão que já pareou e perdeu a identidade tem de ser reportada como DESCONECTADA, "+
			"não como se estivesse ainda esperando o primeiro escaneio",
			connected, loggedIn, markSeen)
	}
}

// NOTA sobre cobertura de SessionStatus (F378): este pacote não tem como
// simular "identidade presente" na borda pública sem Chrome real — o
// Evaluator do adapter (`d.sessions.Evaluator`) falha ANTES de alcançar
// classifyIdentity sempre que `cfgFor` aponta para um binário inexistente
// (o mesmo caminho de TestSessionStatusDetidaMasIlegivelNaoAdivinha), então
// um teste que chamasse SessionStatus depois de markIdentitySeen passaria
// pela MESMA razão que o defeito original passava — não prova nada (é
// exactamente o "controle negativo que não morde" que ARMADILHAS.md #3
// cataloga; a primeira versão desta suíte tinha um assim, removido).
// classifyIdentity — a função pura acima — é onde a decisão realmente mora,
// e é isso que os três testes acima travam sem precisar de browser. A
// integração ponta a ponta foi medida ao vivo contra um Chrome real (ver
// HOUSEKEEP F378: `code_age_seconds=0` num QR novo depois do celular
// desvincular o aparelho, confirmando que o caminho "identidade ausente"
// realmente é alcançado nesse cenário).

// TestSessionStatusDetidaMasIlegivelNaoAdivinha: a ADR-0005 D6 separa
// intenção de estado observado. Uma sessão DETIDA cujo Evaluator não
// consegue ser lido (aqui, cfgFor aponta para um binário inexistente — o
// mesmo caso de "página inalcançável" que um Chrome real produziria) não
// vira "conectado" por posse: sem conseguir olhar a página, a resposta
// honesta continua (false, false), nunca uma suposição otimista.
func TestSessionStatusDetidaMasIlegivelNaoAdivinha(t *testing.T) {
	reg := registry.New(1)
	d := NewDisconnector(adapter.NewSessions(reg, cfgFor))

	if _, err := reg.Acquire("s1", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if c, l := d.SessionStatus(context.Background(), "s1"); c || l {
		t.Fatalf("sessão detida e ilegível reportada como conectada=%v autenticada=%v, quer (false,false)", c, l)
	}
}

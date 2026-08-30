package session

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
	adapter "wa-api/pkg/infra/headless"
	"wa-api/pkg/infra/headless/registry"
)

func cfgFor(string) (headless.StartConfig, error) {
	return headless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// TestDisconnectorSatisfazOControladorInteiro é o mirror de
// TestDesconectaMasNaoSAI (o nome antigo, que travava a RECUSA por
// política) — desde F381 (Socket.logout MEASURED, reopening H122) a
// recusa deixou de ser deliberada: Disconnector agora satisfaz
// appport.SessionController por inteiro, e um teste que ainda esperasse a
// recusa passaria a falhar exatamente como este passou a falhar quando o
// logout foi implementado — o sinal de que a medição mudou de fato, não
// uma regressão a mascarar.
func TestDisconnectorSatisfazOControladorInteiro(t *testing.T) {
	var d any = NewDisconnector(adapter.NewSessions(registry.New(1), cfgFor))

	if _, ok := d.(appport.SessionDisconnector); !ok {
		t.Fatal("não satisfaz SessionDisconnector")
	}
	if _, ok := d.(appport.SessionLogouter); !ok {
		t.Fatal("não satisfaz SessionLogouter — F381 devia ter fechado essa lacuna")
	}
	if _, ok := d.(appport.SessionController); !ok {
		t.Fatal("não satisfaz a composição inteira; ver acima")
	}
}

// TestLogout_EvaluatorInalcancavelDevolveSessionNotConnected: mesmo
// gatilho de TestSessionStatusDetidaMasIlegivelNaoAdivinha (cfgFor aponta
// para um binário inexistente — o mesmo caso de "página inalcançável" que
// um Chrome real produziria). Logout tem de recusar com o MESMO código que
// pkg/infra/noise/runtime/session/guard.go já usa para a condição
// idêntica (`!client.IsConnected()`): apperr.CodeSessionNotConnected, 409
// — é o código que LogoutUseCase.Execute verifica para chamar
// detacher.Detach mesmo em falha (F80), então divergir aqui quebraria essa
// limpeza de estado local para headless especificamente.
func TestLogout_EvaluatorInalcancavelDevolveSessionNotConnected(t *testing.T) {
	reg := registry.New(1)
	d := NewDisconnector(adapter.NewSessions(reg, cfgFor))

	if _, err := reg.Acquire("s1", headless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	err := d.Logout(context.Background(), "s1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("Logout devolveu %v (%T), quer um *apperr.AppError", err, err)
	}
	if appErr.Code != apperr.CodeSessionNotConnected {
		t.Fatalf("appErr.Code = %q, quer %q", appErr.Code, apperr.CodeSessionNotConnected)
	}
}

// Desconectar LIBERTA o slot, senão o teto vira uma contagem que só sobe.
func TestDesconectarLibertaOSlot(t *testing.T) {
	reg := registry.New(1)
	s := adapter.NewSessions(reg, cfgFor)
	d := NewDisconnector(s)

	if _, err := reg.Acquire("s1", headless.StartConfig{}, registry.KindOperational); err != nil {
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
// da F378: o usuário conectou no headless, desconectou pelo APARELHO
// (desvincular no celular), e o painel mostrou "conectada, não
// autenticada" em vez de "desconectada" — o MESMO evento físico que no
// noise produz "desconectada" (client.IsConnected() cai). MEDIDO ao
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

	if _, err := reg.Acquire("s1", headless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if c, l := d.SessionStatus(context.Background(), "s1"); c || l {
		t.Fatalf("sessão detida e ilegível reportada como conectada=%v autenticada=%v, quer (false,false)", c, l)
	}
}

package messenger

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
	"wa-api/pkg/infra/headless/registry"
)

func cfgFor(string) (headless.StartConfig, error) {
	return headless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

type markerDuplo struct {
	jid string
	err error
}

func (m *markerDuplo) MarkRead(_ context.Context, jid, _ string) (headless.MarkReadResult, error) {
	m.jid = jid
	return headless.MarkReadResult{}, m.err
}

type reactorDuplo struct {
	chamadas []string
	msgID    string
	emoji    string
	err      error
}

func (r *reactorDuplo) Add(_ context.Context, msgID, emoji, _ string) (headless.ReactionResult, error) {
	r.chamadas = append(r.chamadas, "add")
	r.msgID, r.emoji = msgID, emoji
	return headless.ReactionResult{}, r.err
}

func (r *reactorDuplo) Remove(_ context.Context, msgID, _ string) (headless.ReactionResult, error) {
	r.chamadas = append(r.chamadas, "remove")
	r.msgID = msgID
	return headless.ReactionResult{}, r.err
}

func com(mk marker, rc reactor) *Messenger {
	m := NewMessenger(adapter.NewSessions(registry.New(1), cfgFor))
	if mk != nil {
		m.newMarker = func(context.Context, string) (marker, error) { return mk, nil }
	}
	if rc != nil {
		m.newReactor = func(context.Context, string) (reactor, error) { return rc, nil }
	}
	return m
}

func TestSatisfazOPortDeMensageria(t *testing.T) {
	var m any = NewMessenger(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := m.(appport.ChatMessenger); !ok {
		t.Fatal("não satisfaz ChatMessenger")
	}
	// E NÃO cunha id: a página gera a chave ao enviar, e um id fornecido pelo
	// chamador não tem para onde ir.
	//
	// A asserção é feita contra uma interface ANÔNIMA, e não contra um port com
	// nome: appport.MessageComposer existia quando isto foi escrito e sumiu na
	// fusão de feature/wa-noise, que partiu a mensageria em portas menores.
	// A propriedade medida sobreviveu ao nome — amarrá-la de novo a um nome
	// faria o próximo rename apagar o guarda em silêncio.
	type cunhaID interface {
		NewMessageID(ctx context.Context, txtID string) (string, error)
	}
	if _, ok := m.(cunhaID); ok {
		t.Fatal("passou a cunhar id: alguém implementou NewMessageID num " +
			"transporte onde a chave da referência LANÇA (H98)")
	}
}

// O JID chega CONVERTIDO à capability, e não na grafia do socket.
func TestOJIDChegaConvertido(t *testing.T) {
	d := &markerDuplo{}
	if err := com(d, nil).MarkRead(context.Background(), "s1", nil, time.Time{},
		domain.JID("5511999999999@s.whatsapp.net"), ""); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if d.jid != "5511999999999"+headless.ServerPhone {
		t.Fatalf("a capability recebeu %q, quero a grafia da página", d.jid)
	}
}

// TestReacaoVaziaREMOVEEmVezDeAdicionarVazio: a convenção é do domínio — o use
// case transforma a palavra "remove" em string vazia antes de chegar aqui.
// Encaminhar as duas para Add deixaria a página adivinhar.
func TestReacaoVaziaREMOVEEmVezDeAdicionarVazio(t *testing.T) {
	comEmoji := &reactorDuplo{}
	if _, err := com(nil, comEmoji).SendReaction(context.Background(), "s1", "",
		domain.Reaction{TargetMessageID: "msg-1", Text: "👍"}); err != nil {
		t.Fatalf("SendReaction: %v", err)
	}
	if len(comEmoji.chamadas) != 1 || comEmoji.chamadas[0] != "add" {
		t.Fatalf("com emoji chamou %v, quero add", comEmoji.chamadas)
	}

	semEmoji := &reactorDuplo{}
	if _, err := com(nil, semEmoji).SendReaction(context.Background(), "s1", "",
		domain.Reaction{TargetMessageID: "msg-1"}); err != nil {
		t.Fatalf("SendReaction: %v", err)
	}
	if len(semEmoji.chamadas) != 1 || semEmoji.chamadas[0] != "remove" {
		t.Fatalf("sem emoji chamou %v, quero remove", semEmoji.chamadas)
	}
}

// Reação sem alvo é recusada: a página precisa da chave da mensagem, e uma
// vazia chegaria lá como busca por "".
func TestReacaoSemAlvoERecusada(t *testing.T) {
	d := &reactorDuplo{}
	if _, err := com(nil, d).SendReaction(context.Background(), "s1", "",
		domain.Reaction{Text: "👍"}); !errors.Is(err, errNoTarget) {
		t.Fatalf("got %v, want errNoTarget", err)
	}
	if len(d.chamadas) != 0 {
		t.Fatalf("a recusa ainda chamou a página: %v", d.chamadas)
	}
}

// A falha da capability propaga: marcar lida que falhou não pode passar por
// sucesso, senão o chamador limpa um badge que continua aceso.
func TestFalhaDaCapabilityPropaga(t *testing.T) {
	d := &markerDuplo{err: errors.New("a página recusou")}
	if err := com(d, nil).MarkRead(context.Background(), "s1", nil, time.Time{},
		domain.JID("5511999999999@c.us"), ""); err == nil {
		t.Fatal("a falha virou sucesso silencioso")
	}
}

// EnsureSession responde POSSE sem bootar — a ADR-0005 D6 separa posse de
// prontidão, e subir um browser para responder a uma consulta de estado
// transformaria uma checagem barata num minuto de trabalho.
func TestEnsureSessionRespondePosseSemBootar(t *testing.T) {
	reg := registry.New(2)
	m := NewMessenger(adapter.NewSessions(reg, cfgFor))

	if err := m.EnsureSession(context.Background(), "desconhecida"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("got %v, want ErrUnknownSession", err)
	}
	if _, err := reg.Acquire("conhecida", headless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := m.EnsureSession(context.Background(), "conhecida"); err != nil {
		t.Fatalf("posse negada para sessão detida: %v", err)
	}
}

// A falha ao resolver a CONFIGURAÇÃO propaga nos dois caminhos. Engoli-la faria
// o chamador acreditar que marcou lida — ou que reagiu — quando a sessão nem
// existe.
func TestFalhaDeConfiguracaoPropagaNosDoisCaminhos(t *testing.T) {
	semPerfil := func(string) (headless.StartConfig, error) {
		return headless.StartConfig{}, errors.New("sem perfil para esta sessão")
	}
	m := NewMessenger(adapter.NewSessions(registry.New(1), semPerfil))

	if err := m.MarkRead(context.Background(), "s1", nil, time.Time{},
		domain.JID("5511999999999@c.us"), ""); err == nil {
		t.Fatal("MarkRead: falha de configuração virou sucesso")
	}
	if _, err := m.SendReaction(context.Background(), "s1", "",
		domain.Reaction{TargetMessageID: "msg-1", Text: "👍"}); err == nil {
		t.Fatal("SendReaction: falha de configuração virou reação enviada")
	}
}

// A identidade inválida é recusada ANTES de resolver a sessão, para que um
// pedido impossível não gaste capacidade limitada.
func TestIdentidadeInvalidaNaoGastaSlot(t *testing.T) {
	reg := registry.New(1)
	m := NewMessenger(adapter.NewSessions(reg, cfgFor))

	if err := m.MarkRead(context.Background(), "s1", nil, time.Time{},
		domain.JID("status@broadcast"), ""); err == nil {
		t.Fatal("um broadcast foi aceito como conversa")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot de sessão", reg.Len())
	}
}

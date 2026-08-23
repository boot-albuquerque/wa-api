package messenger

import (
	"context"
	"errors"
	"testing"
	"time"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

type markerDuplo struct {
	jid string
	err error
}

func (m *markerDuplo) MarkRead(_ context.Context, jid, _ string) (waheadless.MarkReadResult, error) {
	m.jid = jid
	return waheadless.MarkReadResult{}, m.err
}

type reactorDuplo struct {
	chamadas []string
	msgID    string
	emoji    string
	err      error
}

func (r *reactorDuplo) Add(_ context.Context, msgID, emoji, _ string) (waheadless.ReactionResult, error) {
	r.chamadas = append(r.chamadas, "add")
	r.msgID, r.emoji = msgID, emoji
	return waheadless.ReactionResult{}, r.err
}

func (r *reactorDuplo) Remove(_ context.Context, msgID, _ string) (waheadless.ReactionResult, error) {
	r.chamadas = append(r.chamadas, "remove")
	r.msgID = msgID
	return waheadless.ReactionResult{}, r.err
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
	// E NÃO satisfaz MessageComposer, que este transporte recusa: a página cunha
	// o id ao enviar, e um id fornecido pelo chamador não tem para onde ir.
	if _, ok := m.(appport.MessageComposer); ok {
		t.Fatal("passou a satisfazer MessageComposer: alguém implementou " +
			"NewMessageID num transporte onde a chave da referência LANÇA (H98)")
	}
}

// O JID chega CONVERTIDO à capability, e não na grafia do socket.
func TestOJIDChegaConvertido(t *testing.T) {
	d := &markerDuplo{}
	if err := com(d, nil).MarkRead(context.Background(), "s1", nil, time.Time{},
		domain.JID("5511999999999@s.whatsapp.net"), ""); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if d.jid != "5511999999999"+waheadless.ServerPhone {
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

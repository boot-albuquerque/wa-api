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

// setterDuplo registra o JID e o valor que chegaram à capability — que é o que
// este adaptador decide. A capability já tem os seus próprios testes.
type setterDuplo struct {
	jid      string
	archived bool
	err      error
}

func (s *setterDuplo) SetArchived(_ context.Context, jid string, archived bool, _ string) (waheadless.ChatStateChange, error) {
	s.jid, s.archived = jid, archived
	return waheadless.ChatStateChange{}, s.err
}

func comSetter(s setter) *Archiver {
	a := NewArchiver(registry.New(1), cfgFor)
	a.newSetter = func(context.Context, string) (setter, error) { return s, nil }
	return a
}

// TestOJIDChegaConvertidoACapability é a prova de ponta a ponta da decisão 74
// DENTRO do adaptador: o que a página recebe é a grafia dela, não a do socket.
func TestOJIDChegaConvertidoACapability(t *testing.T) {
	d := &setterDuplo{}
	a := comSetter(d)

	// Entrada na grafia do SOCKET, que é o caso perigoso.
	if err := a.ArchiveChat(context.Background(), "s1", domain.JID("5511999999999@s.whatsapp.net"), true); err != nil {
		t.Fatalf("ArchiveChat: %v", err)
	}
	if strings.HasSuffix(d.jid, "@s.whatsapp.net") {
		t.Fatalf("a capability recebeu %q, a grafia do SOCKET: a página "+
			"responderia 'não há tal conversa' para uma conversa presente", d.jid)
	}
	if !strings.HasSuffix(d.jid, waheadless.ServerPhone) {
		t.Fatalf("a capability recebeu %q, quero sufixo %q", d.jid, waheadless.ServerPhone)
	}
}

// Arquivar e desarquivar passam o valor CORRETO. Inverter o booleano é o bug
// que compila, passa em todo teste de tipo, e faz o oposto do pedido.
func TestOValorDeArquivamentoNaoEInvertido(t *testing.T) {
	for _, quer := range []bool{true, false} {
		d := &setterDuplo{}
		if err := comSetter(d).ArchiveChat(context.Background(), "s1",
			domain.JID("5511999999999@c.us"), quer); err != nil {
			t.Fatalf("ArchiveChat(%v): %v", quer, err)
		}
		if d.archived != quer {
			t.Fatalf("pedido %v chegou como %v à capability", quer, d.archived)
		}
	}
}

// A falha da capability PROPAGA. Engoli-la faria o chamador acreditar que
// arquivou — silêncio com forma de sucesso, que é o que a invariante 14 proíbe.
func TestFalhaDaCapabilityPropaga(t *testing.T) {
	d := &setterDuplo{err: errors.New("a página recusou")}
	if err := comSetter(d).ArchiveChat(context.Background(), "s1",
		domain.JID("5511999999999@c.us"), true); err == nil {
		t.Fatal("a falha da página virou sucesso silencioso")
	}
}

// A falha ao resolver a CONFIGURAÇÃO propaga, em vez de virar sucesso mudo.
func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	a := NewArchiver(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	})
	if err := a.ArchiveChat(context.Background(), "s1", domain.JID("5511999999999@c.us"), true); err == nil {
		t.Fatal("falha de configuração virou sucesso")
	}
}

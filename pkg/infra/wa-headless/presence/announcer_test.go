package presence

import (
	"context"
	"errors"
	"strings"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

func novo() *Announcer { return NewAnnouncer(adapter.NewSessions(registry.New(1), cfgFor)) }

// TestAnunciaMasNaoAssina trava a recusa por DEPENDÊNCIA HUMANA.
//
// É uma categoria diferente da recusa da decisão 80: lá,
// RequestUnavailableMessage não faz SENTIDO para quem dirige a página. Aqui a
// operação faz todo o sentido e simplesmente não chega — a H144 mediu
// `isMyContact:false isAddressBookContact:false`, e o vínculo de agenda cria-se
// no TELEFONE.
//
// Distinguir as duas importa: uma nunca vai ser implementada, a outra passa a
// funcionar no dia em que um humano salvar o contato. Tratá-las como a mesma
// coisa perderia essa informação.
func TestAnunciaMasNaoAssina(t *testing.T) {
	var a any = novo()

	if _, ok := a.(appport.PresenceAnnouncer); !ok {
		t.Fatal("não satisfaz PresenceAnnouncer, que é o que ele existe para fazer")
	}
	if _, ok := a.(appport.PresenceSubscriber); ok {
		t.Fatal("passou a satisfazer PresenceSubscriber: alguém implementou a " +
			"assinatura de presença, que a H144 mediu como dependente de uma ação " +
			"NO TELEFONE — se isso mudou, a H144 precisa de ser revista primeiro")
	}
	if _, ok := a.(appport.PresenceController); ok {
		t.Fatal("satisfaz a composição inteira; ver acima")
	}
}

// O estado de conversa é mapeado contra um conjunto FECHADO. O port aceita
// string livre porque o upstream nunca validou, e repassá-la transformaria um
// typo do chamador numa chamada a função de página que não existe.
func TestEstadoDeConversaEMapeadoContraConjuntoFechado(t *testing.T) {
	a := novo()
	err := a.SendChatPresence(context.Background(), "s1", domain.JID("5511999999999@c.us"), "datilografando", "")
	if err == nil {
		t.Fatal("um estado desconhecido foi aceito e seguiu para a página")
	}
	if !errors.Is(err, waheadless.ErrUnknownPresenceState) {
		t.Fatalf("erro não identifica a causa: %v", err)
	}
}

// O vocabulário do OUTRO transporte continua a funcionar: um chamador escrito
// contra o socket não deve quebrar ao trocar de transporte.
func TestOVocabularioDoSocketContinuaAceito(t *testing.T) {
	a := novo()
	for _, estado := range []string{"composing", "typing", "paused", "stopped", "recording", "audio"} {
		err := a.SendChatPresence(context.Background(), "s1", domain.JID("5511999999999@c.us"), estado, "")
		if errors.Is(err, waheadless.ErrUnknownPresenceState) {
			t.Errorf("estado %q recusado como desconhecido", estado)
		}
	}
}

// Presença global desconhecida é recusada, e não silenciosamente tratada como
// offline: "não mandámos nada" e "mandámos indisponível" são coisas diferentes.
func TestPresencaGlobalDesconhecidaERecusada(t *testing.T) {
	a := novo()
	err := a.SendPresence(context.Background(), "s1", domain.PresenceType("talvez"))
	if err == nil {
		t.Fatal("presença desconhecida foi aceita")
	}
	if !strings.Contains(err.Error(), "unknown presence") {
		t.Fatalf("erro não nomeia a causa: %v", err)
	}
}

// A validação acontece ANTES de tocar no registry: um pedido que nunca poderia
// funcionar não pode gastar capacidade limitada.
func TestValidacaoAconteceAntesDeGastarSlot(t *testing.T) {
	reg := registry.New(1)
	a := NewAnnouncer(adapter.NewSessions(reg, cfgFor))

	_ = a.SendPresence(context.Background(), "s1", domain.PresenceType("talvez"))
	_ = a.SendChatPresence(context.Background(), "s1", domain.JID("status@broadcast"), "composing", "")

	if reg.Len() != 0 {
		t.Fatalf("Len=%d: uma recusa de validação consumiu slot de sessão", reg.Len())
	}
}

type announcerDuplo struct {
	jid        string
	estado     waheadless.PresenceState
	disponivel bool
	online     bool
}

func (a *announcerDuplo) Set(_ context.Context, jid string, s waheadless.PresenceState, _ string) error {
	a.jid, a.estado = jid, s
	return nil
}

func (a *announcerDuplo) SetOnline(_ context.Context, available bool, _ string) error {
	a.online, a.disponivel = true, available
	return nil
}

func comAnnouncer(d announcer) *Announcer {
	a := NewAnnouncer(adapter.NewSessions(registry.New(1), cfgFor))
	a.newAnnouncer = func(context.Context, string) (announcer, error) { return d, nil }
	return a
}

// O vocabulário do socket é TRADUZIDO para o estado desta página, e não
// repassado. Um "typing" cru chamaria uma função de página que não existe.
func TestOVocabularioDoSocketETraduzidoENaoRepassado(t *testing.T) {
	d := &announcerDuplo{}
	if err := comAnnouncer(d).SendChatPresence(context.Background(), "s1",
		domain.JID("5511999999999@c.us"), "typing", ""); err != nil {
		t.Fatalf("SendChatPresence: %v", err)
	}
	if d.estado != waheadless.PresenceComposing {
		t.Fatalf("estado=%q, quero %q: o vocabulário do socket não foi traduzido",
			d.estado, waheadless.PresenceComposing)
	}
}

// Disponível e indisponível chegam com o valor CERTO. Inverter o booleano
// anunciaria o oposto do pedido, e compila.
func TestPresencaGlobalNaoEInvertida(t *testing.T) {
	for _, caso := range []struct {
		p    domain.PresenceType
		quer bool
	}{{domain.PresenceAvailable, true}, {domain.PresenceUnavailable, false}} {
		d := &announcerDuplo{}
		if err := comAnnouncer(d).SendPresence(context.Background(), "s1", caso.p); err != nil {
			t.Fatalf("SendPresence(%v): %v", caso.p, err)
		}
		if !d.online || d.disponivel != caso.quer {
			t.Fatalf("%v chegou como disponivel=%v", caso.p, d.disponivel)
		}
	}
}

// A falha ao resolver a CONFIGURAÇÃO propaga.
func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	a := NewAnnouncer(adapter.NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	}))
	if err := a.SendPresence(context.Background(), "s1", domain.PresenceAvailable); err == nil {
		t.Fatal("falha de configuração virou anúncio bem-sucedido")
	}
}

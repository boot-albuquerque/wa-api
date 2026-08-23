package identity

import (
	"context"
	"errors"
	"fmt"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

func TestSatisfazOPortDeIdentidade(t *testing.T) {
	var r any = NewResolver(registry.New(1), cfgFor)
	if _, ok := r.(appport.IdentityResolver); !ok {
		t.Fatal("não satisfaz IdentityResolver")
	}
	// E NÃO satisfaz a composição, que exigiria avatar e roster.
	if _, ok := r.(appport.ContactDirectory); ok {
		t.Fatal("satisfaz ContactDirectory inteira: alguém juntou capacidades " +
			"que a decisão 82 separou de propósito")
	}
}

// A identidade é convertida ANTES de resolver a sessão, como nos demais
// adaptadores: um pedido impossível não gasta capacidade limitada.
func TestIdentidadeInvalidaNaoGastaSlot(t *testing.T) {
	reg := registry.New(1)
	r := NewResolver(reg, cfgFor)

	if _, err := r.GetLIDForPN(context.Background(), "s1", domain.JID("status@broadcast")); err == nil {
		t.Fatal("um broadcast foi aceito como identidade de pessoa")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot", reg.Len())
	}
}

// TestListaVaziaNaoAbreSessao: pedir a resolução de NADA não deve subir browser.
// É o caso degenerado que costuma passar despercebido, e o custo aqui é um
// Chrome inteiro.
func TestListaVaziaNaoAbreSessao(t *testing.T) {
	reg := registry.New(1)
	r := NewResolver(reg, cfgFor)

	got, err := r.GetManyLIDsForPNs(context.Background(), "s1", nil)
	if err == nil && len(got) != 0 {
		t.Fatalf("lista vazia devolveu %d entradas", len(got))
	}
}

// TestFalhaDeLookupABORTAEmVezDeVirarAusencia trava a regra que um controle
// negativo mostrou estar SOLTA: ela vivia num comentário, inline, e invertê-la
// compilava com a suíte verde.
//
// A distinção é a diferença entre um fato e um palpite com forma de fato:
// "não está no WhatsApp" é resposta definitiva; um timeout não é resposta
// nenhuma, e reportá-lo como ausência faria o chamador agir sobre uma ausência
// que nunca foi medida.
func TestFalhaDeLookupABORTAEmVezDeVirarAusencia(t *testing.T) {
	naoEsta := fmt.Errorf("envelope: %w", waheadless.ErrNotOnWhatsApp)
	if check, err := classifyCheck("5511999999999", waheadless.Identity{}, naoEsta); err != nil {
		t.Fatalf("ErrNotOnWhatsApp devia ser resposta, não falha: %v", err)
	} else if check.IsIn {
		t.Fatal("ErrNotOnWhatsApp virou IsIn=true")
	}

	falhaReal := errors.New("a página não respondeu")
	if _, err := classifyCheck("5511999999999", waheadless.Identity{}, falhaReal); err == nil {
		t.Fatal("uma falha REAL virou uma resposta: o chamador concluiria " +
			"'não está no WhatsApp' a partir de um erro que nunca mediu nada")
	}

	if check, err := classifyCheck("5511999999999", waheadless.Identity{JID: "x@lid"}, nil); err != nil || !check.IsIn {
		t.Fatalf("sucesso mal classificado: check=%+v err=%v", check, err)
	}
}

// lookuperDuplo devolve o par que a página daria. Ele imita a REGRA: qualquer
// metade pode vir vazia, e vazio significa "a página não produziu", nunca
// "não existe" — que é o que o tipo Pair documenta.
type lookuperDuplo struct {
	pares map[string]waheadless.IdentityPair
	err   error
}

func (l lookuperDuplo) NumberID(context.Context, string, string) (waheadless.Identity, error) {
	return waheadless.Identity{}, l.err
}

func (l lookuperDuplo) LidAndPhone(_ context.Context, jid, _ string) (waheadless.IdentityPair, error) {
	if l.err != nil {
		return waheadless.IdentityPair{}, l.err
	}
	return l.pares[jid], nil
}

func comLookuper(l lookuper) *Resolver {
	r := NewResolver(registry.New(1), cfgFor)
	r.newLookuper = func(context.Context, string) (lookuper, error) { return l, nil }
	return r
}

// TestMapeamentoAusenteEJIDVaziaSemErro trava o contrato escrito no port:
// ausência de mapeamento é RESPOSTA, não falha. Devolver erro aqui faria o
// chamador tratar "esta pessoa não tem LID conhecido" como avaria.
func TestMapeamentoAusenteEJIDVaziaSemErro(t *testing.T) {
	r := comLookuper(lookuperDuplo{pares: map[string]waheadless.IdentityPair{}})

	got, err := r.GetLIDForPN(context.Background(), "s1", domain.JID("5511999999999@c.us"))
	if err != nil {
		t.Fatalf("ausência de mapeamento virou erro: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, quero vazia", got)
	}
}

// As duas direções não são a mesma: GetPNForLID devolve a metade PN, e
// GetLIDForPN a metade LID. Trocá-las compila, porque os tipos são iguais.
func TestAsDuasDirecoesNaoSaoTrocadas(t *testing.T) {
	const entrada = "5511999999999@c.us"
	l := lookuperDuplo{pares: map[string]waheadless.IdentityPair{
		entrada: {LID: "111@lid", PN: "5511999999999@c.us"},
	}}

	lid, err := comLookuper(l).GetLIDForPN(context.Background(), "s1", domain.JID(entrada))
	if err != nil || string(lid) != "111@lid" {
		t.Fatalf("GetLIDForPN devolveu %q (err=%v), quero o LID", lid, err)
	}
	pn, err := comLookuper(l).GetPNForLID(context.Background(), "s1", domain.JID(entrada))
	if err != nil || string(pn) != "5511999999999@c.us" {
		t.Fatalf("GetPNForLID devolveu %q (err=%v), quero o PN", pn, err)
	}
}

// TestLoteIgnoraOQueNaoResolveEmVezDeFalhar: o port diz que PN sem mapeamento
// fica AUSENTE do mapa. Falhar o lote inteiro por causa de um faria o chamador
// perder a informação sobre todos os outros.
func TestLoteIgnoraOQueNaoResolveEmVezDeFalhar(t *testing.T) {
	l := lookuperDuplo{pares: map[string]waheadless.IdentityPair{
		"5511111111111@c.us": {LID: "111@lid"},
		// o segundo não está no mapa: a página não produziu
	}}
	r := comLookuper(l)

	got, err := r.GetManyLIDsForPNs(context.Background(), "s1", []domain.JID{
		"5511111111111@c.us", "5522222222222@c.us",
	})
	if err != nil {
		t.Fatalf("o lote falhou por causa de uma entrada sem mapeamento: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mapa=%v, quero só a entrada que resolveu", got)
	}
}

// A falha ao resolver a CONFIGURAÇÃO propaga, em vez de virar "não está no
// WhatsApp" — que seria de novo o palpite com forma de fato.
func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	r := NewResolver(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	})
	if _, err := r.IsOnWhatsApp(context.Background(), "s1", []string{"5511999999999"}); err == nil {
		t.Fatal("falha de configuração virou 'não está no WhatsApp'")
	}
}

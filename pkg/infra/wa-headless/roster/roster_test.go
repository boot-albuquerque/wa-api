package roster

import (
	"context"
	"errors"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// listerDuplo imita a REGRA medida: Rows é a contagem de LINHAS da coleção, que
// é MAIOR que o número de pessoas porque a mesma pessoa chega em duas linhas
// (H129: 944 linhas dobradas em 390 pessoas). Um dublê que devolvesse Rows igual
// ao número de contatos esconderia justamente o defeito que este teste procura.
type listerDuplo struct {
	roster waheadless.ContactRoster
	err    error
}

func (l listerDuplo) List(context.Context, string) (waheadless.ContactRoster, error) {
	return l.roster, l.err
}

func comLister(l lister) *Roster {
	r := NewRoster(registry.New(1), cfgFor)
	r.newLister = func(context.Context, string) (lister, error) { return l, nil }
	return r
}

func rosterDeProva() waheadless.ContactRoster {
	return waheadless.ContactRoster{
		Contacts: []waheadless.RosterContact{
			{PN: "5511111111111@c.us", LID: "111@lid", Pushname: "Ana"},
			{LID: "222@lid", Pushname: "Bruno", VerifiedName: "Bruno LTDA"},
		},
		// Quatro linhas para duas pessoas: é o estado NORMAL.
		Rows: 4, Merged: 2,
	}
}

func TestSatisfazOPortDeRoster(t *testing.T) {
	var r any = NewRoster(registry.New(1), cfgFor)
	if _, ok := r.(appport.ContactRoster); !ok {
		t.Fatal("não satisfaz ContactRoster")
	}
}

// TestAContagemEDePESSOASENaoDeLINHAS: a coleção da página tem uma linha por
// identidade, então devolver Rows reportaria cerca do DOBRO dos contatos que
// alguém tem.
func TestAContagemEDePESSOASENaoDeLINHAS(t *testing.T) {
	_, n, err := comLister(listerDuplo{roster: rosterDeProva()}).
		GetAllContacts(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	if n == 4 {
		t.Fatal("a contagem devolveu as LINHAS da coleção: o chamador veria o " +
			"dobro dos contatos que tem")
	}
	if n != 2 {
		t.Fatalf("contagem=%d, quero 2 pessoas", n)
	}
}

// TestPessoaFundidaEIndexadaPelasDUASIdentidades: quem chega com PN e LID tem de
// ser encontrável por qualquer das duas metades, porque o chamador não escolhe
// qual recebeu — o histórico fala telefone e a coleção de mensagens fala lid.
func TestPessoaFundidaEIndexadaPelasDUASIdentidades(t *testing.T) {
	nomes, err := comLister(listerDuplo{roster: rosterDeProva()}).
		ContactNames(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ContactNames: %v", err)
	}
	if _, ok := nomes[domain.JID("5511111111111@c.us")]; !ok {
		t.Error("a pessoa fundida não é encontrável pelo PN")
	}
	if _, ok := nomes[domain.JID("111@lid")]; !ok {
		t.Error("a pessoa fundida não é encontrável pelo LID")
	}
}

// TestNomesVaziosDegradamPelaOrdemDeclarada: FullName e FirstName ficam vazios
// porque getName responde 1 de 944 no perfil medido — é propriedade do PERFIL,
// não do build. O domínio já declarou a ordem de degradação, e este teste prova
// que ela funciona em vez de deixar o vazio parecer defeito.
func TestNomesVaziosDegradamPelaOrdemDeclarada(t *testing.T) {
	nomes, err := comLister(listerDuplo{roster: rosterDeProva()}).
		ContactNames(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ContactNames: %v", err)
	}

	ana := nomes[domain.JID("111@lid")]
	if ana.FullName != "" || ana.FirstName != "" {
		t.Fatalf("esta página não lê a agenda; got FullName=%q FirstName=%q",
			ana.FullName, ana.FirstName)
	}
	if ana.Melhor() != "Ana" {
		t.Fatalf("Melhor()=%q: a degradação para PushName não aconteceu", ana.Melhor())
	}

	bruno := nomes[domain.JID("222@lid")]
	if bruno.Melhor() != "Bruno" {
		t.Fatalf("Melhor()=%q, quero o PushName antes do BusinessName", bruno.Melhor())
	}
}

// GetUserInfo devolve SÓ os JIDs pedidos, e não o roster inteiro.
func TestGetUserInfoFiltraPelosJIDsPedidos(t *testing.T) {
	got, err := comLister(listerDuplo{roster: rosterDeProva()}).
		GetUserInfo(context.Background(), "s1", []domain.JID{"111@lid"})
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	m, ok := got.(map[domain.JID]domain.ContactName)
	if !ok {
		t.Fatalf("tipo inesperado: %T", got)
	}
	if len(m) != 1 {
		t.Fatalf("devolveu %d entradas para 1 JID pedido", len(m))
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	_, _, err := comLister(listerDuplo{err: errors.New("a página recusou")}).
		GetAllContacts(context.Background(), "s1")
	if err == nil {
		t.Fatal("a falha virou roster vazio")
	}
}

func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	r := NewRoster(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	})
	if _, err := r.ContactNames(context.Background(), "s1"); err == nil {
		t.Fatal("falha de configuração virou roster vazio")
	}
}

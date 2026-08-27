package newsletter

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

type followerDuplo struct {
	entries []waheadless.ChannelEntry
	err     error
}

func (f followerDuplo) Followed(context.Context, string) ([]waheadless.ChannelEntry, error) {
	return f.entries, f.err
}

func comFollower(f follower) *Reader {
	r := NewReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.newFollower = func(context.Context, string) (follower, error) { return f, nil }
	return r
}

// NÃO satisfaz NewsletterReader, e o teste trava a HONESTIDADE disso.
//
// A fusão de feature/wa-noise levou a porta de 2 para 13 métodos. Este
// adaptador serve dois — ListSubscribed e CreateNewsletter — e declarar que
// serve a porta seria a mentira que a decisão 80 existe para impedir.
//
// O teste é escrito ao contrário de propósito: quando alguém implementar os
// onze que faltam, ele FALHA e obriga a decidir — satisfazer a porta inteira,
// ou parti-la pela decisão 92. Um teste que só verificasse os dois métodos
// deixaria o adaptador crescer até meio-satisfazer para sempre.
func TestNaoFingeSatisfazerOPortInteiroDeNewsletter(t *testing.T) {
	var r any = NewReader(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := r.(appport.NewsletterReader); ok {
		t.Fatal("passou a satisfazer NewsletterReader: decida entre servir os 13 " +
			"métodos de verdade ou partir a porta (decisão 92), e atualize o inventário")
	}
	// O que ele SERVE, e é verificável: a metade que existe.
	type serve interface {
		ListSubscribed(ctx context.Context, txtID string) ([]domain.NewsletterMetadata, error)
		CreateNewsletter(ctx context.Context, txtID, name, description string, picture []byte) (*domain.NewsletterMetadata, error)
	}
	if _, ok := r.(serve); !ok {
		t.Fatal("deixou de servir ListSubscribed/CreateNewsletter")
	}
}

// TestListaVaziaEFatiaVaziaENaoNil: seguir nada é resposta legítima, e devolvê-la
// como nil obrigaria cada chamador a distinguir dois casos que são o mesmo.
func TestListaVaziaEFatiaVaziaENaoNil(t *testing.T) {
	got, err := comFollower(followerDuplo{entries: nil}).
		ListSubscribed(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListSubscribed: %v", err)
	}
	if got == nil {
		t.Fatal("lista vazia veio como nil")
	}
	if len(got) != 0 {
		t.Fatalf("len=%d", len(got))
	}
}

// TestOAdaptadorNaoFILTRAOQueNaoSabeJulgar é a decisão desta fatia, travada.
//
// A H139 mediu que Followed reflete o CACHE do cliente: seis canais apagados por
// outra conta continuavam no modelo local. A tentação é filtrá-los aqui — mas o
// DirectoryEntry não carrega vivacidade, então filtrar seria inventar um
// julgamento a partir de dado que não existe.
//
// Devolver o cache COMO cache é honesto; devolver uma lista filtrada reivindicaria
// um frescor que este transporte não entrega.
func TestOAdaptadorNaoFILTRAOQueNaoSabeJulgar(t *testing.T) {
	entradas := []waheadless.ChannelEntry{
		{JID: "1@newsletter", Membership: "owner"},
		{JID: "2@newsletter", Membership: "guest"},
		// Uma entrada sem membership: a página não disse, e não cabe a nós
		// decidir que ela não conta.
		{JID: "3@newsletter"},
	}
	got, err := comFollower(followerDuplo{entries: entradas}).
		ListSubscribed(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListSubscribed: %v", err)
	}
	if n := len(got); n != len(entradas) {
		t.Fatalf("devolveu %d de %d entradas: o adaptador filtrou por um "+
			"critério que o DirectoryEntry não carrega", n, len(entradas))
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	_, err := comFollower(followerDuplo{err: errors.New("a página recusou")}).
		ListSubscribed(context.Background(), "s1")
	if err == nil {
		t.Fatal("a falha virou lista vazia — 'não segue nada' e 'não conseguimos ler' " +
			"são coisas diferentes")
	}
}

// TestNormalizacaoDaEntradaDaPagina trava o mapeamento que a migração para DTO
// introduziu: até ele existir, este adaptador ENTREGAVA channel.DirectoryEntry
// ao codificador JSON, e como esse tipo não tem etiqueta `json` nenhuma, uma
// sessão headless servia `{"JID":…,"Subscribers":…,"Membership":…}` na mesma
// rota em que uma sessão wa-noise servia `{"id":…,"thread_metadata":{…}}`.
//
// O que ele afirma, campo a campo, é que o dado da PÁGINA chega ao domínio; o
// nome no fio é afirmado pelo teste de contrato da rota.
func TestNormalizacaoDaEntradaDaPagina(t *testing.T) {
	criado := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	got, err := comFollower(followerDuplo{entries: []waheadless.ChannelEntry{{
		JID:         "1@newsletter",
		Name:        "Avisos",
		Description: "Horários",
		Subscribers: 7,
		Verified:    true,
		Membership:  "guest",
		CreatedAt:   criado,
	}}}).ListSubscribed(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListSubscribed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	m := got[0]
	if m.JID != "1@newsletter" || m.Name.Text != "Avisos" || m.Description.Text != "Horários" {
		t.Errorf("identidade/textos = %+v", m)
	}
	if m.SubscriberCount != 7 || !m.CreatedAt.Equal(criado) {
		t.Errorf("contagem/instante = %d, %v", m.SubscriberCount, m.CreatedAt)
	}
	if m.VerificationState != "verified" {
		t.Errorf("verification_state = %q, quero \"verified\"", m.VerificationState)
	}
	// Membership viaja VERBATIM para Role: a palavra é da página, e traduzi-la
	// para o vocabulário do protocolo reivindicaria um mapeamento que ninguém
	// mediu.
	if m.Viewer == nil || m.Viewer.Role != "guest" {
		t.Errorf("viewer = %+v, quero role=guest", m.Viewer)
	}
	// O que a página NÃO diz fica zero em vez de inventado: um canal que ela
	// nunca descreveu não é "active".
	if m.State != "" || m.InviteCode != "" || m.Picture != nil || m.Preview != nil {
		t.Errorf("campos que a página não reporta vieram preenchidos: %+v", m)
	}
}

// TestSemMembershipNaoInventaRelacao: a página que não disse nada sobre a
// relação não faz nascer um `viewer` com papel vazio, que um cliente leria
// como "há relação, papel desconhecido".
func TestSemMembershipNaoInventaRelacao(t *testing.T) {
	got, err := comFollower(followerDuplo{entries: []waheadless.ChannelEntry{
		{JID: "3@newsletter"},
	}}).ListSubscribed(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListSubscribed: %v", err)
	}
	if got[0].Viewer != nil {
		t.Errorf("viewer = %+v, quero nil", got[0].Viewer)
	}
}

// TestNaoVerificadoNaoEDesconhecido regista a decisão AMBÍGUA desta fatia.
//
// A página responde um booleano; o protocolo tem TRÊS estados (`verified`,
// `unverified`, `""` em canal apagado). O falso da página vira `unverified` e
// não `""`, porque deixar vazio tornaria "não verificado" impossível de
// reportar neste motor — mas isso significa que este motor nunca diz
// "desconhecido".
func TestNaoVerificadoNaoEDesconhecido(t *testing.T) {
	got, err := comFollower(followerDuplo{entries: []waheadless.ChannelEntry{
		{JID: "4@newsletter", Verified: false},
	}}).ListSubscribed(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListSubscribed: %v", err)
	}
	if got[0].VerificationState != "unverified" {
		t.Errorf("verification_state = %q, quero \"unverified\"", got[0].VerificationState)
	}
}

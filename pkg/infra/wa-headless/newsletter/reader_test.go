package newsletter

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

func TestSatisfazOPortDeNewsletter(t *testing.T) {
	var r any = NewReader(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := r.(appport.NewsletterReader); !ok {
		t.Fatal("não satisfaz NewsletterReader")
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
	entradas, ok := got.([]waheadless.ChannelEntry)
	if !ok {
		t.Fatalf("tipo inesperado: %T", got)
	}
	if entradas == nil {
		t.Fatal("lista vazia veio como nil")
	}
	if len(entradas) != 0 {
		t.Fatalf("len=%d", len(entradas))
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
	if n := len(got.([]waheadless.ChannelEntry)); n != len(entradas) {
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

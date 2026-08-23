package avatar

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

func TestSatisfazOPortDeAvatar(t *testing.T) {
	var r any = NewReader(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := r.(appport.AvatarReader); !ok {
		t.Fatal("não satisfaz AvatarReader")
	}
	if _, ok := r.(appport.ContactDirectory); ok {
		t.Fatal("satisfaz ContactDirectory inteira; a decisão 82 separou de propósito")
	}
}

func TestIdentidadeInvalidaNaoGastaSlot(t *testing.T) {
	reg := registry.New(1)
	r := NewReader(adapter.NewSessions(reg, cfgFor))

	if _, err := r.GetProfilePicture(context.Background(), "s1", domain.JID(""), false); err == nil {
		t.Fatal("JID vazio foi aceito")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot", reg.Len())
	}
}

// fetcherDuplo é o dublê da capability. Ele imita a REGRA da produção: Present
// é o campo que decide, e uma foto ausente vem com URLs vazias — não é o dublê
// que inventa uma convenção mais simpática (armadilha nº 1 do ARMADILHAS.md).
type fetcherDuplo struct {
	pic waheadless.AvatarPicture
	err error
}

func (f fetcherDuplo) Fetch(context.Context, string, string) (waheadless.AvatarPicture, error) {
	return f.pic, f.err
}

func comFetcher(f fetcher) *Reader {
	r := NewReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.newFetcher = func(context.Context, string) (fetcher, error) { return f, nil }
	return r
}

// TestFotoAusenteDevolveNilSemErro trava a regra que a F83 custou caro:
// um avatar_url vazio por FALHA era indistinguível de um vazio por NÃO HAVER
// foto. Aqui a distinção é estrutural — ausência é nil sem erro, falha é erro.
func TestFotoAusenteDevolveNilSemErro(t *testing.T) {
	r := comFetcher(fetcherDuplo{pic: waheadless.AvatarPicture{Present: false}})

	got, err := r.GetProfilePicture(context.Background(), "s1", domain.JID("5511999999999@c.us"), false)
	if err != nil {
		t.Fatalf("ausência de foto virou erro: %v", err)
	}
	if got != nil {
		t.Fatalf("ausência de foto devolveu %+v, quero nil", got)
	}
}

// E a falha continua a ser falha, e não uma ausência silenciosa.
func TestFalhaNaoViraAusencia(t *testing.T) {
	r := comFetcher(fetcherDuplo{err: errors.New("a página não respondeu")})

	got, err := r.GetProfilePicture(context.Background(), "s1", domain.JID("5511999999999@c.us"), false)
	if err == nil {
		t.Fatal("uma falha virou 'esta pessoa não tem foto'")
	}
	if got != nil {
		t.Fatalf("falha devolveu %+v além do erro", got)
	}
}

// preview escolhe a miniatura, e NÃO a foto inteira. Trocar as duas é o tipo de
// bug que passa despercebido porque as duas são URLs válidas.
func TestPreviewEscolheAMiniatura(t *testing.T) {
	pic := waheadless.AvatarPicture{
		Present: true, URL: "https://exemplo/cheia.jpg",
		PreviewURL: "https://exemplo/mini.jpg", Tag: "etiqueta-1",
	}

	cheia, err := comFetcher(fetcherDuplo{pic: pic}).
		GetProfilePicture(context.Background(), "s1", domain.JID("5511999999999@c.us"), false)
	if err != nil || cheia == nil {
		t.Fatalf("cheia: %v %+v", err, cheia)
	}
	if cheia.URL != pic.URL {
		t.Fatalf("preview=false devolveu %q, quero a foto inteira", cheia.URL)
	}

	mini, err := comFetcher(fetcherDuplo{pic: pic}).
		GetProfilePicture(context.Background(), "s1", domain.JID("5511999999999@c.us"), true)
	if err != nil || mini == nil {
		t.Fatalf("mini: %v %+v", err, mini)
	}
	if mini.URL != pic.PreviewURL {
		t.Fatalf("preview=true devolveu %q, quero a miniatura", mini.URL)
	}
	// O ID é a Tag, que é o que muda quando a foto muda — é o que permite a um
	// chamador pular um download que já tem.
	if mini.ID != pic.Tag {
		t.Fatalf("ID=%q, quero a Tag %q", mini.ID, pic.Tag)
	}
}

// A falha ao resolver a CONFIGURAÇÃO da sessão propaga com a causa nomeada.
// É um caminho de erro real — um txtID sem perfil configurado — e engoli-lo
// faria o chamador ver "sem foto" quando o problema é que a sessão nem existe.
func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	r := NewReader(adapter.NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	}))
	_, err := r.GetProfilePicture(context.Background(), "s1", domain.JID("5511999999999@c.us"), false)
	if err == nil {
		t.Fatal("falha de configuração virou 'esta pessoa não tem foto'")
	}
	if !strings.Contains(err.Error(), "config for session") {
		t.Fatalf("erro não nomeia a causa: %v", err)
	}
}

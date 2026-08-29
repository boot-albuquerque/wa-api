package profile

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
	"wa-api/pkg/infra/headless/registry"
)

func cfgFor(string) (headless.StartConfig, error) {
	return headless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

func TestSatisfazOPortDeAcessoAoPerfil(t *testing.T) {
	var p any = NewProvider(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := p.(appport.ProfileAccessProvider); !ok {
		t.Fatal("não satisfaz ProfileAccessProvider")
	}
}

// TestOwnJIDPrefereOLIDEDizSeConheceAlgum trava a escolha, que não é arbitrária:
// este build arquiva sob LID — 397 de 399 mensagens na conta de referência — e
// devolver o telefone quando existe LID daria uma identidade que a página não
// usa para indexar nada.
func TestOwnJIDPrefereOLIDEDizSeConheceAlgum(t *testing.T) {
	comOsDois := &snapshot{identity: headless.OwnIdentity{
		LID: headless.OwnWID{Serialized: "111@lid", User: "111"},
		PN:  headless.OwnWID{Serialized: "5511999999999@c.us", User: "5511999999999"},
	}}
	got, ok := comOsDois.OwnJID()
	if !ok || string(got) != "111@lid" {
		t.Fatalf("com os dois devolveu %q (ok=%v), quero o LID", got, ok)
	}

	soTelefone := &snapshot{identity: headless.OwnIdentity{
		PN: headless.OwnWID{Serialized: "5511999999999@c.us", User: "5511999999999"},
	}}
	got, ok = soTelefone.OwnJID()
	if !ok || string(got) != "5511999999999@c.us" {
		t.Fatalf("só com telefone devolveu %q (ok=%v)", got, ok)
	}

	// Nenhum materializado: o segundo retorno existe para o chamador NÃO ter de
	// comparar com string vazia.
	vazio := &snapshot{}
	if got, ok := vazio.OwnJID(); ok || got != "" {
		t.Fatalf("sem identidade devolveu %q (ok=%v), quero vazio e false", got, ok)
	}
}

// TestPushNameVazioEMedidoENaoEsquecido: o getter de display name EXISTE neste
// build e devolve null contra o perfil pareado real. Vazio é o zero-value que o
// port declara como "ainda não se sabe", e não um campo por preencher.
func TestPushNameVazioEMedidoENaoEsquecido(t *testing.T) {
	s := &snapshot{identity: headless.OwnIdentity{
		LID: headless.OwnWID{Serialized: "111@lid"},
	}}
	if s.PushName() != "" {
		t.Fatalf("PushName=%q: se a página passou a expor o display name, isso é "+
			"mudança de capacidade e precisa de medição, não de um valor aqui",
			s.PushName())
	}
}

// TestDeviceInfoEZeroValueDeliberado: o socket preenche isto de um registro de
// pareamento local que um driver de página não tem — a página segura uma sessão,
// não um registro de dispositivos. Valores inventados seriam PIORES que vazios:
// isto alimenta superfície de diagnóstico, e uma plataforma plausível e errada é
// mais difícil de desconfiar que uma em branco.
func TestDeviceInfoEZeroValueDeliberado(t *testing.T) {
	var vazio domain.SessionDeviceInfo
	if (&snapshot{}).DeviceInfo() != vazio {
		t.Fatal("DeviceInfo devolveu algo: este transporte não tem registro de " +
			"dispositivos, e inventar alimentaria diagnóstico com dado falso")
	}
}

// A identidade inválida é recusada antes de qualquer trabalho de página.
func TestIdentidadeInvalidaERecusada(t *testing.T) {
	s := &snapshot{}
	if _, _, err := s.ProfilePictureURL(context.Background(), domain.JID("status@broadcast")); err == nil {
		t.Fatal("um broadcast foi aceito como identidade de pessoa")
	}
	if _, _, err := s.ContactInfo(context.Background(), domain.JID("")); err == nil {
		t.Fatal("JID vazio foi aceito")
	}
}

type fetcherDuplo struct {
	pic headless.AvatarPicture
	err error
}

func (f fetcherDuplo) Fetch(context.Context, string, string) (headless.AvatarPicture, error) {
	return f.pic, f.err
}

type listerDuplo struct {
	roster headless.ContactRoster
	err    error
}

func (l listerDuplo) List(context.Context, string) (headless.ContactRoster, error) {
	return l.roster, l.err
}

func comCapabilities(f fetcher, l lister) *snapshot {
	s := &snapshot{}
	if f != nil {
		s.newFetcher = func() fetcher { return f }
	}
	if l != nil {
		s.newLister = func() lister { return l }
	}
	return s
}

// Ausência de foto é PAR VAZIO sem erro; falha de leitura é ERRO. É a distinção
// da F83: um avatar_url vazio por falha era indistinguível de um vazio por não
// haver foto, e o par de retornos separados existe para isso.
func TestFotoAusenteContraFalhaDeLeitura(t *testing.T) {
	url, tag, err := comCapabilities(fetcherDuplo{pic: headless.AvatarPicture{Present: false}}, nil).
		ProfilePictureURL(context.Background(), domain.JID("5511999999999@c.us"))
	if err != nil {
		t.Fatalf("ausência de foto virou erro: %v", err)
	}
	if url != "" || tag != "" {
		t.Fatalf("ausência devolveu url=%q tag=%q", url, tag)
	}

	_, _, err = comCapabilities(fetcherDuplo{err: errors.New("a página não respondeu")}, nil).
		ProfilePictureURL(context.Background(), domain.JID("5511999999999@c.us"))
	if err == nil {
		t.Fatal("uma falha de leitura virou 'esta pessoa não tem foto'")
	}
}

// TestContatoForaDoRosterERespostaENaoFalha: não conhecer alguém é uma resposta.
// Devolver erro faria o chamador tratar "não está na agenda" como avaria.
func TestContatoForaDoRosterERespostaENaoFalha(t *testing.T) {
	roster := headless.ContactRoster{Contacts: []headless.RosterContact{
		{LID: "111@lid", Pushname: "Ana"},
	}}

	nome, negocio, err := comCapabilities(nil, listerDuplo{roster: roster}).
		ContactInfo(context.Background(), domain.JID("999@lid"))
	if err != nil {
		t.Fatalf("contato desconhecido virou erro: %v", err)
	}
	if nome != "" || negocio != "" {
		t.Fatalf("contato desconhecido devolveu %q/%q", nome, negocio)
	}

	// E quem ESTÁ no roster é encontrado, senão o teste acima passaria mesmo
	// com a busca quebrada.
	nome, _, err = comCapabilities(nil, listerDuplo{roster: roster}).
		ContactInfo(context.Background(), domain.JID("111@lid"))
	if err != nil || nome != "Ana" {
		t.Fatalf("contato conhecido devolveu %q (err=%v)", nome, err)
	}
}

// A falha ao LER o roster é erro, e não "não conhecemos ninguém".
func TestFalhaDoRosterNaoViraDesconhecido(t *testing.T) {
	if _, _, err := comCapabilities(nil, listerDuplo{err: errors.New("a página recusou")}).
		ContactInfo(context.Background(), domain.JID("111@lid")); err == nil {
		t.Fatal("a falha de leitura virou contato desconhecido")
	}
}

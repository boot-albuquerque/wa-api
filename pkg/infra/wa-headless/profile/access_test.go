package profile

import (
	"context"
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
	comOsDois := &snapshot{identity: waheadless.OwnIdentity{
		LID: waheadless.OwnWID{Serialized: "111@lid", User: "111"},
		PN:  waheadless.OwnWID{Serialized: "5511999999999@c.us", User: "5511999999999"},
	}}
	got, ok := comOsDois.OwnJID()
	if !ok || string(got) != "111@lid" {
		t.Fatalf("com os dois devolveu %q (ok=%v), quero o LID", got, ok)
	}

	soTelefone := &snapshot{identity: waheadless.OwnIdentity{
		PN: waheadless.OwnWID{Serialized: "5511999999999@c.us", User: "5511999999999"},
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
	s := &snapshot{identity: waheadless.OwnIdentity{
		LID: waheadless.OwnWID{Serialized: "111@lid"},
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

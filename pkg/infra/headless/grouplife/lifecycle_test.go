package grouplife

import (
	"context"
	"errors"
	"strings"
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

type duplo struct {
	criado headless.GroupCreated
	entrou headless.GroupJoined
	jids   []string
	saiuDe string
	err    error
}

func (d *duplo) Ensure(_ context.Context, _ string, participants []string, _ string) (headless.GroupCreated, error) {
	d.jids = participants
	return d.criado, d.err
}
func (d *duplo) JoinByInvite(context.Context, string, string) (headless.GroupJoined, error) {
	return d.entrou, d.err
}
func (d *duplo) Leave(_ context.Context, groupJID, _ string) error {
	d.saiuDe = groupJID
	return d.err
}

func com(d lifecycle) *Manager {
	m := NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	m.newLifecycle = func(context.Context, string) (lifecycle, error) { return d, nil }
	return m
}

func TestSatisfazOPortDeCicloDeVida(t *testing.T) {
	var m any = NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := m.(appport.GroupLifecycle); !ok {
		t.Fatal("não satisfaz GroupLifecycle")
	}
}

// TestEntradaPENDENTENaoViraAdesao é a regra que não pode ser achatada: um grupo
// com aprovação transforma a entrada numa SOLICITAÇÃO, e reportá-la como adesão
// faria o chamador anunciar uma entrada que não aconteceu.
func TestEntradaPENDENTENaoViraAdesao(t *testing.T) {
	d := &duplo{entrou: headless.GroupJoined{Pending: true}}

	got, err := com(d).JoinGroup(context.Background(), "s1", "AbCdEf")
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}
	j, ok := got.(headless.GroupJoined)
	if !ok {
		t.Fatalf("tipo inesperado: %T", got)
	}
	if !j.Pending {
		t.Fatal("o Pending sumiu no caminho: o chamador anunciaria uma entrada " +
			"que é, na verdade, uma solicitação por aprovar")
	}
	if j.GroupJID != "" {
		t.Fatalf("uma entrada pendente devolveu jid de grupo %q", j.GroupJID)
	}
}

// TestCreatedDistingueCriarDeEncontrar: a capability chama-se Ensure, e duas
// chamadas iguais devolvem o MESMO grupo. Perder esse campo faria parecer que
// dois grupos foram criados quando foi um.
func TestCreatedDistingueCriarDeEncontrar(t *testing.T) {
	got, err := com(&duplo{criado: headless.GroupCreated{JID: "120363@g.us", Created: false}}).
		CreateGroup(context.Background(), "s1", "Laboratório", nil, domain.CreateGroupOpts{})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if got.Created {
		t.Fatal("um grupo ENCONTRADO foi reportado como criado")
	}
	// E o grupo em si atravessou o mapeamento: sem isto, um adaptador que
	// devolvesse Created certo e grupo nulo passaria.
	if got.Group == nil || got.Group.JID != "120363@g.us" {
		t.Fatalf("o grupo nao atravessou o mapeamento: %#v", got.Group)
	}
}

// Os participantes chegam CONVERTIDOS à capability, na grafia da página.
func TestParticipantesChegamConvertidos(t *testing.T) {
	d := &duplo{}
	if _, err := com(d).CreateGroup(context.Background(), "s1", "Equipa",
		[]domain.JID{"5511999999999@s.whatsapp.net"}, domain.CreateGroupOpts{}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if len(d.jids) != 1 || strings.HasSuffix(d.jids[0], "@s.whatsapp.net") {
		t.Fatalf("a capability recebeu %v, na grafia do SOCKET", d.jids)
	}
}

// Entradas inválidas são recusadas antes de tocar na página.
func TestEntradasInvalidasSaoRecusadas(t *testing.T) {
	d := &duplo{}
	if _, err := com(d).CreateGroup(context.Background(), "s1", "", nil, domain.CreateGroupOpts{}); err == nil {
		t.Fatal("nome vazio foi aceito")
	}
	if _, err := com(d).JoinGroup(context.Background(), "s1", ""); err == nil {
		t.Fatal("código vazio foi aceito")
	}
	if err := com(d).LeaveGroup(context.Background(), "s1", domain.JID("")); err == nil {
		t.Fatal("jid vazio foi aceito")
	}
}

// Os três caminhos de erro que todo adaptador desta fase precisa de ter, e que
// as quatro rodadas anteriores mostraram serem sempre os mesmos.
func TestOsTresCaminhosDeErro(t *testing.T) {
	// 1. posse sem boot
	reg := registry.New(2)
	m := NewManager(adapter.NewSessions(reg, cfgFor))
	if err := m.EnsureSession(context.Background(), "desconhecida"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("EnsureSession: got %v, want ErrUnknownSession", err)
	}

	// 2. identidade inválida não gasta slot
	if err := m.LeaveGroup(context.Background(), "s1", domain.JID("status@broadcast")); err == nil {
		t.Fatal("um broadcast foi aceito como grupo")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot", reg.Len())
	}

	// 3. falha de configuração propaga
	semPerfil := NewManager(adapter.NewSessions(registry.New(1), func(string) (headless.StartConfig, error) {
		return headless.StartConfig{}, errors.New("sem perfil")
	}))
	if _, err := semPerfil.JoinGroup(context.Background(), "s1", "AbCdEf"); err == nil {
		t.Fatal("falha de configuração virou entrada bem-sucedida")
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	if err := com(&duplo{err: errors.New("a página recusou")}).
		LeaveGroup(context.Background(), "s1", domain.JID("120363@g.us")); err == nil {
		t.Fatal("a falha virou saída bem-sucedida")
	}
}

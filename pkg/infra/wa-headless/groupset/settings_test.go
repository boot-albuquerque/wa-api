package groupset

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

const grupo = domain.JID("120363000000000000@g.us")

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

type duplo struct {
	rename     waheadless.GroupRename
	descrito   waheadless.GroupDescribed
	mudanca    waheadless.GroupPolicyChange
	vistoGrupo string
	vistoTexto string
	vistaPol   waheadless.GroupPolicy
	vistoOn    bool
	err        error
}

func (d *duplo) SetSubject(_ context.Context, groupJID, subject, _ string) (waheadless.GroupRename, error) {
	d.vistoGrupo, d.vistoTexto = groupJID, subject
	return d.rename, d.err
}

func (d *duplo) SetDescription(_ context.Context, groupJID, description, _ string) (waheadless.GroupDescribed, error) {
	d.vistoGrupo, d.vistoTexto = groupJID, description
	return d.descrito, d.err
}

func (d *duplo) SetPolicy(_ context.Context, groupJID string, p waheadless.GroupPolicy, on bool, _ string) (waheadless.GroupPolicyChange, error) {
	d.vistoGrupo, d.vistaPol, d.vistoOn = groupJID, p, on
	return d.mudanca, d.err
}

func com(d settings) *Manager {
	m := NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	m.newSettings = func(context.Context, string) (settings, error) { return d, nil }
	return m
}

func TestSatisfazOPortEstreito(t *testing.T) {
	var m any = NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := m.(appport.GroupInfoSettings); !ok {
		t.Fatal("não satisfaz GroupInfoSettings")
	}
}

// --- as TRÊS confirmações, cada uma no seu vocabulário ---
//
// O que estes testes travam não é o mapeamento: é que aplicar a mesma regra às
// três seria errado, porque a página confirma cada uma de um jeito diferente e
// isso foi MEDIDO.

// Rename não tem booleano: tem o NOME do campo do modelo que carregou a
// mudança. Campo vazio é a capability a dizer que não soube onde olhar.
func TestNomeSemCampoDoModeloNaoPassaPorFeito(t *testing.T) {
	d := &duplo{rename: waheadless.GroupRename{Field: "", AlreadyInState: false}}
	err := com(d).SetGroupName(context.Background(), "s1", grupo, "equipe")
	if err == nil {
		t.Fatal("um nome sem campo do modelo que o confirme passou por feito")
	}
	if strings.Contains(err.Error(), "equipe") {
		t.Fatalf("o erro vazou o nome do grupo: %v", err)
	}
}

func TestNomeJaNoEstadoNaoPrecisaDeCampo(t *testing.T) {
	d := &duplo{rename: waheadless.GroupRename{AlreadyInState: true}}
	if err := com(d).SetGroupName(context.Background(), "s1", grupo, "equipe"); err != nil {
		t.Fatalf("um no-op virou erro: %v", err)
	}
}

// Described traz o texto RELIDO do servidor: aqui a pós-condição é comparável,
// e é a confirmação mais forte das três.
func TestDescricaoRELIDADiferenteNaoPassaPorFeita(t *testing.T) {
	d := &duplo{descrito: waheadless.GroupDescribed{Text: "outra coisa", Source: "desc"}}
	err := com(d).SetGroupTopic(context.Background(), "s1", grupo, "combinado da equipe")
	if err == nil {
		t.Fatal("uma descrição relida DIFERENTE passou por feita")
	}
	if strings.Contains(err.Error(), "combinado") || strings.Contains(err.Error(), "outra coisa") {
		t.Fatalf("o erro vazou conteúdo de descrição: %v", err)
	}
}

// O servidor normaliza espaço, e recusar por causa disso seria falso negativo.
func TestDescricaoSoDIFERENTENOESPACOEAceita(t *testing.T) {
	d := &duplo{descrito: waheadless.GroupDescribed{Text: "  combinado  ", Source: "desc"}}
	if err := com(d).SetGroupTopic(context.Background(), "s1", grupo, "combinado"); err != nil {
		t.Fatalf("diferença só de espaço virou recusa: %v", err)
	}
}

// PolicyChange tem Verified, e ele é verdadeiro para mudança REAL — ao
// contrário de Membership.Verified, e a diferença é medida (H85).
func TestPoliticaNaoVERIFICADANaoPassaPorFeita(t *testing.T) {
	d := &duplo{mudanca: waheadless.GroupPolicyChange{Wanted: true, Verified: false, NoOp: false}}
	if err := com(d).SetGroupAnnounce(context.Background(), "s1", grupo, true); err == nil {
		t.Fatal("uma política sem confirmação lida de volta passou por feita")
	}
}

// --- as duas políticas não são a mesma, e trocá-las mudaria outra coisa ---

func TestAnnounceELockedSaoPoliticasDIFERENTES(t *testing.T) {
	verificada := waheadless.GroupPolicyChange{Verified: true}

	d := &duplo{mudanca: verificada}
	if err := com(d).SetGroupAnnounce(context.Background(), "s1", grupo, true); err != nil {
		t.Fatalf("announce: %v", err)
	}
	if d.vistaPol != waheadless.PolicyMessagesAdminsOnly {
		t.Fatalf("SetGroupAnnounce mandou %q; announce é quem pode ENVIAR", d.vistaPol)
	}

	d2 := &duplo{mudanca: verificada}
	if err := com(d2).SetGroupLocked(context.Background(), "s1", grupo, true); err != nil {
		t.Fatalf("locked: %v", err)
	}
	if d2.vistaPol != waheadless.PolicyInfoAdminsOnly {
		t.Fatalf("SetGroupLocked mandou %q; locked é quem pode EDITAR a info", d2.vistaPol)
	}
	if d.vistaPol == d2.vistaPol {
		t.Fatal("as duas mandaram a MESMA política — uma delas muda a coisa errada")
	}
}

func TestDesligarChegaComoDesligar(t *testing.T) {
	d := &duplo{mudanca: waheadless.GroupPolicyChange{Verified: true}}
	if err := com(d).SetGroupLocked(context.Background(), "s1", grupo, false); err != nil {
		t.Fatalf("locked=false: %v", err)
	}
	if d.vistoOn {
		t.Fatal("pediu destrancar e a capability recebeu trancar")
	}
}

func TestGrafiaDaPaginaChegaACapability(t *testing.T) {
	d := &duplo{rename: waheadless.GroupRename{Field: "subject"}}
	if err := com(d).SetGroupName(context.Background(), "s1", grupo, "equipe"); err != nil {
		t.Fatalf("SetGroupName: %v", err)
	}
	if strings.Contains(d.vistoGrupo, "@s.whatsapp.net") {
		t.Fatalf("a capability recebeu %q, na grafia do SOCKET", d.vistoGrupo)
	}
}

func TestOsTresCaminhosDeErro(t *testing.T) {
	// 1. posse sem boot
	reg := registry.New(2)
	m := NewManager(adapter.NewSessions(reg, cfgFor))
	if err := m.EnsureSession(context.Background(), "desconhecida"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("EnsureSession: got %v, want ErrUnknownSession", err)
	}

	// 2. identidade inválida não gasta slot
	if err := m.SetGroupName(context.Background(), "s1", domain.JID("status@broadcast"), "x"); err == nil {
		t.Fatal("um broadcast foi aceito como grupo")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot", reg.Len())
	}

	// 3. falha de configuração propaga
	semPerfil := NewManager(adapter.NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil")
	}))
	if err := semPerfil.SetGroupTopic(context.Background(), "s1", grupo, "x"); err == nil {
		t.Fatal("falha de configuração virou descrição bem-sucedida")
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	ctx := context.Background()
	falha := func() *Manager { return com(&duplo{err: errors.New("a página recusou")}) }
	if err := falha().SetGroupName(ctx, "s1", grupo, "x"); err == nil {
		t.Fatal("a falha virou renomeação bem-sucedida")
	}
	if err := falha().SetGroupTopic(ctx, "s1", grupo, "x"); err == nil {
		t.Fatal("a falha virou descrição bem-sucedida")
	}
	if err := falha().SetGroupAnnounce(ctx, "s1", grupo, true); err == nil {
		t.Fatal("a falha virou política bem-sucedida")
	}
}

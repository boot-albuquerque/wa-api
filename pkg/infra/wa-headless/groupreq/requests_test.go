package groupreq

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

// duploPedidos imita a capability: um resultado POR SOLICITANTE, porque é o que
// a groupreq.Manager faz — um RPC por participante (groupreq.go:198).
type duploPedidos struct {
	lista       waheadless.GroupRequestList
	resultados  []waheadless.GroupRequestAction
	vistoGrupo  string
	vistoQuem   []string
	vistoRejeic bool
	err         error
}

func (d *duploPedidos) List(_ context.Context, groupJID, _ string) (waheadless.GroupRequestList, error) {
	d.vistoGrupo = groupJID
	return d.lista, d.err
}

func (d *duploPedidos) Approve(_ context.Context, groupJID string, quem []string, _ string) ([]waheadless.GroupRequestAction, error) {
	d.vistoGrupo, d.vistoQuem = groupJID, quem
	return d.resultados, d.err
}

func (d *duploPedidos) Reject(_ context.Context, groupJID string, quem []string, _ string) ([]waheadless.GroupRequestAction, error) {
	d.vistoGrupo, d.vistoQuem, d.vistoRejeic = groupJID, quem, true
	return d.resultados, d.err
}

type duploPolitica struct {
	mudanca    waheadless.GroupPolicyChange
	vistoGrupo string
	vistaPol   waheadless.GroupPolicy
	vistoOn    bool
	err        error
}

func (d *duploPolitica) SetPolicy(_ context.Context, groupJID string, p waheadless.GroupPolicy, on bool, _ string) (waheadless.GroupPolicyChange, error) {
	d.vistoGrupo, d.vistaPol, d.vistoOn = groupJID, p, on
	return d.mudanca, d.err
}

func com(r requests, p policy) *Manager {
	m := NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	if r != nil {
		m.newRequests = func(context.Context, string) (requests, error) { return r, nil }
	}
	if p != nil {
		m.newPolicy = func(context.Context, string) (policy, error) { return p, nil }
	}
	return m
}

func ok(jid string) waheadless.GroupRequestAction {
	return waheadless.GroupRequestAction{RequesterJID: jid, OK: true}
}

func recusado(jid string, code int) waheadless.GroupRequestAction {
	return waheadless.GroupRequestAction{RequesterJID: jid, OK: false, Code: code}
}

func TestSatisfazOPortDePedidos(t *testing.T) {
	var m any = NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := m.(appport.GroupRequests); !ok {
		t.Fatal("não satisfaz GroupRequests")
	}
}

// O desfecho parcial é o coração desta fatia: o port devolve só error, e a
// capability devolve um resultado por solicitante. Silenciar a recusa de um
// deles seria dizer que três aprovações aconteceram quando duas aconteceram.
func TestRecusaParcialNaoPassaPorSucesso(t *testing.T) {
	d := &duploPedidos{resultados: []waheadless.GroupRequestAction{
		ok("a@lid"), recusado("b@lid", 403), ok("c@lid"),
	}}
	_, err := com(d, nil).UpdateRequestParticipants(context.Background(), "s1", grupo,
		[]domain.JID{"a@lid", "b@lid", "c@lid"}, domain.RequestApprove)
	if err == nil {
		t.Fatal("uma recusa da página passou por aprovação bem-sucedida")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("o erro não carrega o código da página: %v", err)
	}
	if strings.Contains(err.Error(), "b@lid") {
		t.Fatalf("o erro vazou a identidade do solicitante: %v", err)
	}
}

// Lista de resultados MAIS CURTA que a de pedidos não é concordância: é
// ausência. Um solicitante sobre o qual a página nunca respondeu não foi
// aprovado.
func TestSolicitanteSemRespostaNaoContaComoAprovado(t *testing.T) {
	d := &duploPedidos{resultados: []waheadless.GroupRequestAction{ok("a@lid")}}
	_, err := com(d, nil).UpdateRequestParticipants(context.Background(), "s1", grupo,
		[]domain.JID{"a@lid", "b@lid"}, domain.RequestApprove)
	if err == nil {
		t.Fatal("um solicitante sem resposta passou por aprovado")
	}
	if !strings.Contains(err.Error(), "1 sem resposta") {
		t.Fatalf("o erro não diz quantos ficaram sem resposta: %v", err)
	}
}

func TestTodosAprovadosNaoEErro(t *testing.T) {
	d := &duploPedidos{resultados: []waheadless.GroupRequestAction{ok("a@lid"), ok("b@lid")}}
	if _, err := com(d, nil).UpdateRequestParticipants(context.Background(), "s1", grupo,
		[]domain.JID{"a@lid", "b@lid"}, domain.RequestApprove); err != nil {
		t.Fatalf("duas aprovações boas viraram erro: %v", err)
	}
}

func TestAprovarERejeitarNaoSaoAMesmaChamada(t *testing.T) {
	d := &duploPedidos{resultados: []waheadless.GroupRequestAction{ok("a@lid")}}
	if _, err := com(d, nil).UpdateRequestParticipants(context.Background(), "s1", grupo,
		[]domain.JID{"a@lid"}, domain.RequestReject); err != nil {
		t.Fatalf("rejeição: %v", err)
	}
	if !d.vistoRejeic {
		t.Fatal("RequestReject chamou Approve — o veredito foi invertido")
	}
}

func TestVereditoDesconhecidoERecusado(t *testing.T) {
	d := &duploPedidos{resultados: []waheadless.GroupRequestAction{ok("a@lid")}}
	if _, err := com(d, nil).UpdateRequestParticipants(context.Background(), "s1", grupo,
		[]domain.JID{"a@lid"}, domain.RequestAction("talvez")); err == nil {
		t.Fatal("um veredito desconhecido foi aceito")
	}
}

// A conversão de grafia: a capability tem de receber o jid da PÁGINA, não o do
// socket. Foi este o controle que mordeu na fatia anterior.
func TestGrafiaDaPaginaChegaAsCapabilities(t *testing.T) {
	d := &duploPedidos{}
	if _, err := com(d, nil).GetRequestParticipants(context.Background(), "s1", grupo); err != nil {
		t.Fatalf("List: %v", err)
	}
	if strings.Contains(d.vistoGrupo, "@s.whatsapp.net") {
		t.Fatalf("a capability recebeu %q, na grafia do SOCKET", d.vistoGrupo)
	}

	p := &duploPolitica{mudanca: waheadless.GroupPolicyChange{Wanted: true, Verified: true}}
	if err := com(nil, p).SetJoinApprovalMode(context.Background(), "s1", grupo, true); err != nil {
		t.Fatalf("SetJoinApprovalMode: %v", err)
	}
	if strings.Contains(p.vistoGrupo, "@s.whatsapp.net") {
		t.Fatalf("a política recebeu %q, na grafia do SOCKET", p.vistoGrupo)
	}
}

// O modo de aprovação é uma POLÍTICA, e a política certa. Trocá-la por
// `restrict` ou `announcement` mudaria outra coisa do grupo em silêncio.
func TestOModoDeAprovacaoEAPoliticaCerta(t *testing.T) {
	p := &duploPolitica{mudanca: waheadless.GroupPolicyChange{Wanted: false, Verified: true}}
	if err := com(nil, p).SetJoinApprovalMode(context.Background(), "s1", grupo, false); err != nil {
		t.Fatalf("SetJoinApprovalMode: %v", err)
	}
	if p.vistaPol != waheadless.PolicyJoinNeedsApproval {
		t.Fatalf("mudou a política %q em vez do modo de aprovação", p.vistaPol)
	}
	if p.vistoOn {
		t.Fatal("pediu desligar e a capability recebeu ligar")
	}
}

// Ao contrário de uma mudança de participantes, esta É verificável na mesma
// sessão (H85). Então uma mudança que a página não leu de volta não pode passar
// por feita — é a invariante 14 no ponto em que ela tem dente.
func TestMudancaNaoVERIFICADANaoPassaPorFeita(t *testing.T) {
	p := &duploPolitica{mudanca: waheadless.GroupPolicyChange{Wanted: true, Verified: false, NoOp: false}}
	if err := com(nil, p).SetJoinApprovalMode(context.Background(), "s1", grupo, true); err == nil {
		t.Fatal("uma mudança sem confirmação lida de volta passou por feita")
	}
}

// O no-op é o caso em que não há valor novo para confirmar, e recusá-lo faria
// pedir o que o grupo já tem virar erro.
func TestNoOpNaoPrecisaDeConfirmacao(t *testing.T) {
	p := &duploPolitica{mudanca: waheadless.GroupPolicyChange{Wanted: true, NoOp: true, Verified: false}}
	if err := com(nil, p).SetJoinApprovalMode(context.Background(), "s1", grupo, true); err != nil {
		t.Fatalf("um no-op virou erro: %v", err)
	}
}

func TestEntradasInvalidasSaoRecusadas(t *testing.T) {
	m := com(&duploPedidos{}, &duploPolitica{})
	ctx := context.Background()
	if _, err := m.GetRequestParticipants(ctx, "s1", domain.JID("status@broadcast")); err == nil {
		t.Fatal("um broadcast foi aceito como grupo")
	}
	if _, err := m.UpdateRequestParticipants(ctx, "s1", grupo, nil, domain.RequestApprove); err == nil {
		t.Fatal("uma lista vazia de solicitantes foi aceita")
	}
	if err := m.SetJoinApprovalMode(ctx, "s1", domain.JID("status@broadcast"), true); err == nil {
		t.Fatal("um broadcast foi aceito como grupo")
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
	if _, err := m.GetRequestParticipants(context.Background(), "s1", domain.JID("status@broadcast")); err == nil {
		t.Fatal("um broadcast foi aceito como grupo")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot", reg.Len())
	}

	// 3. falha de configuração propaga
	semPerfil := NewManager(adapter.NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil")
	}))
	if _, err := semPerfil.GetRequestParticipants(context.Background(), "s1", grupo); err == nil {
		t.Fatal("falha de configuração virou leitura bem-sucedida")
	}
	if err := semPerfil.SetJoinApprovalMode(context.Background(), "s1", grupo, true); err == nil {
		t.Fatal("falha de configuração virou política bem-sucedida")
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	ctx := context.Background()
	if _, err := com(&duploPedidos{err: errors.New("a página recusou")}, nil).
		GetRequestParticipants(ctx, "s1", grupo); err == nil {
		t.Fatal("a falha virou leitura bem-sucedida")
	}
	if _, err := com(&duploPedidos{err: errors.New("a página recusou")}, nil).
		UpdateRequestParticipants(ctx, "s1", grupo, []domain.JID{"a@lid"}, domain.RequestApprove); err == nil {
		t.Fatal("a falha virou aprovação bem-sucedida")
	}
	if err := com(nil, &duploPolitica{err: errors.New("a página recusou")}).
		SetJoinApprovalMode(ctx, "s1", grupo, true); err == nil {
		t.Fatal("a falha virou política bem-sucedida")
	}
}

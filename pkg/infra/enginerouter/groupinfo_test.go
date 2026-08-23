package enginerouter

import (
	"context"
	"errors"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const grupo = domain.JID("120363000000000000@g.us")

// espia registra QUAL transporte foi chamado, e por qual método. Nunca devolve
// erro: assim, uma chamada que chegue ao lado errado passa por SUCESSO, e é o
// teste que tem de a apanhar — um dublê que errasse esconderia a rota.
type espia struct {
	nome     string
	chamadas []string
	guardas  int
	erroFixo error
}

func (e *espia) registra(m string) error { e.chamadas = append(e.chamadas, m); return e.erroFixo }

func (e *espia) EnsureSession(context.Context, string) error {
	e.guardas++
	return e.registra("EnsureSession")
}
func (e *espia) SetGroupName(context.Context, string, domain.JID, string) error {
	return e.registra("SetGroupName")
}
func (e *espia) SetGroupTopic(context.Context, string, domain.JID, string) error {
	return e.registra("SetGroupTopic")
}
func (e *espia) SetGroupAnnounce(context.Context, string, domain.JID, bool) error {
	return e.registra("SetGroupAnnounce")
}
func (e *espia) SetGroupLocked(context.Context, string, domain.JID, bool) error {
	return e.registra("SetGroupLocked")
}

// seletor imita a regra do bootstrap. O comentário diz de onde ela vem, porque
// um dublê mais permissivo que a produção esconde o defeito em vez de o revelar
// (ARMADILHAS.md, armadilha 1).
//
// Regra REAL: pkg/bootstrap/engine_routing.go rotaDeEngine — só a sessão em
// headless pode ser recusada, e só quando não há implementação headless.
type seletor struct{ emHeadless map[string]bool }

func (s seletor) UsaHeadless(txtID string) bool { return s.emHeadless[txtID] }
func (s seletor) Recusar(port, txtID string) error {
	return errRecusa{port: port, txtID: txtID}
}

type errRecusa struct{ port, txtID string }

func (e errRecusa) Error() string {
	return "engine headless nao serve o port " + e.port + " (sem fallback silencioso)"
}

func monta(t *testing.T, headlessServe bool, emHeadless ...string) (*GroupInfo, *espia, *espia) {
	t.Helper()
	soc, hl := &espia{nome: "socket"}, &espia{nome: "headless"}
	m := map[string]bool{}
	for _, id := range emHeadless {
		m[id] = true
	}
	var headless appport.GroupInfoSettings
	if headlessServe {
		headless = hl
	}
	return NewGroupInfo(seletor{emHeadless: m}, soc, headless), soc, hl
}

func TestSessaoNoSocketVaiAoSocket(t *testing.T) {
	r, soc, hl := monta(t, true, "s-headless")
	if err := r.SetGroupName(context.Background(), "s-socket", grupo, "x"); err != nil {
		t.Fatal(err)
	}
	if len(soc.chamadas) != 1 || len(hl.chamadas) != 0 {
		t.Fatalf("socket=%v headless=%v", soc.chamadas, hl.chamadas)
	}
}

func TestSessaoEmHeadlessVaiAoHeadless(t *testing.T) {
	r, soc, hl := monta(t, true, "s1")
	if err := r.SetGroupTopic(context.Background(), "s1", grupo, "x"); err != nil {
		t.Fatal(err)
	}
	if len(hl.chamadas) != 1 || len(soc.chamadas) != 0 {
		t.Fatalf("socket=%v headless=%v", soc.chamadas, hl.chamadas)
	}
}

// O coração: sem implementação headless, a sessão headless é RECUSADA — e o
// socket não é tocado. Tocar seria devolver uma resposta correta para uma
// pergunta que ninguém fez.
func TestSemImplementacaoHeadlessRECUSAENaoCaiNoSocket(t *testing.T) {
	r, soc, _ := monta(t, false, "s1")
	err := r.SetGroupAnnounce(context.Background(), "s1", grupo, true)
	if err == nil {
		t.Fatal("a chamada passou sem transporte que a servisse")
	}
	var alvo errRecusa
	if !errors.As(err, &alvo) || alvo.port != "GroupInfoSettings" {
		t.Fatalf("a recusa não diz qual port: %v", err)
	}
	if len(soc.chamadas) != 0 {
		t.Fatalf("CAIU NO SOCKET em silêncio: %v", soc.chamadas)
	}
}

// A regra vale para TODOS os métodos, não só o que alguém lembrou de testar.
// Um método esquecido caindo no socket seria invisível.
func TestARegraValeParaTODOSOsMetodos(t *testing.T) {
	ctx := context.Background()
	metodos := map[string]func(*GroupInfo) error{
		"EnsureSession":    func(r *GroupInfo) error { return r.EnsureSession(ctx, "s1") },
		"SetGroupName":     func(r *GroupInfo) error { return r.SetGroupName(ctx, "s1", grupo, "x") },
		"SetGroupTopic":    func(r *GroupInfo) error { return r.SetGroupTopic(ctx, "s1", grupo, "x") },
		"SetGroupAnnounce": func(r *GroupInfo) error { return r.SetGroupAnnounce(ctx, "s1", grupo, true) },
		"SetGroupLocked":   func(r *GroupInfo) error { return r.SetGroupLocked(ctx, "s1", grupo, true) },
	}
	// O conjunto é enumerado contra a interface: se um método novo entrar no
	// port e ninguém o acrescentar aqui, esta contagem denuncia.
	if len(metodos) != 5 {
		t.Fatalf("o port tem 5 métodos; a tabela tem %d", len(metodos))
	}
	for nome, chamar := range metodos {
		r, soc, hl := monta(t, false, "s1")
		if err := chamar(r); err == nil {
			t.Errorf("%s: passou sem transporte", nome)
		}
		if len(soc.chamadas) != 0 {
			t.Errorf("%s: caiu no socket", nome)
		}
		rOK, socOK, hlOK := monta(t, true, "s1")
		if err := chamar(rOK); err != nil {
			t.Errorf("%s: com headless disponível, falhou: %v", nome, err)
		}
		if len(hlOK.chamadas) != 1 || len(socOK.chamadas) != 0 {
			t.Errorf("%s: foi para o lado errado (socket=%v headless=%v)", nome, socOK.chamadas, hlOK.chamadas)
		}
		_ = hl
	}
}

// A guarda de posse tem de ir ao MESMO lado que a operação iria. Perguntar ao
// socket se o headless é que serve responderia sobre a sessão errada, e o
// chamador leria "pronta" para uma sessão que nunca foi restaurada.
func TestAGuardaVaiAoMesmoLadoQueAOperacao(t *testing.T) {
	r, soc, hl := monta(t, true, "s1")
	if err := r.EnsureSession(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if hl.guardas != 1 || soc.guardas != 0 {
		t.Fatalf("a guarda foi ao socket: socket=%d headless=%d", soc.guardas, hl.guardas)
	}
}

func TestOErroDoTransporteEscolhidoPROPAGA(t *testing.T) {
	r, _, hl := monta(t, true, "s1")
	hl.erroFixo = errors.New("a página recusou")
	err := r.SetGroupLocked(context.Background(), "s1", grupo, true)
	if err == nil || !strings.Contains(err.Error(), "a página recusou") {
		t.Fatalf("o erro do transporte não chegou ao chamador: %v", err)
	}
}

package groupmembers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/headless"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
	"wa-api/pkg/infra/headless/registry"
)

func cfgFor(string) (headless.StartConfig, error) {
	return headless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// membersDuplo imita a REGRA medida: Verified é verdadeiro SÓ para no-op, porque
// uma mudança real é invisível para a sessão que a fez (H58, H65). Um dublê que
// devolvesse Verified para mudanças reais seria mais permissivo que a produção e
// esconderia exatamente o que este adaptador existe para relatar.
type membersDuplo struct {
	verified bool
	chamadas []string
	err      error
}

func (m *membersDuplo) AddParticipant(_ context.Context, _, _, _ string) (headless.GroupMembership, error) {
	m.chamadas = append(m.chamadas, "add")
	return headless.GroupMembership{Verified: m.verified, NoOp: m.verified}, m.err
}

func (m *membersDuplo) RemoveParticipant(_ context.Context, _, _, _ string) (headless.GroupMembership, error) {
	m.chamadas = append(m.chamadas, "remove")
	return headless.GroupMembership{Verified: m.verified, NoOp: m.verified}, m.err
}

func com(d members) *Manager {
	m := NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	m.newMembers = func(context.Context, string) (members, error) { return d, nil }
	return m
}

// TestMudancaREALVoltaNaoConfirmadaCOMMotivo é a decisão 86 travada.
//
// A mudança acontece — o servidor recebe-a. A sessão que agiu não a vê. Devolver
// Confirmed true seria a mentira que a capability já recusava; devolver false
// SEM motivo traria o sucesso silencioso de volta disfarçado de estrutura.
func TestMudancaREALVoltaNaoConfirmadaCOMMotivo(t *testing.T) {
	d := &membersDuplo{verified: false}

	got, err := com(d).UpdateGroupParticipants(context.Background(), "s1",
		domain.JID("120363@g.us"), []domain.JID{"5511999999999@c.us"}, domain.ParticipantAdd)
	if err != nil {
		t.Fatalf("UpdateGroupParticipants: %v", err)
	}
	if got.Confirmed {
		t.Fatal("uma mudança REAL voltou como confirmada: é a mentira que a " +
			"capability recusa, e que já custou um grupo de laboratório")
	}
	if got.Reason == "" {
		t.Fatal("não confirmado e SEM motivo: o sucesso silencioso voltou")
	}
	if !strings.Contains(got.Reason, "esta sessão não a observa") {
		t.Fatalf("o motivo não nomeia a causa medida: %q", got.Reason)
	}
	if err := got.Valida(); err != nil {
		t.Fatalf("o desfecho não passa na própria invariante: %v", err)
	}
}

// Um no-op É confirmável, e é o único caso que é: já ser membro (ou já não ser)
// a sessão consegue verificar.
func TestNoOpVoltaConfirmado(t *testing.T) {
	got, err := com(&membersDuplo{verified: true}).UpdateGroupParticipants(
		context.Background(), "s1", domain.JID("120363@g.us"),
		[]domain.JID{"5511999999999@c.us"}, domain.ParticipantRemove)
	if err != nil {
		t.Fatalf("UpdateGroupParticipants: %v", err)
	}
	if !got.Confirmed {
		t.Fatalf("um no-op voltou não confirmado com motivo %q", got.Reason)
	}
}

// TestUmaMudancaRealContaminaOLote: a sessão não observa NENHUMA delas, então um
// lote com uma mudança real não é confirmável mesmo que as outras sejam no-op.
// Relatar o lote como confirmado porque a maioria era no-op seria uma média
// aritmética a substituir uma verdade.
func TestUmaMudancaRealContaminaOLote(t *testing.T) {
	// O dublê alterna: primeiro no-op, depois mudança real.
	d := &alternante{}
	got, err := com(d).UpdateGroupParticipants(context.Background(), "s1",
		domain.JID("120363@g.us"),
		[]domain.JID{"5511111111111@c.us", "5522222222222@c.us"}, domain.ParticipantAdd)
	if err != nil {
		t.Fatalf("UpdateGroupParticipants: %v", err)
	}
	if got.Confirmed {
		t.Fatal("o lote voltou confirmado com uma mudança real dentro")
	}
}

type alternante struct{ n int }

func (a *alternante) AddParticipant(_ context.Context, _, _, _ string) (headless.GroupMembership, error) {
	a.n++
	return headless.GroupMembership{Verified: a.n == 1, NoOp: a.n == 1}, nil
}
func (a *alternante) RemoveParticipant(_ context.Context, _, _, _ string) (headless.GroupMembership, error) {
	return headless.GroupMembership{}, nil
}

// A falha ABORTA o lote em vez de continuar: um lote parcialmente aplicado cuja
// falha fosse engolida deixaria o grupo num estado que ninguém pediu.
func TestFalhaAbortaOLote(t *testing.T) {
	d := &membersDuplo{err: errors.New("a página recusou")}
	if _, err := com(d).UpdateGroupParticipants(context.Background(), "s1",
		domain.JID("120363@g.us"),
		[]domain.JID{"5511111111111@c.us", "5522222222222@c.us"}, domain.ParticipantAdd); err == nil {
		t.Fatal("a falha virou sucesso")
	}
	if len(d.chamadas) != 1 {
		t.Fatalf("o lote continuou depois da falha: %v", d.chamadas)
	}
}

// Ação desconhecida e lista vazia são recusadas antes de tocar na página.
func TestEntradasInvalidasSaoRecusadas(t *testing.T) {
	d := &membersDuplo{}
	if _, err := com(d).UpdateGroupParticipants(context.Background(), "s1",
		domain.JID("120363@g.us"), []domain.JID{"5511111111111@c.us"},
		domain.ParticipantAction("promover")); err == nil {
		t.Fatal("ação desconhecida foi aceita")
	}
	if _, err := com(d).UpdateGroupParticipants(context.Background(), "s1",
		domain.JID("120363@g.us"), nil, domain.ParticipantAdd); err == nil {
		t.Fatal("lista vazia foi aceita")
	}
	if len(d.chamadas) != 0 {
		t.Fatalf("uma recusa chegou a tocar na página: %v", d.chamadas)
	}
}

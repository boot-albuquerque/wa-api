package groupdir

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

// listerDuplo imita a REGRA: a página não tem coleção de grupos, então a lista
// vem MISTURADA e o filtro é do adaptador. Um dublê que devolvesse só grupos
// esconderia exatamente o defeito que este teste procura.
type listerDuplo struct {
	lista headless.ChatList
	chat  headless.Chat
	err   error
}

func (l listerDuplo) List(context.Context, int, string) (headless.ChatList, error) {
	return l.lista, l.err
}
func (l listerDuplo) ByJID(context.Context, string, string) (headless.Chat, error) {
	return l.chat, l.err
}

type inviterDuplo struct {
	code headless.GroupInviteCode
	info headless.GroupInvite
	err  error
}

func (i inviterDuplo) InviteCode(context.Context, string, string) (headless.GroupInviteCode, error) {
	return i.code, i.err
}
func (i inviterDuplo) InviteInfo(context.Context, string, string) (headless.GroupInvite, error) {
	return i.info, i.err
}

func com(l lister, i inviter) *Directory {
	d := NewDirectory(adapter.NewSessions(registry.New(1), cfgFor))
	if l != nil {
		d.newLister = func(context.Context, string) (lister, error) { return l, nil }
	}
	if i != nil {
		d.newInviter = func(context.Context, string) (inviter, error) { return i, nil }
	}
	return d
}

func listaMista() headless.ChatList {
	return headless.ChatList{
		Chats: []headless.Chat{
			{JID: "111@lid", Title: "Ana"},
			{JID: "120363000000000@g.us", Title: "Laboratório", IsGroup: true},
			{JID: "222@lid", Title: "Bruno"},
			{JID: "120363111111111@g.us", Title: "Equipa", IsGroup: true},
		},
		Total: 4,
	}
}

func TestSatisfazOPortDeDiretorio(t *testing.T) {
	var d any = NewDirectory(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := d.(appport.GroupDirectory); !ok {
		t.Fatal("não satisfaz GroupDirectory")
	}
}

// TestAContagemEDEGRUPOSENaoDeConversas: a página não separa grupos numa
// coleção própria, então devolver o total de conversas faria o chamador acreditar
// que está em centenas de grupos.
func TestAContagemEDEGRUPOSENaoDeConversas(t *testing.T) {
	_, n, err := com(listerDuplo{lista: listaMista()}, nil).
		ListJoinedGroups(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListJoinedGroups: %v", err)
	}
	if n == 4 {
		t.Fatal("a contagem devolveu TODAS as conversas: o chamador acreditaria " +
			"estar em grupos onde só tem conversas de pessoas")
	}
	if n != 2 {
		t.Fatalf("contagem=%d, quero 2 grupos", n)
	}
}

// Os nomes vêm só dos grupos, e são o título que o próprio app renderiza.
func TestNomesSaoSoDosGrupos(t *testing.T) {
	nomes, err := com(listerDuplo{lista: listaMista()}, nil).
		GroupNames(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GroupNames: %v", err)
	}
	if len(nomes) != 2 {
		t.Fatalf("nomes=%v, quero só os dois grupos", nomes)
	}
	if nomes[domain.JID("120363000000000@g.us")] != "Laboratório" {
		t.Fatalf("título errado: %v", nomes)
	}
	if _, temPessoa := nomes[domain.JID("111@lid")]; temPessoa {
		t.Fatal("uma pessoa entrou no mapa de nomes de GRUPO")
	}
}

// TestJIDDePessoaNaoPassaPorGrupo: pedir informação de grupo com o jid de uma
// pessoa devolveria uma conversa bem-formada e errada. Recusar aqui é mais
// barato que deixar o chamador tratar uma pessoa como grupo.
func TestJIDDePessoaNaoPassaPorGrupo(t *testing.T) {
	d := com(listerDuplo{chat: headless.Chat{JID: "111@lid", IsGroup: false}}, nil)
	_, err := d.GetGroupInfo(context.Background(), "s1", domain.JID("120363@g.us"))
	if err == nil {
		t.Fatal("uma conversa de pessoa foi devolvida como grupo")
	}
	if !strings.Contains(err.Error(), "not a group") {
		t.Fatalf("o erro não nomeia a causa: %v", err)
	}
}

// TestOLinkEPedidoEXPLICITAMENTE: Link() é método e não campo justamente para
// que a forma perigosa — o url que qualquer um pode seguir — seja pedida, e para
// que ela nunca divirja do código.
func TestOLinkEPedidoEXPLICITAMENTE(t *testing.T) {
	got, err := com(nil, inviterDuplo{code: headless.GroupInviteCode{Code: "AbCdEf"}}).
		GetGroupInviteLink(context.Background(), "s1", domain.JID("120363@g.us"))
	if err != nil {
		t.Fatalf("GetGroupInviteLink: %v", err)
	}
	if got == "AbCdEf" {
		t.Fatal("devolveu o CÓDIGO cru onde o port promete o LINK")
	}
	if !strings.Contains(got, "AbCdEf") {
		t.Fatalf("o link não contém o código: %q", got)
	}
}

// Código vazio é recusado antes de tocar na página.
func TestCodigoVazioERecusado(t *testing.T) {
	if _, err := com(nil, inviterDuplo{}).
		GetGroupInfoFromLink(context.Background(), "s1", ""); err == nil {
		t.Fatal("código vazio foi aceito")
	}
}

func TestFalhaDaCapabilityPropaga(t *testing.T) {
	if _, _, err := com(listerDuplo{err: errors.New("a página recusou")}, nil).
		ListJoinedGroups(context.Background(), "s1"); err == nil {
		t.Fatal("a falha virou lista vazia — 'não está em grupos' e 'não " +
			"conseguimos ler' são coisas diferentes")
	}
}

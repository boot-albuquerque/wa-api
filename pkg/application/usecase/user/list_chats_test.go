package user_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// ListChatsUseCase junta três fontes que nenhuma sozinha responde "com quem
// eu falei por último": o histórico local (quando), o roster (nome do
// contato) e os grupos (nome do grupo).
//
// A junção é por JID, e é aí que mora o risco: o histórico mistura `@lid` e
// `@s.whatsapp.net` enquanto o roster é majoritariamente `@lid` (F65).

const listaUser = "user-1"

func t0(min int) time.Time {
	return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)
}

// portasDaLista devolve três chats: dois contatos (um no roster, outro não) e
// um grupo.
func portasDaLista() (*contractsfake.ChatActivityReader, *contractsfake.ContactDirectory, *contractsfake.GroupDirectory) {
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{
				"111@lid":     t0(10), // no roster
				"222@lid":     t0(30), // fora do roster
				"120363@g.us": t0(20), // grupo
			}, nil
		},
	}
	cd := &contractsfake.ContactDirectory{
		ContactNamesFunc: func(context.Context, string) (map[domain.JID]domain.ContactName, error) {
			return map[domain.JID]domain.ContactName{
				"111@lid": {FullName: "Alice Agenda", PushName: "Ali"},
			}, nil
		},
	}
	gd := &contractsfake.GroupDirectory{
		GroupNamesFunc: func(context.Context, string) (map[domain.JID]string, error) {
			return map[domain.JID]string{"120363@g.us": "Time de Obras"}, nil
		},
	}
	return ar, cd, gd
}

func listar(t *testing.T, ar *contractsfake.ChatActivityReader, cd *contractsfake.ContactDirectory, gd *contractsfake.GroupDirectory, limit, offset int) *domain.ChatListPage {
	t.Helper()
	page, err := user.NewListChatsUseCase(ar, cd, gd, &contractsfake.Logger{}).
		Execute(context.Background(), listaUser, limit, offset)
	if err != nil {
		t.Fatalf("Execute devolveu erro: %v", err)
	}
	return page
}

// TestLista_OrdenaDaMaisRecente é o serviço principal da rota.
func TestLista_OrdenaDaMaisRecente(t *testing.T) {
	ar, cd, gd := portasDaLista()
	page := listar(t, ar, cd, gd, 0, 0)

	if len(page.Chats) != 3 || page.Total != 3 {
		t.Fatalf("esperava 3 conversas, veio %d (total %d)", len(page.Chats), page.Total)
	}
	quero := []string{"222@lid", "120363@g.us", "111@lid"} // 30, 20, 10 min
	for i, jid := range quero {
		if page.Chats[i].JID != jid {
			t.Errorf("posicao %d = %q, quero %q — a ordenacao por ultima interacao quebrou",
				i, page.Chats[i].JID, jid)
		}
	}
}

// TestLista_NomeiaContatoEGrupoDeFontesDiferentes: o roster não tem entradas
// `@g.us`, então procurar um grupo nele devolveria vazio SEM erro — o defeito
// seria silencioso.
func TestLista_NomeiaContatoEGrupoDeFontesDiferentes(t *testing.T) {
	ar, cd, gd := portasDaLista()
	page := listar(t, ar, cd, gd, 0, 0)

	porJID := map[string]domain.ChatSummary{}
	for _, c := range page.Chats {
		porJID[c.JID] = c
	}

	if got := porJID["111@lid"]; got.Name != "Alice Agenda" || got.IsGroup {
		t.Errorf("contato do roster: name=%q is_group=%v", got.Name, got.IsGroup)
	}
	if got := porJID["120363@g.us"]; got.Name != "Time de Obras" || !got.IsGroup {
		t.Errorf("grupo: name=%q is_group=%v", got.Name, got.IsGroup)
	}
	// Fora do roster sai SEM nome, e não fica de fora: omiti-la esconderia
	// uma conversa que existe.
	if got, ok := porJID["222@lid"]; !ok || got.Name != "" {
		t.Errorf("contato fora do roster: presente=%v name=%q", ok, got.Name)
	}
}

// TestLista_Paginacao cobre o padrão, o corte e o offset além do fim — este
// último é o pedido normal de quem chegou ao fim da lista, e um slice mal
// feito entraria em pânico ali.
func TestLista_Paginacao(t *testing.T) {
	for _, tc := range []struct {
		nome          string
		limit, offset int
		querN         int
		querLimit     int
	}{
		{"padrao quando nao pedem", 0, 0, 3, user.LimitPadrao},
		{"corta pelo limit", 2, 0, 2, 2},
		{"offset no meio", 2, 2, 1, 2},
		{"offset alem do fim", 10, 99, 0, 10},
		{"limit negativo vira padrao", -5, 0, 3, user.LimitPadrao},
		{"offset negativo vira zero", 0, -3, 3, user.LimitPadrao},
		{"limit acima do teto e' cortado", 10000, 0, 3, user.LimitMaximo},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			ar, cd, gd := portasDaLista()

			page := listar(t, ar, cd, gd, tc.limit, tc.offset)

			if len(page.Chats) != tc.querN {
				t.Errorf("veio %d conversas, quero %d", len(page.Chats), tc.querN)
			}
			if page.Limit != tc.querLimit {
				t.Errorf("limit ecoado = %d, quero %d", page.Limit, tc.querLimit)
			}
			// Total é da lista INTEIRA, não da página: é o que diz ao cliente
			// quanto falta sem obrigá-lo a paginar até o fim.
			if page.Total != 3 {
				t.Errorf("total = %d, quero 3 (o total nao pode ser o da pagina)", page.Total)
			}
		})
	}
}

// TestLista_EmpateEhDeterministico: sem desempate, dois chats com o mesmo
// timestamp trocam de posição entre chamadas (ordem de map é aleatória), e um
// cliente paginando vê a mesma conversa duas vezes ou nenhuma.
func TestLista_EmpateEhDeterministico(t *testing.T) {
	mesmo := t0(5)
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{"aaa@lid": mesmo, "bbb@lid": mesmo, "ccc@lid": mesmo}, nil
		},
	}
	cd, gd := &contractsfake.ContactDirectory{}, &contractsfake.GroupDirectory{}

	primeira := listar(t, ar, cd, gd, 0, 0)
	for i := 0; i < 20; i++ {
		outra := listar(t, ar, cd, gd, 0, 0)
		for j := range primeira.Chats {
			if primeira.Chats[j].JID != outra.Chats[j].JID {
				t.Fatalf("ordem mudou entre chamadas na posicao %d: %q vs %q",
					j, primeira.Chats[j].JID, outra.Chats[j].JID)
			}
		}
	}
}

// TestLista_RosterIndisponivelDegrada: ordenar conversas é o serviço
// principal; o nome é enriquecimento. Falhar a rota porque o roster não veio
// a tornaria indisponível justamente quando a rede está ruim.
func TestLista_RosterIndisponivelDegrada(t *testing.T) {
	ar, cd, gd := portasDaLista()
	cd.ContactNamesFunc = func(context.Context, string) (map[domain.JID]domain.ContactName, error) {
		return nil, errors.New("roster fora do ar")
	}
	gd.GroupNamesFunc = func(context.Context, string) (map[domain.JID]string, error) {
		return nil, errors.New("grupos fora do ar")
	}

	page := listar(t, ar, cd, gd, 0, 0)

	if len(page.Chats) != 3 {
		t.Fatalf("a lista sumiu com o roster: %d conversas", len(page.Chats))
	}
	for _, c := range page.Chats {
		if c.Name != "" {
			t.Errorf("%s veio com nome %q apesar das fontes indisponiveis", c.JID, c.Name)
		}
		if c.LastActivity.IsZero() {
			t.Errorf("%s perdeu o timestamp, que e' o dado principal", c.JID)
		}
	}
}

// TestLista_HistoricoIndisponivelFalha: o histórico É a lista. Sem ele não há
// resposta parcial que faça sentido — degradar aqui devolveria uma lista
// vazia indistinguível de "não há conversas".
func TestLista_HistoricoIndisponivelFalha(t *testing.T) {
	falha := errors.New("banco fora do ar")
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return nil, falha
		},
	}

	_, err := user.NewListChatsUseCase(ar, &contractsfake.ContactDirectory{}, &contractsfake.GroupDirectory{}, &contractsfake.Logger{}).
		Execute(context.Background(), listaUser, 0, 0)

	if !errors.Is(err, falha) {
		t.Fatalf("a causa se perdeu: %v", err)
	}
}

// TestNomeMelhor_Preferencia trava a ordem declarada: sem ela, cada chamador
// inventaria a sua e a lista mudaria de nome conforme quem a monta.
func TestNomeMelhor_Preferencia(t *testing.T) {
	for _, tc := range []struct {
		nome  string
		c     domain.ContactName
		quero string
	}{
		{"agenda vence todos", domain.ContactName{FullName: "A", PushName: "B", BusinessName: "C", FirstName: "D"}, "A"},
		{"push vence comercial", domain.ContactName{PushName: "B", BusinessName: "C"}, "B"},
		{"comercial vence primeiro nome", domain.ContactName{BusinessName: "C", FirstName: "D"}, "C"},
		{"primeiro nome e' o ultimo recurso", domain.ContactName{FirstName: "D"}, "D"},
		{"sem nome nenhum", domain.ContactName{}, ""},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			if got := tc.c.Melhor(); got != tc.quero {
				t.Errorf("Melhor() = %q, quero %q", got, tc.quero)
			}
		})
	}
}

// --- F84: o pushName do histórico como fonte de nome ---------------------

// TestLista_NomeVemDoHistoricoQuandoRosterNaoSabe é o ganho da F84 na lista.
// Para identidades @lid o roster está vazio na maioria dos casos, e o
// pushName que o WhatsApp manda em cada mensagem é a fonte mais completa.
func TestLista_NomeVemDoHistoricoQuandoRosterNaoSabe(t *testing.T) {
	ar, cd, gd := portasDaLista()
	ar.GetChatPushNamesFunc = func(context.Context, string) (map[string]string, error) {
		return map[string]string{"222@lid": "Bruno do Historico"}, nil
	}

	page := listar(t, ar, cd, gd, 0, 0)

	for _, c := range page.Chats {
		if c.JID == "222@lid" {
			if c.Name != "Bruno do Historico" {
				t.Fatalf("name = %q, quero o do historico — a fonte da F84 nao esta ligada", c.Name)
			}
			return
		}
	}
	t.Fatal("222@lid sumiu da lista")
}

// TestLista_RosterVenceOHistorico fixa a ORDEM. O roster é o nome que QUEM
// CONSULTA escolheu; o pushName é o que o contato escolheu para si. Inverter
// faria "Maria Contadora" da agenda virar o apelido que a pessoa usa.
func TestLista_RosterVenceOHistorico(t *testing.T) {
	ar, cd, gd := portasDaLista()
	ar.GetChatPushNamesFunc = func(context.Context, string) (map[string]string, error) {
		return map[string]string{"111@lid": "Apelido Dela"}, nil
	}

	page := listar(t, ar, cd, gd, 0, 0)

	for _, c := range page.Chats {
		if c.JID == "111@lid" {
			if c.Name != "Alice Agenda" {
				t.Fatalf("name = %q, quero o da agenda — a ordem das fontes inverteu", c.Name)
			}
			return
		}
	}
	t.Fatal("111@lid sumiu da lista")
}

// TestLista_HistoricoIndisponivelNaoDerruba: a terceira fonte também degrada.
func TestLista_HistoricoDeNomesIndisponivelDegrada(t *testing.T) {
	ar, cd, gd := portasDaLista()
	ar.GetChatPushNamesFunc = func(context.Context, string) (map[string]string, error) {
		return nil, errors.New("banco fora do ar")
	}

	page := listar(t, ar, cd, gd, 0, 0)

	if len(page.Chats) != 3 {
		t.Fatalf("a lista sumiu: %d conversas", len(page.Chats))
	}
	// O roster continua valendo para quem ele conhece.
	for _, c := range page.Chats {
		if c.JID == "111@lid" && c.Name != "Alice Agenda" {
			t.Errorf("o roster parou de valer quando o historico falhou: %q", c.Name)
		}
	}
}

// TestLista_GrupoNaoUsaOHistorico: grupo tem nome próprio, e o pushName de um
// participante NÃO é o nome do grupo. Usar o histórico ali batizaria o grupo
// com o nome de quem falou por último.
func TestLista_GrupoNaoUsaOHistorico(t *testing.T) {
	ar, cd, gd := portasDaLista()
	gd.GroupNamesFunc = func(context.Context, string) (map[domain.JID]string, error) {
		return map[domain.JID]string{}, nil // grupo desconhecido
	}
	ar.GetChatPushNamesFunc = func(context.Context, string) (map[string]string, error) {
		return map[string]string{"120363@g.us": "Participante Qualquer"}, nil
	}

	page := listar(t, ar, cd, gd, 0, 0)

	for _, c := range page.Chats {
		if c.JID == "120363@g.us" && c.Name != "" {
			t.Fatalf("grupo batizado com %q, que e' o pushName de um participante", c.Name)
		}
	}
}

// --- F84: os dois espaços de identidade têm de se encontrar --------------

// TestLista_NomeGravadoComoPN_AlcancaChatNormalizadoParaLID reproduz a
// condição REAL medida em produção, e que os testes acima não pegavam porque
// seus dublês chaveavam tudo no mesmo espaço.
//
// O histórico grava o chat_jid como ele veio — e 120 dos 135 chats com
// pushName vieram como `@s.whatsapp.net`. A lista, porém, normaliza as
// chaves de atividade para `@lid` (normalizeToLID). Sem aliasar as tabelas
// de consulta, os dois lados nunca se encontram: medido, 11 nomes
// aproveitados de 480 chats enquanto 7.273 mensagens tinham nome gravado.
func TestLista_NomeGravadoComoPN_AlcancaChatNormalizadoParaLID(t *testing.T) {
	const pn = "5511999@s.whatsapp.net"
	const lid = "90937@lid"

	ar := &contractsfake.ChatActivityReader{
		// A atividade chega como PN...
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{pn: t0(10)}, nil
		},
		// ...e o pushName também foi gravado sob o PN.
		GetChatPushNamesFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{pn: "Maria do Historico"}, nil
		},
	}
	// ...mas o store conhece o LID, então normalizeToLID reescreve a chave
	// de atividade para @lid. É exatamente aqui que o encontro se perdia.
	cd := &contractsfake.ContactDirectory{
		GetManyLIDsForPNsFunc: func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			return map[domain.JID]domain.JID{pn: lid}, nil
		},
	}
	gd := &contractsfake.GroupDirectory{}

	page := listar(t, ar, cd, gd, 0, 0)

	if len(page.Chats) != 1 {
		t.Fatalf("esperava 1 conversa, veio %d", len(page.Chats))
	}
	c := page.Chats[0]
	if c.JID != lid {
		t.Fatalf("JID = %q, quero o normalizado %q", c.JID, lid)
	}
	if c.Name != "Maria do Historico" {
		t.Fatalf("name = %q; o nome gravado como PN nao alcancou o chat normalizado para LID", c.Name)
	}
}

// TestLista_RosterGravadoComoPN_AlcancaChatNormalizado: o mesmo vale para o
// roster, que também mistura os dois espaços (886 chaves PN medidas).
func TestLista_RosterGravadoComoPN_AlcancaChatNormalizado(t *testing.T) {
	const pn = "5511999@s.whatsapp.net"
	const lid = "90937@lid"

	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{pn: t0(10)}, nil
		},
	}
	cd := &contractsfake.ContactDirectory{
		ContactNamesFunc: func(context.Context, string) (map[domain.JID]domain.ContactName, error) {
			return map[domain.JID]domain.ContactName{pn: {FullName: "Maria da Agenda"}}, nil
		},
		GetManyLIDsForPNsFunc: func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			return map[domain.JID]domain.JID{pn: lid}, nil
		},
	}

	page := listar(t, ar, cd, &contractsfake.GroupDirectory{}, 0, 0)

	if len(page.Chats) != 1 || page.Chats[0].Name != "Maria da Agenda" {
		t.Fatalf("o roster gravado como PN nao alcancou o chat normalizado: %+v", page.Chats)
	}
}

// TestLista_AliasNaoSobrescreveEntradaDiretaPorLID: uma entrada que JÁ veio
// chaveada por LID é mais direta que a derivada de um PN, e não pode ser
// substituída por ela.
func TestLista_AliasNaoSobrescreveEntradaDiretaPorLID(t *testing.T) {
	const pn = "5511999@s.whatsapp.net"
	const lid = "90937@lid"

	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{lid: t0(10)}, nil
		},
		GetChatPushNamesFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{lid: "Nome Direto", pn: "Nome Derivado"}, nil
		},
	}
	cd := &contractsfake.ContactDirectory{
		GetManyLIDsForPNsFunc: func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			return map[domain.JID]domain.JID{pn: lid}, nil
		},
	}

	page := listar(t, ar, cd, &contractsfake.GroupDirectory{}, 0, 0)

	if len(page.Chats) != 1 || page.Chats[0].Name != "Nome Direto" {
		t.Fatalf("o alias derivado do PN sobrescreveu a entrada direta por LID: %+v", page.Chats)
	}
}

// --- F181: o telefone como identificador legível -----------------------------

// TestListaChats_TelefoneResolvidoParaConversaSemNome é o defeito medido em
// campo: 48 conversas mostram `182699419517150@lid`, que não diz nada a
// ninguém, e nós temos o número para 100% delas.
func TestListaChats_TelefoneResolvidoParaConversaSemNome(t *testing.T) {
	ar, cd, gd := portasDaLista()
	cd.GetPNForLIDFunc = func(_ context.Context, _ string, lid domain.JID) (domain.JID, error) {
		if lid == "222@lid" {
			return "556799881100:37@s.whatsapp.net", nil
		}
		return "", nil
	}

	page := listar(t, ar, cd, gd, 10, 0)
	sem := acharChat(t, page, "222@lid")
	if sem.Phone != "556799881100" {
		t.Errorf("Phone = %q, quero \"556799881100\"", sem.Phone)
	}
	// O JID NÃO muda: é o que o cliente usa nas outras rotas.
	if sem.JID != "222@lid" {
		t.Errorf("JID = %q: o telefone e' campo PROPRIO, nao substituto", sem.JID)
	}
	// E o telefone NÃO vira nome: quem ordena por nome não pode passar a
	// misturar nomes com números.
	if sem.Name != "" {
		t.Errorf("Name = %q: telefone nao pode virar nome", sem.Name)
	}
}

// TestListaChats_TelefoneNaoApagaNomeExistente: quem TEM nome continua a tê-lo,
// e ganha o telefone por acréscimo.
func TestListaChats_TelefoneNaoApagaNomeExistente(t *testing.T) {
	ar, cd, gd := portasDaLista()
	cd.GetPNForLIDFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "551199999999@s.whatsapp.net", nil
	}

	com := acharChat(t, listar(t, ar, cd, gd, 10, 0), "111@lid")
	if com.Name != "Alice Agenda" {
		t.Errorf("Name = %q, quero \"Alice Agenda\": o telefone nao pode sobrepor o nome", com.Name)
	}
	if com.Phone != "551199999999" {
		t.Errorf("Phone = %q, quero o numero", com.Phone)
	}
}

// TestListaChats_ResolucaoCustaPELAPAGINA é a invariante que mais me importou.
//
// A lista tem 734 conversas no tenant real e devolve no máximo `limit`.
// Resolver ANTES de fatiar faria o trabalho crescer com o TOTAL em vez de com o
// que se devolve — um mecanismo cujo custo cresce com o que ninguém pediu, que
// é a Regra 1 do CLAUDE.md ignorada.
//
// O teste conta CHAMADAS, não resultados: é a única forma de a inversão
// aparecer, porque resolver tudo e devolver a página produz exatamente a mesma
// resposta.
func TestListaChats_ResolucaoCustaPELAPAGINA(t *testing.T) {
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			m := make(map[string]time.Time, 100)
			for i := range 100 {
				m[fmt.Sprintf("%d@lid", 1000+i)] = t0(i)
			}
			return m, nil
		},
	}
	cd := &contractsfake.ContactDirectory{
		GetPNForLIDFunc: func(context.Context, string, domain.JID) (domain.JID, error) {
			return "5511000000000@s.whatsapp.net", nil
		},
	}

	listar(t, ar, cd, &contractsfake.GroupDirectory{}, 5, 0)

	if n := len(cd.GetPNForLIDCalls); n != 5 {
		t.Fatalf("resolucoes = %d para uma pagina de 5 em 100 conversas: o custo tem de crescer com a PAGINA, nao com o total", n)
	}
}

// TestListaChats_GrupoNaoConsultaOResolvedor: grupo não tem telefone, e o store
// real recusa GetPNForLID com um JID que não seja @lid.
func TestListaChats_GrupoNaoConsultaOResolvedor(t *testing.T) {
	ar, cd, gd := portasDaLista()
	cd.GetPNForLIDFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "5511000000000@s.whatsapp.net", nil
	}

	page := listar(t, ar, cd, gd, 10, 0)
	if g := acharChat(t, page, "120363@g.us"); g.Phone != "" {
		t.Errorf("grupo veio com Phone = %q", g.Phone)
	}
	for _, c := range cd.GetPNForLIDCalls {
		if !strings.HasSuffix(string(c.LID), "@lid") {
			t.Errorf("resolvedor chamado com %q: o store real recusa quem nao e' @lid", c.LID)
		}
	}
}

// TestListaChats_ResolvedorEmFalhaNaoDerrubaALista: o telefone é
// enriquecimento, como o nome de contato e o de grupo já eram. Sessão fora do
// ar devolve a lista sem telefone, não um erro.
func TestListaChats_ResolvedorEmFalhaNaoDerrubaALista(t *testing.T) {
	ar, cd, gd := portasDaLista()
	cd.GetPNForLIDFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "", errors.New("sessao offline")
	}

	page := listar(t, ar, cd, gd, 10, 0)
	if len(page.Chats) != 3 {
		t.Fatalf("conversas = %d, quero 3: resolvedor em falha nao pode derrubar a lista", len(page.Chats))
	}
	for _, c := range page.Chats {
		if c.Phone != "" {
			t.Errorf("chat %s veio com Phone %q apesar da falha", c.JID, c.Phone)
		}
	}
}

func acharChat(t *testing.T, page *domain.ChatListPage, jid string) domain.ChatSummary {
	t.Helper()
	for _, c := range page.Chats {
		if c.JID == jid {
			return c
		}
	}
	t.Fatalf("chat %q ausente da pagina", jid)
	return domain.ChatSummary{}
}

// TestListaChats_ParaNaPrimeiraFalhaDoResolvedor trava o comportamento que o
// gate de cobertura de log me levou a escrever.
//
// GetPNForLID só falha quando a sessão não está registada, e isso não muda
// entre uma conversa e a seguinte da mesma página. Insistir produziria N
// chamadas condenadas e N linhas de log por requisição — a F180 outra vez.
//
// O teste conta CHAMADAS: com dez conversas e o resolvedor sempre em falha,
// tem de haver exatamente UMA.
func TestListaChats_ParaNaPrimeiraFalhaDoResolvedor(t *testing.T) {
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			m := make(map[string]time.Time, 10)
			for i := range 10 {
				m[fmt.Sprintf("%d@lid", 2000+i)] = t0(i)
			}
			return m, nil
		},
	}
	cd := &contractsfake.ContactDirectory{
		GetPNForLIDFunc: func(context.Context, string, domain.JID) (domain.JID, error) {
			return "", errors.New("sessao offline")
		},
	}

	page := listar(t, ar, cd, &contractsfake.GroupDirectory{}, 10, 0)

	if n := len(cd.GetPNForLIDCalls); n != 1 {
		t.Fatalf("chamadas = %d para 10 conversas com o resolvedor sempre em falha: quero 1, porque a falha nao muda entre conversas", n)
	}
	if len(page.Chats) != 10 {
		t.Errorf("conversas = %d, quero 10: parar de resolver nao pode encurtar a lista", len(page.Chats))
	}
}

// TestListaChats_UmaFalhaNaoImpedeAsAnteriores: quem já foi resolvido ANTES da
// falha mantém o telefone. Parar não pode desfazer o que já se conseguiu.
func TestListaChats_UmaFalhaNaoImpedeAsAnteriores(t *testing.T) {
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{
				"aaa@lid": t0(30), // mais recente: resolvido primeiro
				"bbb@lid": t0(20),
				"ccc@lid": t0(10),
			}, nil
		},
	}
	chamadas := 0
	cd := &contractsfake.ContactDirectory{
		GetPNForLIDFunc: func(context.Context, string, domain.JID) (domain.JID, error) {
			chamadas++
			if chamadas == 1 {
				return "551188887777@s.whatsapp.net", nil
			}
			return "", errors.New("sessao caiu no meio")
		},
	}

	page := listar(t, ar, cd, &contractsfake.GroupDirectory{}, 10, 0)
	if p := acharChat(t, page, "aaa@lid").Phone; p != "551188887777" {
		t.Errorf("Phone da primeira = %q, quero o numero: a falha seguinte nao pode desfazer o que ja' resolveu", p)
	}
	if n := len(cd.GetPNForLIDCalls); n != 2 {
		t.Errorf("chamadas = %d, quero 2 (uma boa, uma falha, e para)", n)
	}
}

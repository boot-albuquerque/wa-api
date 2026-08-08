package user_test

import (
	"context"
	"errors"
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

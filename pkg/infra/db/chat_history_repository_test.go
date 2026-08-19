package db

import (
	"context"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	"github.com/jmoiron/sqlx"
)

// Estes testes rodam contra SQLite REAL com o schema de producao (mesma via de
// message_history_test.go). Um fake de repositorio nao serviria: o defeito que
// eles travam e' a AUSENCIA de `WHERE user_id` na consulta do ramo `index`, e
// um fake que "esquece" o WHERE nao revela vazamento nenhum — ele nao tem
// WHERE para esquecer. So' um banco de verdade responde a pergunta.

// seedMessage grava uma mensagem com timestamp CONTROLADO.
//
// SaveMessageToHistory carimba time.Now() internamente, o que serve para os
// testes de idempotencia dela mas nao para os de ORDENACAO daqui: dois
// inserts consecutivos podem cair no mesmo instante observavel. O INSERT
// direto abaixo usa as MESMAS colunas do INSERT de producao
// (message_history.go), so' com o timestamp escolhido pelo teste.
func seedMessage(t *testing.T, db *sqlx.DB, userID, chatJID, messageID string, ts time.Time) {
	t.Helper()
	query := db.Rebind(`INSERT INTO message_history
		(user_id, chat_jid, sender_jid, message_id, timestamp, message_type,
		 text_content, media_link, quoted_message_id, datajson, sender_push_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if _, err := db.Exec(query, userID, chatJID, "sender@s.whatsapp.net", messageID, ts,
		"text", "texto de "+messageID, "", "", "", ""); err != nil {
		t.Fatalf("seed %s/%s/%s: %v", userID, chatJID, messageID, err)
	}
}

func TestChatHistoryRepositoryListChatMessages_ScopesByUserAndChat(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	seedMessage(t, db, "A", "c1@s.whatsapp.net", "A-C1-1", base)
	seedMessage(t, db, "A", "c2@s.whatsapp.net", "A-C2-1", base)
	seedMessage(t, db, "B", "c1@s.whatsapp.net", "B-C1-1", base)

	got, err := repo.ListChatMessages(context.Background(), "A", "c1@s.whatsapp.net", 50)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, quero 1 (so' a mensagem de A no chat c1); got = %+v", len(got), got)
	}
	if got[0].MessageID != "A-C1-1" {
		t.Errorf("MessageID = %q, quero A-C1-1", got[0].MessageID)
	}
	if got[0].UserID != "A" {
		t.Errorf("UserID = %q, quero A", got[0].UserID)
	}
}

// TestChatHistoryRepositoryListChatMessages_OrdersByTimestampDesc usa
// timestamps ASSIMETRICOS e fora da ordem de insercao: com timestamps
// crescentes na ordem de insercao, um `ORDER BY id` acidental passaria.
func TestChatHistoryRepositoryListChatMessages_OrdersByTimestampDesc(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	seedMessage(t, db, "A", "c1@s.whatsapp.net", "MEIO", base.Add(30*time.Minute))
	seedMessage(t, db, "A", "c1@s.whatsapp.net", "NOVA", base.Add(4*time.Hour))
	seedMessage(t, db, "A", "c1@s.whatsapp.net", "VELHA", base)

	got, err := repo.ListChatMessages(context.Background(), "A", "c1@s.whatsapp.net", 50)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	quero := []string{"NOVA", "MEIO", "VELHA"}
	if len(got) != len(quero) {
		t.Fatalf("len = %d, quero %d", len(got), len(quero))
	}
	for i, id := range quero {
		if got[i].MessageID != id {
			t.Errorf("posicao %d = %q, quero %q (ordem completa: %v)", i, got[i].MessageID, id, ids(got))
		}
	}
}

func TestChatHistoryRepositoryListChatMessages_LimitKeepsTheNewest(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	seedMessage(t, db, "A", "c1@s.whatsapp.net", "VELHA", base)
	seedMessage(t, db, "A", "c1@s.whatsapp.net", "NOVA", base.Add(time.Hour))

	got, err := repo.ListChatMessages(context.Background(), "A", "c1@s.whatsapp.net", 1)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 || got[0].MessageID != "NOVA" {
		t.Fatalf("got = %v, quero apenas [NOVA]", ids(got))
	}
}

// TestChatHistoryRepositoryListChatMessages_EmptyIsEmptySliceNotNil trava a
// forma do wire: ausencia de mensagem devolve `[]`, e uma slice nil marshalaria
// como `null`.
func TestChatHistoryRepositoryListChatMessages_EmptyIsEmptySliceNotNil(t *testing.T) {
	repo := NewChatHistoryRepository(newHistoryDB(t))

	got, err := repo.ListChatMessages(context.Background(), "A", "vazio@s.whatsapp.net", 50)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if got == nil {
		t.Fatal("slice nil; quero slice vazia (nil vira `null` no JSON, e o contrato e' `[]`)")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, quero 0", len(got))
	}
}

// TestChatHistoryRepositoryListChatMessages_ReadsNullableColumns cobre linhas
// gravadas ANTES das migracoes 6 e 8 (quoted_message_id e datajson), que
// carregam NULL: escanear NULL para string falha, e o teste morde se o scan
// voltar a usar string em vez de sql.NullString.
func TestChatHistoryRepositoryListChatMessages_ReadsNullableColumns(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)

	query := db.Rebind(`INSERT INTO message_history
		(user_id, chat_jid, sender_jid, message_id, timestamp, message_type,
		 text_content, media_link, quoted_message_id, datajson)
		VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL)`)
	if _, err := db.Exec(query, "A", "c1@s.whatsapp.net", "s@s.whatsapp.net", "ANTIGA",
		time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), "text"); err != nil {
		t.Fatalf("seed linha antiga: %v", err)
	}

	got, err := repo.ListChatMessages(context.Background(), "A", "c1@s.whatsapp.net", 50)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, quero 1", len(got))
	}
	if got[0].TextContent != "" || got[0].MediaLink != "" || got[0].QuotedMessageID != "" || got[0].DataJson != "" {
		t.Errorf("colunas NULL deveriam virar string vazia; got = %+v", got[0])
	}
}

// --- ramo index: isolamento de tenant ------------------------------------
//
// Os quatro casos do CAP-09A. O quarto (identidade ausente) e' de fronteira
// HTTP e vive em pkg/bootstrap/chat_history_route_test.go, porque so' a rota
// registrada exercita o fail-closed.

// Caso 1: A e B tem historico; a consulta de A devolve SO' a chave A.
func TestChatHistoryRepositoryChatIndexByUser_OnlyTheCallerKey(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	seedMessage(t, db, "A", "a1@s.whatsapp.net", "A-1", base)
	seedMessage(t, db, "A", "a2@s.whatsapp.net", "A-2", base.Add(time.Hour))
	seedMessage(t, db, "B", "b1@s.whatsapp.net", "B-1", base.Add(2*time.Hour))

	// Mapa em Go nao tem ordem: repetir a assercao e' o que distingue
	// "isolado" de "deu sorte na iteracao".
	for volta := 0; volta < 20; volta++ {
		got, err := repo.ChatIndexByUser(context.Background(), "A")
		if err != nil {
			t.Fatalf("ChatIndexByUser: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("volta %d: chaves = %d (%v), quero SO' a chave do caller", volta, len(got), chaves(got))
		}
		if _, temB := got["B"]; temB {
			t.Fatalf("volta %d: a chave B apareceu na resposta de A: %v", volta, got)
		}
		chats := got["A"]
		if len(chats) != 2 {
			t.Fatalf("volta %d: chats de A = %d, quero 2", volta, len(chats))
		}
		// Mais recente primeiro.
		if chats[0].ChatJID != "a2@s.whatsapp.net" || chats[1].ChatJID != "a1@s.whatsapp.net" {
			t.Errorf("volta %d: ordem = %v, quero [a2, a1] (last_message_time DESC)", volta, chats)
		}
		for _, c := range chats {
			if _, err := time.Parse(time.RFC3339Nano, c.LastUpdated); err != nil {
				t.Errorf("last_updated %q nao e' RFC3339Nano: %v", c.LastUpdated, err)
			}
		}
	}
}

// Caso 2: B tem um chat com o MESMO chat_jid de A. O isolamento e' por
// user_id, nao por chat_jid — se a consulta agrupasse so' por chat_jid, a
// linha de B entraria no total de A e este teste morderia.
func TestChatHistoryRepositoryChatIndexByUser_SameChatJIDAcrossTenants(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	const compartilhado = "mesmo@s.whatsapp.net"
	seedMessage(t, db, "A", compartilhado, "A-1", base)
	// A de B e' MAIS RECENTE: numa consulta global, o MAX(timestamp) do
	// grupo seria o de B, e o last_updated de A viria errado.
	seedMessage(t, db, "B", compartilhado, "B-1", base.Add(10*time.Hour))

	got, err := repo.ChatIndexByUser(context.Background(), "A")
	if err != nil {
		t.Fatalf("ChatIndexByUser: %v", err)
	}
	if len(got) != 1 || len(got["A"]) != 1 {
		t.Fatalf("got = %v, quero exatamente um chat sob a chave A", got)
	}
	quero := base.Format(time.RFC3339Nano)
	if got["A"][0].LastUpdated != quero {
		t.Errorf("last_updated = %q, quero %q — o timestamp de B vazou para o grupo de A",
			got["A"][0].LastUpdated, quero)
	}
}

// Caso 3: A nao tem historico e B tem. A resposta de A e' mapa VAZIO, e nada
// de B aparece.
func TestChatHistoryRepositoryChatIndexByUser_EmptyMapWhenCallerHasNoHistory(t *testing.T) {
	db := newHistoryDB(t)
	repo := NewChatHistoryRepository(db)

	seedMessage(t, db, "B", "b1@s.whatsapp.net", "B-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	got, err := repo.ChatIndexByUser(context.Background(), "A")
	if err != nil {
		t.Fatalf("ChatIndexByUser: %v", err)
	}
	if got == nil {
		t.Fatal("mapa nil; quero mapa vazio (a semantica historica de `{}`)")
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, quero mapa vazio — nenhum dado de B pode aparecer para A", got)
	}
}

// --- gate de History: a metade que vive na persistencia -------------------

func TestChatHistoryRepositoryHistoryLimit_ReadsPersistedValue(t *testing.T) {
	db := newRepoDB(t)
	repo := NewChatHistoryRepository(db)

	if _, err := db.Exec(db.Rebind(
		`INSERT INTO users (id, name, token, token_hash, history) VALUES (?, ?, ?, ?, ?)`),
		"A", "tenant A", "t-a", "hash-a", 25); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	got, err := repo.HistoryLimit(context.Background(), "A")
	if err != nil {
		t.Fatalf("HistoryLimit: %v", err)
	}
	if got != 25 {
		t.Errorf("HistoryLimit = %d, quero 25", got)
	}
}

// Usuario inexistente le 0 — desabilitado. Fail-CLOSED: um erro devolvido aqui
// viraria 500, e um default > 0 abriria o historico para um id que nao existe.
func TestChatHistoryRepositoryHistoryLimit_MissingUserReadsAsDisabled(t *testing.T) {
	repo := NewChatHistoryRepository(newRepoDB(t))

	got, err := repo.HistoryLimit(context.Background(), "nao-existe")
	if err != nil {
		t.Fatalf("HistoryLimit: %v", err)
	}
	if got != 0 {
		t.Errorf("HistoryLimit = %d, quero 0 (desabilitado)", got)
	}
}

func ids(msgs []appport.ChatHistoryMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.MessageID)
	}
	return out
}

func chaves(m map[string][]appport.ChatIndexEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

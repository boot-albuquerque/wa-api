package db

import (
	"testing"
	"time"
)

// A migração 13 dá coluna própria ao pushName (F84). Antes ele só existia
// dentro de `datajson`, e a lista de conversas teria de desserializar um JSON
// por linha para lê-lo — sobre 40 mil mensagens, num caminho de leitura.

// TestMigracao13_ColunaExiste roda contra o schema REAL, pela mesma via dos
// demais testes deste pacote. Um teste que criasse a tabela por conta própria
// combinaria com o próprio erro em vez de expô-lo — foi assim que a F71
// passou despercebida.
func TestMigracao13_ColunaSenderPushNameExiste(t *testing.T) {
	db := newHistoryDB(t)

	var existe int
	err := db.Get(&existe,
		`SELECT COUNT(*) FROM pragma_table_info('message_history') WHERE name = 'sender_push_name'`)
	if err != nil {
		t.Fatalf("consultar pragma_table_info: %v", err)
	}
	if existe != 1 {
		t.Fatalf("coluna sender_push_name ausente apos as migracoes (%d)", existe)
	}
}

// TestSaveMessageToHistory_PersisteOPushName: gravar e ler de volta. Sem
// isto, a coluna poderia existir e nunca receber valor — que é exatamente o
// estado em que a F84 nos deixou por meses, só que dentro do datajson.
func TestSaveMessageToHistory_PersisteOPushName(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", "Maria do Protobuf"); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	var got string
	if err := db.Get(&got, `SELECT sender_push_name FROM message_history WHERE message_id = 'M1'`); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if got != "Maria do Protobuf" {
		t.Errorf("sender_push_name = %q, quero o gravado", got)
	}
}

// TestGetChatPushNames_PegaOMaisRecente: pushName é o nome que a PESSOA
// escolheu para si, e ela pode tê-lo mudado. O último que ela usou é o que
// vale.
func TestGetChatPushNames_PegaOMaisRecente(t *testing.T) {
	db := newHistoryDB(t)
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

	for i, nome := range []string{"Nome Antigo", "Nome Novo"} {
		_, err := db.Exec(db.Rebind(
			`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, sender_push_name)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`),
			"u1", "111@lid", "111@lid", "M"+string(rune('1'+i)),
			base.Add(time.Duration(i)*time.Hour), "text", nome)
		if err != nil {
			t.Fatalf("inserir: %v", err)
		}
	}

	got, err := GetChatPushNamesByUser(db, "u1")
	if err != nil {
		t.Fatalf("GetChatPushNamesByUser: %v", err)
	}
	if got["111@lid"] != "Nome Novo" {
		t.Errorf("pushName = %q, quero o mais recente", got["111@lid"])
	}
}

// TestGetChatPushNames_MensagemSemNomeNaoApagaONome é o caso que a consulta
// tem de acertar: uma mensagem RECENTE sem pushName não pode fazer o chat
// perder o nome que chegou antes. Sem o filtro na consulta, o ROW_NUMBER
// escolheria a linha recente e vazia.
func TestGetChatPushNames_MensagemSemNomeNaoApagaONome(t *testing.T) {
	db := newHistoryDB(t)
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

	linhas := []struct {
		id, nome string
		quando   time.Time
	}{
		{"M1", "Maria", base},
		{"M2", "", base.Add(time.Hour)}, // mais recente, sem nome
	}
	for _, l := range linhas {
		_, err := db.Exec(db.Rebind(
			`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, sender_push_name)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`),
			"u1", "111@lid", "111@lid", l.id, l.quando, "text", l.nome)
		if err != nil {
			t.Fatalf("inserir %s: %v", l.id, err)
		}
	}

	got, err := GetChatPushNamesByUser(db, "u1")
	if err != nil {
		t.Fatalf("GetChatPushNamesByUser: %v", err)
	}
	if got["111@lid"] != "Maria" {
		t.Errorf("pushName = %q; a mensagem recente sem nome apagou o nome anterior", got["111@lid"])
	}
}

// TestGetChatPushNames_SemNomeNenhumNaoEntra: chat cujo remetente nunca
// mandou pushName fica FORA do mapa, e não com string vazia — o caller
// distingue "não sei" de "sei que é vazio".
func TestGetChatPushNames_SemNomeNenhumNaoEntra(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "222@lid", "222@lid", "M9",
		"text", "oi", "", "", "{}", ""); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	got, err := GetChatPushNamesByUser(db, "u1")
	if err != nil {
		t.Fatalf("GetChatPushNamesByUser: %v", err)
	}
	if _, presente := got["222@lid"]; presente {
		t.Errorf("chat sem pushName entrou no mapa: %+v", got)
	}
}

// --- F84: o re-sync tem de CURAR as linhas gravadas sem nome -------------

// TestSaveMessageToHistory_ResyncPreencheONomeQueFaltava é o teste que
// impede o defeito de ser irrecuperável.
//
// A cláusula era ON CONFLICT DO NOTHING. Como um HistorySync novo traz as
// MESMAS message_id, o insert inteiro era descartado e as linhas gravadas
// sem nome (todas, até a F84) ficariam sem nome para sempre — nenhuma
// instalação existente se curaria ao atualizar.
func TestSaveMessageToHistory_ResyncPreencheONomeQueFaltava(t *testing.T) {
	db := newHistoryDB(t)

	// Primeira gravação: como o defeito da F84 deixou o banco.
	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", ""); err != nil {
		t.Fatalf("gravar sem nome: %v", err)
	}
	// Re-sync: mesma message_id, agora COM o nome do protobuf.
	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", "Maria Recuperada"); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	var got string
	if err := db.Get(&got, `SELECT sender_push_name FROM message_history WHERE message_id='M1'`); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if got != "Maria Recuperada" {
		t.Errorf("sender_push_name = %q; o re-sync nao curou a linha sem nome", got)
	}
}

// TestSaveMessageToHistory_ResyncNaoApagaNomeComVazio é a guarda da Evolution
// API (issue #2426): uma mensagem que volte SEM pushName não pode zerar o
// nome que já está gravado.
func TestSaveMessageToHistory_ResyncNaoApagaNomeComVazio(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", "Maria"); err != nil {
		t.Fatalf("gravar com nome: %v", err)
	}
	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", ""); err != nil {
		t.Fatalf("re-sync sem nome: %v", err)
	}

	var got string
	if err := db.Get(&got, `SELECT sender_push_name FROM message_history WHERE message_id='M1'`); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if got != "Maria" {
		t.Errorf("sender_push_name = %q; o vazio apagou um nome que existia", got)
	}
}

// TestSaveMessageToHistory_ResyncNaoSobrescreveNomeExistente: só PREENCHER o
// que falta. Sem esta metade da guarda, um re-sync de histórico ANTIGO
// rebaixaria um nome mais recente.
func TestSaveMessageToHistory_ResyncNaoSobrescreveNomeExistente(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", "Nome Atual"); err != nil {
		t.Fatalf("gravar: %v", err)
	}
	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "oi", "", "", "{}", "Nome Antigo"); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	var got string
	if err := db.Get(&got, `SELECT sender_push_name FROM message_history WHERE message_id='M1'`); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if got != "Nome Atual" {
		t.Errorf("sender_push_name = %q; o re-sync rebaixou um nome que ja existia", got)
	}
}

// TestSaveMessageToHistory_ResyncNaoTocaOutrasColunas: a idempotência do
// resto do insert é o motivo de #292 ter posto o ON CONFLICT aqui, e a F84
// não pode tê-la desfeito.
func TestSaveMessageToHistory_ResyncNaoTocaOutrasColunas(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "texto original", "", "", "{}", ""); err != nil {
		t.Fatalf("gravar: %v", err)
	}
	if err := SaveMessageToHistory(db, "u1", "111@lid", "111@lid", "M1",
		"text", "TEXTO ADULTERADO", "", "", "{}", "Maria"); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	var texto string
	if err := db.Get(&texto, `SELECT text_content FROM message_history WHERE message_id='M1'`); err != nil {
		t.Fatalf("ler: %v", err)
	}
	if texto != "texto original" {
		t.Errorf("text_content = %q; o re-sync passou a sobrescrever colunas alem do nome", texto)
	}
}

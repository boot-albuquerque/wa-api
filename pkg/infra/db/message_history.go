package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SaveMessageToHistory inserts a message into the message_history table.
// The insert is idempotent — duplicate (user_id, message_id) pairs are
// silently skipped via ON CONFLICT DO NOTHING (see #292).
//
// Moved from db_methods.go as part of Clean Architecture migration.
// senderPushName entra no FIM da lista, e não ao lado de senderJID onde
// leria melhor, porque esta função já recebe dez strings posicionais:
// inserir no meio deslocaria todos os argumentos seguintes de todos os
// chamadores, e uma transposição entre `textContent`, `mediaLink` e
// `quotedMessageID` compila em silêncio. No fim, o único argumento que pode
// estar errado é o novo.
func SaveMessageToHistory(db *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJson, senderPushName string) error {
	// ON CONFLICT preenche o nome que falta, e SÓ ele (F84).
	//
	// A cláusula era DO NOTHING, e isso tornava o defeito da F84
	// irrecuperável: um HistorySync novo traz as MESMAS message_id, o insert
	// inteiro era descartado, e as linhas gravadas sem nome ficavam sem nome
	// para sempre. Nenhuma instalação existente se curaria ao atualizar.
	//
	// A guarda é dupla, e as duas metades importam:
	//   - `EXCLUDED.sender_push_name <> ''` — nunca apagar um nome com vazio.
	//     É a lição da Evolution API (issue #2426), onde a falta dessa guarda
	//     zerava o pushName a cada mensagem enviada.
	//   - `message_history.sender_push_name IS NULL OR = ''` — só PREENCHER o
	//     que falta, nunca sobrescrever o que já existe. Sem ela, um
	//     re-sync antigo poderia rebaixar um nome mais recente.
	//
	// Nenhuma outra coluna é tocada: a idempotência do resto do insert
	// continua valendo, que é o motivo de #292 ter posto o ON CONFLICT aqui.
	query := db.Rebind(`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, quoted_message_id, datajson, sender_push_name)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
              ON CONFLICT (user_id, message_id) DO UPDATE
                 SET sender_push_name = EXCLUDED.sender_push_name
               WHERE EXCLUDED.sender_push_name <> ''
                 AND (message_history.sender_push_name IS NULL
                      OR message_history.sender_push_name = '')`)
	_, err := db.Exec(query, userID, chatJID, senderJID, messageID, time.Now(), messageType, textContent, mediaLink, quotedMessageID, dataJson, senderPushName)
	if err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Str("chat_jid", chatJID).Str("message_id", messageID).
			Msg("failed to save message to history")
		return fmt.Errorf("failed to save message to history: %w", err)
	}
	return nil
}

// flexTimeScan escaneia MAX(timestamp) — uma coluna COMPUTADA, que perde o
// "declared type" DATETIME da coluna original — de qualquer um dos dois
// formatos que os dois backends devolvem para uma expressão agregada:
// SQLite (driver modernc.org/sqlite) devolve TEXT (RFC3339); Postgres devolve
// time.Time nativo. `valid=false` (sem erro) para NULL/formato inesperado —
// o caller decide se isso é fatal; aqui é só "sem sinal pra este chat".
type flexTimeScan struct {
	t     time.Time
	valid bool
}

func (f *flexTimeScan) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		return nil
	case time.Time:
		f.t, f.valid = v, true
		return nil
	case string:
		t, ok := parseFlexTimestamp(v)
		f.t, f.valid = t, ok
		return nil
	case []byte:
		t, ok := parseFlexTimestamp(string(v))
		f.t, f.valid = t, ok
		return nil
	default:
		return fmt.Errorf("flexTimeScan: unsupported source type %T", src)
	}
}

// goStringerTimeLayout é o layout que time.Time.String() produz (sem a parte
// monotônica, tratada à parte abaixo) — o formato que o driver sqlite
// (modernc.org/sqlite) devolve pra uma coluna DATETIME agregada via MAX():
// a agregação perde o "declared type" da coluna original, e o driver cai no
// Stringer em vez de preservar RFC3339 (que é o formato de uma coluna NÃO
// agregada, ver TestGetLastActivityByUser_*).
const goStringerTimeLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// parseFlexTimestamp tenta RFC3339 primeiro (formato "normal" de uma coluna
// não agregada) e cai pro layout Stringer do Go — descartando o sufixo
// " m=±..." de clock monotônico que time.Now() carrega e que nenhum layout
// consegue re-parsear (não é um formato de calendário, é um offset interno
// do runtime). Devolve ok=false pra qualquer formato não reconhecido — vira
// "sem sinal pra este chat", não erro fatal do batch inteiro.
func parseFlexTimestamp(raw string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, true
	}
	clean := raw
	if i := strings.Index(clean, " m="); i >= 0 {
		clean = clean[:i]
	}
	if t, err := time.Parse(goStringerTimeLayout, clean); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// GetLastActivityByUser devolve, por chat_jid, o timestamp da mensagem mais
// recente já persistida em message_history (INCLUINDO o backfill do
// HistorySync pós-pareamento, que roda automaticamente e sem custo de
// polling — ver eventhandler_history.go). É a única fonte de "última
// conversa por contato" disponível hoje: GetAllContacts (noise_contacts)
// não carrega nenhum timestamp, só identidade/nome (ver ADR-0001 do
// disparazaap, seção "Limitação conhecida"). Grupos (chat_jid @g.us) e
// broadcasts ficam incluídos no resultado — filtragem é responsabilidade do
// caller (mesmo padrão denylist usado no restante do pipeline de contatos).
func GetLastActivityByUser(db *sqlx.DB, userID string) (map[string]time.Time, error) {
	query := db.Rebind(`SELECT chat_jid, MAX(timestamp) AS last_ts
	                       FROM message_history
	                      WHERE user_id = ?
	                      GROUP BY chat_jid`)
	type row struct {
		ChatJID string       `db:"chat_jid"`
		LastTS  flexTimeScan `db:"last_ts"`
	}
	var rows []row
	if err := db.Select(&rows, query, userID); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Msg("failed to get last activity by chat")
		return nil, fmt.Errorf("failed to get last activity by chat: %w", err)
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		if !r.LastTS.valid {
			log.Warn().Str("chat_jid", r.ChatJID).
				Msg("failed to parse last activity timestamp, skipping chat")
			continue
		}
		out[r.ChatJID] = r.LastTS.t
	}
	return out, nil
}

// TrimMessageHistory removes the oldest messages beyond the given limit for a
// (user_id, chat_jid) pair, pruning the corresponding secrets from storeDB
// (the noise database where wanoise_message_secrets lives).
//
// The three steps are ordered so that a failure in the secrets purge (step 2)
// never blocks the history purge (step 3). The previous version ran both
// against the same handle, which always failed because the two tables live in
// different databases — and then returned on the error, leaving history
// untrimmed forever (F212).
func TrimMessageHistory(db *sqlx.DB, storeDB *sqlx.DB, userID, chatJID string, limit int) error {
	// Step 1: collect the message_ids that will be trimmed.
	var selectIDs string
	if db.DriverName() == "postgres" {
		selectIDs = `SELECT message_id FROM message_history
		             WHERE user_id = $1 AND chat_jid = $2
		             ORDER BY timestamp DESC OFFSET $3`
	} else {
		selectIDs = `SELECT message_id FROM message_history
		             WHERE user_id = ? AND chat_jid = ?
		             ORDER BY timestamp DESC LIMIT -1 OFFSET ?`
	}

	var ids []string
	if err := db.Select(&ids, selectIDs, userID, chatJID, limit); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Str("chat_jid", chatJID).Int("limit", limit).
			Msg("failed to select message ids for trim")
		return fmt.Errorf("failed to select message ids for trim: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	// Step 2: purge secrets from the store database.
	// Failure here is logged but MUST NOT prevent the history purge.
	if storeDB != nil {
		trimMessageSecrets(storeDB, ids, userID, chatJID)
	}

	// Step 3: purge history rows from the application database.
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	deleteHistory := `DELETE FROM message_history
	                   WHERE user_id = ? AND chat_jid = ? AND message_id IN (` +
		strings.Join(placeholders, ",") + `)`
	args = append([]interface{}{userID, chatJID}, args...)

	if _, err := db.Exec(db.Rebind(deleteHistory), args...); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Str("chat_jid", chatJID).Int("limit", limit).
			Msg("failed to trim message history")
		return fmt.Errorf("failed to trim message history: %w", err)
	}

	return nil
}

func trimMessageSecrets(storeDB *sqlx.DB, ids []string, userID, chatJID string) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `DELETE FROM wanoise_message_secrets WHERE message_id IN (` +
		strings.Join(placeholders, ",") + `)`

	if _, err := storeDB.Exec(storeDB.Rebind(q), args...); err != nil {
		log.Error().Err(err).Str("table", "wanoise_message_secrets").
			Str("user_id", userID).Str("chat_jid", chatJID).
			Msg("failed to trim message secrets (non-fatal, history trim continues)")
	}
}

// GetChatPushNamesByUser devolve, por chat_jid, o pushName mais RECENTE que
// já chegou naquela conversa.
//
// É a terceira fonte de nome da lista de conversas, e na prática a mais
// completa: o WhatsApp manda o pushName junto de cada mensagem, enquanto o
// roster local só conhece quem está na agenda — e para identidades `@lid`
// está vazio na maioria dos casos (F84).
//
// "Mais recente" e não "qualquer um" porque pushName é o nome que a PESSOA
// escolheu para si e ela pode tê-lo mudado; o último que ela usou é o que
// vale. Linhas sem nome são filtradas na consulta, para que uma mensagem
// recente sem pushName não apague um nome que chegou antes.
func GetChatPushNamesByUser(db *sqlx.DB, userID string) (map[string]string, error) {
	query := db.Rebind(`
		SELECT chat_jid, sender_push_name FROM (
		    SELECT chat_jid, sender_push_name,
		           ROW_NUMBER() OVER (PARTITION BY chat_jid ORDER BY timestamp DESC) AS rn
		      FROM message_history
		     WHERE user_id = ?
		       AND sender_push_name IS NOT NULL
		       AND sender_push_name <> ''
		) AS recentes
		WHERE rn = 1`)

	type row struct {
		ChatJID  string `db:"chat_jid"`
		PushName string `db:"sender_push_name"`
	}
	var rows []row
	if err := db.Select(&rows, query, userID); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Msg("failed to get push names by chat")
		return nil, fmt.Errorf("failed to get push names by chat: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ChatJID] = r.PushName
	}
	return out, nil
}

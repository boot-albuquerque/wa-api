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
func SaveMessageToHistory(db *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJson string) error {
	query := db.Rebind(`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, quoted_message_id, datajson)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
              ON CONFLICT (user_id, message_id) DO NOTHING`)
	_, err := db.Exec(query, userID, chatJID, senderJID, messageID, time.Now(), messageType, textContent, mediaLink, quotedMessageID, dataJson)
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
// conversa por contato" disponível hoje: GetAllContacts (whatsmeow_contacts)
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

// TrimMessageHistory removes the oldest messages beyond the given limit for
// a (user_id, chat_jid) pair. Handles both PostgreSQL and SQLite drivers.
//
// Moved from db_methods.go as part of Clean Architecture migration.
func TrimMessageHistory(db *sqlx.DB, userID, chatJID string, limit int) error {
	var queryHistory, querySecrets string

	if db.DriverName() == "postgres" {
		queryHistory = `
	            DELETE FROM message_history
	            WHERE id IN (
	                SELECT id FROM message_history
	                WHERE user_id = $1 AND chat_jid = $2
	                ORDER BY timestamp DESC
	                OFFSET $3
	            )`

		querySecrets = `
	            DELETE FROM whatsmeow_message_secrets
	            WHERE message_id IN (
	                SELECT message_id FROM message_history
	                WHERE user_id = $1 AND chat_jid = $2
	                ORDER BY timestamp DESC
	                OFFSET $3
	            )`
	} else { // sqlite
		queryHistory = `
	            DELETE FROM message_history
	            WHERE id IN (
	                SELECT id FROM message_history
	                WHERE user_id = ? AND chat_jid = ?
	                ORDER BY timestamp DESC
	                LIMIT -1 OFFSET ?
	            )`

		querySecrets = `
	            DELETE FROM whatsmeow_message_secrets
	            WHERE message_id IN (
	                SELECT message_id FROM message_history
	                WHERE user_id = ? AND chat_jid = ?
	                ORDER BY timestamp DESC
	                LIMIT -1 OFFSET ?
	            )`
	}

	if _, err := db.Exec(querySecrets, userID, chatJID, limit); err != nil {
		log.Error().Err(err).Str("table", "whatsmeow_message_secrets").Str("user_id", userID).
			Str("chat_jid", chatJID).Int("limit", limit).
			Msg("failed to trim message secrets")
		return fmt.Errorf("failed to trim message secrets: %w", err)
	}

	if _, err := db.Exec(queryHistory, userID, chatJID, limit); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Str("chat_jid", chatJID).Int("limit", limit).
			Msg("failed to trim message history")
		return fmt.Errorf("failed to trim message history: %w", err)
	}

	return nil
}

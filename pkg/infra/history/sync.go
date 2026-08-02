package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// historySender e' o recorte de *whatsmeow.Client que SyncHistoryForChat usa.
// A costura existe para que o envio do pedido de sync possa ser exercitado sem
// socket; *whatsmeow.Client a satisfaz e continua sendo o unico implementador
// em producao. A assercao de tipo permanece de valor unico, entao o
// comportamento para um valor de tipo inesperado e' o mesmo de antes.
type historySender interface {
	BuildHistorySyncRequest(lastKnownMessageInfo *types.MessageInfo, count int) *waE2E.Message
	SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

// SyncDeps provides the external callbacks needed for history sync.
type SyncDeps struct {
	GetWA func(userID string) interface{} // returns *whatsmeow.Client
	GetMC func(userID string) interface{} // returns something with WAClient
}

// SaveMessageFunc saves a message to history.
type SaveMessageFunc func(db *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJSON string) error

// TrimMessageFunc trims message history.
type TrimMessageFunc func(db *sqlx.DB, userID, chatJID string, limit int) error

// SyncHistoryForChat builds and sends a WhatsApp history-sync request.
func SyncHistoryForChat(ctx context.Context, db *sqlx.DB, deps SyncDeps, userID string, chatJID types.JID, count int) error {
	chatJIDStr := chatJID.String()

	var query string
	if db.DriverName() == "postgres" {
		query = `SELECT message_id, chat_jid, sender_jid FROM message_history WHERE user_id = $1 AND chat_jid = $2 ORDER BY timestamp DESC LIMIT 1`
	} else {
		query = `SELECT message_id, chat_jid, sender_jid FROM message_history WHERE user_id = ? AND chat_jid = ? ORDER BY timestamp DESC LIMIT 1`
	}

	type lastMsgRow struct {
		MessageID string `db:"message_id"`
		ChatJID   string `db:"chat_jid"`
		SenderJID string `db:"sender_jid"`
	}
	var lastMsg lastMsgRow

	var lastMessageInfo *types.MessageInfo
	err := db.Get(&lastMsg, query, userID, chatJIDStr)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to get last message from history: %w", err)
	}

	if err == nil && lastMsg.MessageID != "" {
		var senderJID types.JID
		if lastMsg.SenderJID != "" && lastMsg.SenderJID != "me" {
			senderJID, _ = types.ParseJID(lastMsg.SenderJID)
		}
		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chatJID, Sender: senderJID, IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer},
			ID:            lastMsg.MessageID,
		}
	} else {
		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chatJID, IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer},
		}
	}

	waClient := deps.GetWA(userID).(historySender)
	historyMsg := waClient.BuildHistorySyncRequest(lastMessageInfo, count)
	if historyMsg == nil {
		return errors.New("failed to build history sync request")
	}

	myClient := deps.GetMC(userID)
	type waClientGetter interface {
		GetWAClient() *whatsmeow.Client
	}
	waclient := myClient.(waClientGetter).GetWAClient()
	if waclient == nil || waclient.Store == nil || waclient.Store.ID == nil {
		return errors.New("client store not available")
	}

	_, err = waClient.SendMessage(ctx, waclient.Store.ID.ToNonAD(), historyMsg, whatsmeow.SendRequestExtra{Peer: true})
	if err != nil {
		log.Error().Str("userID", userID).Str("chatJID", chatJIDStr).Err(err).Msg("Failed to send WhatsApp history sync request")
		return fmt.Errorf("failed to send history sync request: %w", err)
	}

	log.Info().Str("userID", userID).Str("chatJID", chatJIDStr).Int("count", count).Msg("WhatsApp history sync request sent successfully")
	return nil
}

// SaveOutgoingMessageToHistory persists a sent message and trims history.
func SaveOutgoingMessageToHistory(db *sqlx.DB, saveFn SaveMessageFunc, trimFn TrimMessageFunc, userID, chatJID, messageID, messageType, textContent, mediaLink string, historyLimit int) {
	if historyLimit <= 0 {
		return
	}
	err := saveFn(db, userID, chatJID, "me", messageID, messageType, textContent, mediaLink, "", "")
	if err != nil {
		log.Error().Err(err).Msg("Failed to save outgoing message to history")
	} else {
		err = trimFn(db, userID, chatJID, historyLimit)
		if err != nil {
			log.Error().Err(err).Msg("Failed to trim message history")
		}
	}
}

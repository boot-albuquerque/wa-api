package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// syncHistoryForChat builds and sends a WhatsApp history-sync request for a
// specific chat, using the last known message from the message_history table
// as the pagination cursor.
func syncHistoryForChat(ctx context.Context, db *sqlx.DB, userID string, chatJID types.JID, count int) error {
	chatJIDStr := chatJID.String()

	// Try to get last message info for this chat from database
	var query string
	if db.DriverName() == "postgres" {
		query = `
			SELECT message_id, chat_jid, sender_jid
			FROM message_history
			WHERE user_id = $1 AND chat_jid = $2
			ORDER BY timestamp DESC
			LIMIT 1`
	} else {
		query = `
			SELECT message_id, chat_jid, sender_jid
			FROM message_history
			WHERE user_id = ? AND chat_jid = ?
			ORDER BY timestamp DESC
			LIMIT 1`
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
			var pErr error
			senderJID, pErr = types.ParseJID(lastMsg.SenderJID)
			if pErr != nil {
				log.Warn().Err(pErr).Str("senderJID", lastMsg.SenderJID).Msg("Failed to parse sender JID from history, using empty JID")
				senderJID = types.EmptyJID
			}
		} else {
			senderJID = types.EmptyJID
		}

		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    chatJID,
				Sender:  senderJID,
				IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer,
			},
			ID: lastMsg.MessageID,
		}
	} else {
		lastMessageInfo = &types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    chatJID,
				IsGroup: chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer,
			},
		}
	}

	historyMsg := clientManager.GetWhatsmeowClient(userID).BuildHistorySyncRequest(lastMessageInfo, count)
	if historyMsg == nil {
		return errors.New("failed to build history sync request")
	}

	myClient := clientManager.GetMyClient(userID)
	if myClient == nil || myClient.WAClient == nil || myClient.WAClient.Store == nil || myClient.WAClient.Store.ID == nil {
		return errors.New("client store not available")
	}

	_, err = clientManager.GetWhatsmeowClient(userID).SendMessage(
		ctx,
		myClient.WAClient.Store.ID.ToNonAD(),
		historyMsg,
		whatsmeow.SendRequestExtra{Peer: true},
	)

	if err != nil {
		log.Error().
			Str("userID", userID).
			Str("chatJID", chatJIDStr).
			Err(err).
			Msg("Failed to send WhatsApp history sync request")
		return fmt.Errorf("failed to send history sync request: %w", err)
	}

	log.Info().
		Str("userID", userID).
		Str("chatJID", chatJIDStr).
		Int("count", count).
		Msg("WhatsApp history sync request sent successfully")

	return nil
}

// saveOutgoingMessageToHistory persists a sent message to the history table,
// then trims it to the configured limit.
func saveOutgoingMessageToHistory(db *sqlx.DB, userID, chatJID, messageID, messageType, textContent, mediaLink string, historyLimit int) {
	if historyLimit > 0 {
		err := saveMessageToHistory(db, userID, chatJID, "me", messageID, messageType, textContent, mediaLink, "", "")
		if err != nil {
			log.Error().Err(err).Msg("Failed to save outgoing message to history")
		} else {
			err = trimMessageHistory(db, userID, chatJID, historyLimit)
			if err != nil {
				log.Error().Err(err).Msg("Failed to trim message history")
			}
		}
	}
}

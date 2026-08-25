package history

import (
	"encoding/json"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SaveFunc inserts a message into the message_history table.
type SaveFunc func(db *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJson, senderPushName string) error

// TrimFunc removes the oldest messages beyond the limit for a (user_id, chat_jid) pair.
type TrimFunc func(db *sqlx.DB, storeDB *sqlx.DB, userID, chatJID string, limit int) error

// HistoryLimitFunc returns the history limit for a user. Zero means disabled.
type HistoryLimitFunc func(userID string) int

// OutgoingRecorder persists messages sent by the API into message_history so
// they are available for CAP-55 (forward by key) and F228 (poll sender
// resolution). It is called by the adapter AFTER a successful send — a
// persistence failure is logged, never propagated to the caller, because the
// message already reached WhatsApp.
type OutgoingRecorder struct {
	db      *sqlx.DB
	storeDB *sqlx.DB
	save    SaveFunc
	trim    TrimFunc
	limitFn HistoryLimitFunc
}

// NewOutgoingRecorder creates a recorder.
func NewOutgoingRecorder(db, storeDB *sqlx.DB, save SaveFunc, trim TrimFunc, limitFn HistoryLimitFunc) *OutgoingRecorder {
	return &OutgoingRecorder{
		db:      db,
		storeDB: storeDB,
		save:    save,
		trim:    trim,
		limitFn: limitFn,
	}
}

// Record persists a sent message to history. It builds the datajson envelope
// (events.Message shape) from the proto and metadata, then saves and trims.
//
// senderJID is the user's own JID as a string — the wire form that
// resolvePollSender (F228) needs to read back. For outgoing messages this is
// the user's LID, which the adapter obtains from client.Store().
func (r *OutgoingRecorder) Record(
	userID string,
	chatJID string,
	senderJID string,
	messageID string,
	messageType string,
	textContent string,
	msg *waE2E.Message,
	ts time.Time,
) {
	limit := r.limitFn(userID)
	if limit <= 0 {
		return
	}

	dataJSON := buildOutgoingDataJSON(chatJID, senderJID, messageID, messageType, msg, ts)

	err := r.save(r.db, userID, chatJID, senderJID, messageID, messageType, textContent, "", "", dataJSON, "")
	if err != nil {
		log.Error().Err(err).
			Str("user_id", userID).
			Str("message_id", messageID).
			Msg("failed to persist outgoing message to history")
		return
	}

	if err := r.trim(r.db, r.storeDB, userID, chatJID, limit); err != nil {
		log.Error().Err(err).
			Str("user_id", userID).
			Str("chat_jid", chatJID).
			Int("limit", limit).
			Msg("failed to trim history after outgoing message")
	}
}

// buildOutgoingDataJSON constructs the JSON blob that the forward path
// (SendForwardedMessage) and poll-sender path (GetPollSenderJID) expect.
// The shape matches events.Message: Info + Message + Is* flags.
func buildOutgoingDataJSON(chatJID, senderJID, messageID, messageType string, msg *waE2E.Message, ts time.Time) string {
	chat, _ := types.ParseJID(chatJID)
	sender, _ := types.ParseJID(senderJID)

	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: true,
			IsGroup:  chat.Server == types.GroupServer || chat.Server == types.BroadcastServer,
		},
		ID:        types.MessageID(messageID),
		Timestamp: ts,
		Type:      messageType,
	}

	envelope := map[string]interface{}{
		"Info":                  info,
		"Message":               msg,
		"IsEphemeral":           false,
		"IsViewOnce":            false,
		"IsViewOnceV2":          false,
		"IsViewOnceV2Extension": false,
		"IsDocumentWithCaption": false,
		"IsLottieSticker":       false,
		"IsBotInvoke":           false,
		"IsEdit":                false,
		"SourceWebMsg":          nil,
		"UnavailableRequestID":  "",
		"RetryCount":            0,
		"NewsletterMeta":        nil,
		"RawMessage":            msg,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal outgoing message envelope")
		return "{}"
	}
	return string(data)
}

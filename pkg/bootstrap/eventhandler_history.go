package bootstrap

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de sincronização de histórico: o blob de conversas antigas que o
// telefone envia depois do pareamento, e os avisos de fim de sincronização
// offline.

func (mycli *MyClient) handleHistorySync(evt *events.HistorySync, st *eventState) {
	st.postmap["type"] = "HistorySync"
	st.dowebhook = 1

	// Save HistorySync messages to message_history table
	if evt.Data != nil && evt.Data.Conversations != nil {
		go mycli.persistHistorySync(evt.Data.Conversations)
	}
}

// persistHistorySync era o corpo da goroutine anônima do ramo HistorySync.
// Continua rodando em goroutine própria — quem a chama é
// `go mycli.persistHistorySync(...)` — e a única coisa que a closure
// capturava além de mycli, evt.Data.Conversations, virou parâmetro.
func (mycli *MyClient) persistHistorySync(conversations []*waHistorySync.Conversation) {
	// Get the account owner's JID for messages sent by the instance
	accountOwnerJID := ""
	if mycli.WAClient.Store != nil && mycli.WAClient.Store.ID != nil {
		accountOwnerJID = mycli.WAClient.Store.ID.ToNonAD().String()
	}

	savedCount := 0
	for _, conv := range conversations {
		savedCount += mycli.persistHistorySyncConversation(conv, accountOwnerJID)
	}

	if savedCount > 0 {
		log.Info().
			Str("userID", mycli.UserID).
			Int("savedCount", savedCount).
			Msg("Saved HistorySync messages to message_history")
	}
}

// persistHistorySyncConversation era o corpo do laço externo `for _, conv :=
// range conversations`. Devolve quantas mensagens daquela conversa foram
// gravadas — os dois `continue` do corpo original viraram `return 0`, que é o
// mesmo que não somar nada ao total.
func (mycli *MyClient) persistHistorySyncConversation(conv *waHistorySync.Conversation, accountOwnerJID string) int {
	if conv == nil || conv.ID == nil || conv.Messages == nil {
		return 0
	}

	chatJID, err := types.ParseJID(*conv.ID)
	if err != nil {
		log.Warn().Err(err).Str("convID", *conv.ID).Msg("Failed to parse conversation JID in HistorySync")
		return 0
	}

	saved := 0
	for _, msg := range conv.Messages {
		if mycli.persistHistorySyncMessage(chatJID, accountOwnerJID, msg) {
			saved++
		}
	}
	return saved
}

// persistHistorySyncMessage era o corpo do laço interno `for _, msg := range
// conv.Messages`. Devolve true exatamente quando o corpo original executava
// `savedCount++`; todos os `continue` viraram `return false`, e a saída normal
// do laço sem gravação também.
func (mycli *MyClient) persistHistorySyncMessage(chatJID types.JID, accountOwnerJID string, msg *waHistorySync.HistorySyncMsg) bool {
	if msg == nil || msg.Message == nil {
		return false
	}

	// Extract message data
	messageKey := msg.Message.GetKey()
	if messageKey == nil {
		return false
	}

	messageID := messageKey.GetID()
	if messageID == "" {
		return false
	}

	// Determine sender - never use "me", always use actual JID
	// Use GetFromMe() from MessageKey to determine if message is from account owner
	// This is more reliable than checking GetParticipant()
	isFromMe := messageKey.GetFromMe()
	var senderJID string

	if isFromMe {
		// Message from account owner
		senderJID = accountOwnerJID
		if senderJID == "" {
			// Fallback: use "me" if account owner JID is not available
			senderJID = "me"
			log.Warn().Str("messageID", messageID).Msg("accountOwnerJID is not available for a message from me, using 'me' as senderJID")
		}
	} else {
		// Message from someone else
		participantJID := messageKey.GetParticipant()
		if chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer {
			// Group message: use participant JID
			senderJID = participantJID
		} else {
			// Direct message: sender is the chat itself (chat_jid)
			senderJID = chatJID.String()
		}
	}

	// If senderJID is still empty, skip this message
	if senderJID == "" {
		log.Warn().Str("messageID", messageID).Msg("Cannot determine sender JID, skipping message")
		return false
	}

	// Get message content
	message := msg.Message.GetMessage()
	if message == nil {
		return false
	}

	// Extract message type and content
	messageType := "unknown"
	textContent := ""
	mediaLink := ""
	quotedMessageID := ""

	if message.GetConversation() != "" {
		messageType = "text"
		textContent = message.GetConversation()
	} else if ext := message.GetExtendedTextMessage(); ext != nil {
		messageType = "text"
		textContent = ext.GetText()
		if contextInfo := ext.GetContextInfo(); contextInfo != nil {
			quotedMessageID = contextInfo.GetStanzaID()
		}
	} else if img := message.GetImageMessage(); img != nil {
		messageType = "image"
		textContent = img.GetCaption()
	} else if vid := message.GetVideoMessage(); vid != nil {
		messageType = "video"
		textContent = vid.GetCaption()
	} else if audio := message.GetAudioMessage(); audio != nil {
		messageType = "audio"
	} else if doc := message.GetDocumentMessage(); doc != nil {
		messageType = "document"
		textContent = doc.GetCaption()
	} else if sticker := message.GetStickerMessage(); sticker != nil {
		messageType = "sticker"
	} else if location := message.GetLocationMessage(); location != nil {
		messageType = "location"
		textContent = location.GetName()
	} else if contact := message.GetContactMessage(); contact != nil {
		messageType = "contact"
		textContent = contact.GetDisplayName()
	} else if buttons := message.GetButtonsResponseMessage(); buttons != nil {
		messageType = "buttons_response"
		textContent = buttons.GetSelectedButtonID()
	} else if list := message.GetListResponseMessage(); list != nil {
		messageType = "list_response"
		textContent = list.GetSingleSelectReply().GetSelectedRowID()
	} else if reaction := message.GetReactionMessage(); reaction != nil {
		messageType = "reaction"
		textContent = reaction.GetText()
		if key := reaction.GetKey(); key != nil {
			quotedMessageID = key.GetID()
		}
	}

	// Set default text for media messages without captions
	if textContent == "" && messageType != "text" && messageType != "reaction" && messageType != "delete" {
		switch messageType {
		case "image":
			textContent = ":image:"
		case "video":
			textContent = ":video:"
		case "audio":
			textContent = ":audio:"
		case "document":
			textContent = ":document:"
		case "sticker":
			textContent = ":sticker:"
		case "contact":
			textContent = ":contact:"
		case "location":
			textContent = ":location:"
		}
	}

	// Get message timestamp
	msgTimestamp := time.Now()
	if timestamp := msg.Message.GetMessageTimestamp(); timestamp > 0 {
		msgTimestamp = time.Unix(int64(timestamp), 0)
	}

	// Parse sender JID for MessageInfo
	var senderJIDForInfo types.JID
	if isFromMe {
		if accountOwnerJID != "" {
			var pErr error
			senderJIDForInfo, pErr = types.ParseJID(accountOwnerJID)
			if pErr != nil {
				log.Warn().Err(pErr).Str("accountOwnerJID", accountOwnerJID).Msg("Failed to parse account owner JID in HistorySync")
			}
		}
	} else {
		if chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer {
			// Group: use participant JID
			participant := messageKey.GetParticipant()
			if participant != "" {
				var pErr error
				senderJIDForInfo, pErr = types.ParseJID(participant)
				if pErr != nil {
					log.Warn().Err(pErr).Str("participantJID", participant).Msg("Failed to parse participant JID in HistorySync")
				}
			}
		} else {
			// Direct message: sender is the chat
			senderJIDForInfo = chatJID
		}
	}

	// Try to get PushName from store if available
	pushName := ""
	if !isFromMe && senderJIDForInfo.User != "" {
		if mycli.WAClient != nil && mycli.WAClient.Store != nil {
			if contact, err := mycli.WAClient.Store.Contacts.GetContact(context.Background(), senderJIDForInfo); err == nil {
				pushName = contact.PushName
			}
		}
	}

	// Create MessageInfo structure matching events.Message format
	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chatJID,
			Sender:   senderJIDForInfo,
			IsFromMe: isFromMe,
			IsGroup:  chatJID.Server == types.GroupServer || chatJID.Server == types.BroadcastServer,
		},
		ID:        messageID,
		Timestamp: msgTimestamp,
		Type:      messageType,
		PushName:  pushName,
	}

	// Create events.Message-like structure for datajson
	// This matches the format used in regular message events
	// RawMessage should be the full waE2E.Message structure
	messageEvent := map[string]interface{}{
		"Info":                  messageInfo,
		"Message":               message,
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
		"RawMessage":            msg.Message,
	}

	// Serialize to JSON for datajson field
	evtJSON, err := json.Marshal(messageEvent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal HistorySync message event to JSON")
		evtJSON = []byte("{}")
	}

	// Save message to history
	// Only save if there's meaningful content
	if textContent != "" || mediaLink != "" || (messageType != "text" && messageType != "reaction") {
		err = saveMessageToHistory(mycli.DB,
			mycli.UserID,
			chatJID.String(),
			senderJID,
			messageID,
			messageType,
			textContent,
			mediaLink,
			quotedMessageID,
			string(evtJSON),
		)
		if err != nil {
			log.Error().Err(err).
				Str("userID", mycli.UserID).
				Str("chatJID", chatJID.String()).
				Str("messageID", messageID).
				Msg("Failed to save HistorySync message to history")
		} else {
			return true
		}
	}
	return false
}

func (mycli *MyClient) handleOfflineSyncCompleted(evt *events.OfflineSyncCompleted, st *eventState) {
	st.postmap["type"] = "OfflineSyncCompleted"
	st.dowebhook = 1
	log.Info().Msg("Offline sync completed")
}

func (mycli *MyClient) handleOfflineSyncPreview(evt *events.OfflineSyncPreview, st *eventState) {
	st.postmap["type"] = "OfflineSyncPreview"
	st.dowebhook = 1
	log.Info().Msg("Offline sync preview")
}

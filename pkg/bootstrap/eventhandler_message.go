package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de mensagem: a mensagem em si e os avisos sobre mensagens
// (recibo de leitura/entrega, mensagem indecifrável, retentativa de mídia,
// mensagem de Facebook).

// messageS3Config era um struct anônimo declarado no topo do ramo
// events.Message. Virou tipo nomeado porque agora atravessa uma fronteira de
// função; as tags `db` são as mesmas, e é o que sqlx preenche.
type messageS3Config struct {
	Enabled       string `db:"s3_enabled"`
	MediaDelivery string `db:"media_delivery"`
}

func (mycli *MyClient) handleMessage(evt *events.Message, st *eventState) {
	appCtx.LastMessageCache.Set(mycli.UserID, &evt.Info, cache.DefaultExpiration)

	s3Config := mycli.resolveMessageS3Config(st.txtid)

	// Lazy init S3 client if needed (handles reconnect-after-restart when connectOnStartup skipped this user)
	if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
		ensureS3ClientForUser(st.txtid)
	}

	st.postmap["type"] = "Message"
	st.dowebhook = 1

	logMessageReceived(evt)
	mycli.decoratePollVote(evt, st)
	mycli.decryptSecretEncryptedMessage(evt)

	if !*skipMedia {
		mycli.processMessageMedia(evt, s3Config, st)
	}

	// Save message to history regardless of skipMedia setting
	mycli.saveMessageHistory(evt, st)
}

// resolveMessageS3Config lê a configuração de S3 do cache de usuário e, se ela
// não estiver lá, do banco. O fallback em caso de erro de banco é o mesmo de
// antes: S3 desligado e entrega em base64.
func (mycli *MyClient) resolveMessageS3Config(txtid string) messageS3Config {
	var s3Config messageS3Config

	myuserinfo, found := appCtx.UserInfoCache.Get(mycli.Token)
	if !found {
		err := mycli.DB.Get(&s3Config, "SELECT CASE WHEN s3_enabled = 1 THEN 'true' ELSE 'false' END AS s3_enabled, media_delivery FROM users WHERE id = $1", txtid)
		if err != nil {
			log.Error().Err(err).Msg("onMessage Failed to get S3 config from DB as it was not on cache")
			s3Config.Enabled = "false"
			s3Config.MediaDelivery = "base64"
		}
	} else {
		s3Config.Enabled = myuserinfo.(Values).Get("S3Enabled")
		s3Config.MediaDelivery = myuserinfo.(Values).Get("MediaDelivery")
	}
	return s3Config
}

func logMessageReceived(evt *events.Message) {
	metaParts := []string{fmt.Sprintf("pushname: %s", evt.Info.PushName), fmt.Sprintf("timestamp: %s", evt.Info.Timestamp)}
	if evt.Info.Type != "" {
		metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
	}
	if evt.Info.Category != "" {
		metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
	}
	if evt.IsViewOnce {
		metaParts = append(metaParts, "view once")
	}
	if evt.IsViewOnce {
		metaParts = append(metaParts, "ephemeral")
	}

	log.Info().Str("id", evt.Info.ID).Str("source", evt.Info.SourceString()).Str("parts", strings.Join(metaParts, ", ")).Msg("Message Received")
}

// decoratePollVote: If this is a poll vote, decrypt the E2E-encrypted payload
// so the webhook can expose which options were selected. Votes arrive as
// SHA-256 hashes of the option text; we match those back to the plaintext
// options remembered at send time (see SendPoll in handlers.go). If the
// session was restarted between send and vote we cannot resolve plaintext;
// hashes are still emitted so the consumer can perform matching itself if it
// has stored options.
func (mycli *MyClient) decoratePollVote(evt *events.Message, st *eventState) {
	if evt.Message.GetPollUpdateMessage() == nil {
		return
	}
	pollMsgID := evt.Message.GetPollUpdateMessage().GetPollCreationMessageKey().GetID()

	pollVote, perr := mycli.WAClient.DecryptPollVote(context.Background(), evt)
	if perr != nil {
		log.Warn().Err(perr).Str("pollMsgID", pollMsgID).Msg("DecryptPollVote failed")
	}

	if perr == nil && pollVote != nil {
		hashes := pollVote.GetSelectedOptions()
		hashB64 := make([]string, 0, len(hashes))
		for _, h := range hashes {
			hashB64 = append(hashB64, base64.StdEncoding.EncodeToString(h))
		}

		selected := make([]string, 0, len(hashes))
		if stored := clientManager.GetPollOptions(mycli.UserID, pollMsgID); len(stored) > 0 {
			optionsByHash := make(map[string]string, len(stored))
			for _, opt := range stored {
				sum := sha256.Sum256([]byte(opt))
				optionsByHash[string(sum[:])] = opt
			}
			for _, h := range hashes {
				if opt, found := optionsByHash[string(h)]; found {
					selected = append(selected, opt)
				}
			}
		}

		st.postmap["pollVote"] = map[string]interface{}{
			"pollCreationMsgID": pollMsgID,
			"selectedOptions":   selected,
			"selectedHashesB64": hashB64,
		}
	}
}

// decryptSecretEncryptedMessage troca evt.Message pelo conteúdo decifrado, em
// vez de devolvê-lo: o ramo original fazia `evt.Message = decrypted`, e tudo
// que roda depois — mídia, histórico, webhook — lê o campo já trocado.
func (mycli *MyClient) decryptSecretEncryptedMessage(evt *events.Message) {
	if encMessage := evt.Message.GetSecretEncryptedMessage(); encMessage != nil {
		decrypted, derr := mycli.WAClient.DecryptSecretEncryptedMessage(context.Background(), evt)
		if derr != nil {
			log.Warn().
				Err(derr).
				Str("messageID", evt.Info.ID).
				Str("secretEncType", encMessage.GetSecretEncType().String()).
				Msg("DecryptSecretEncryptedMessage failed")
		} else if decrypted != nil {
			log.Info().
				Str("messageID", evt.Info.ID).
				Str("secretEncType", encMessage.GetSecretEncType().String()).
				Msg("Decrypted secretEncryptedMessage; swapping evt.Message")
			evt.Message = decrypted
		}
	}
}

func (mycli *MyClient) processMessageMedia(evt *events.Message, s3Config messageS3Config, st *eventState) {
	isIncoming := !evt.Info.IsFromMe
	chatJID := evt.Info.Sender.String()
	if evt.Info.IsGroup {
		chatJID = evt.Info.Chat.String()
	}

	// Conversão direta, e não literal campo a campo: messageS3Config e
	// mediaS3Config têm os mesmos campos na mesma ordem, e o staticcheck
	// (S1016) rejeita o literal. O valor produzido é idêntico ao do ramo
	// original.
	s3cfg := mediaS3Config(s3Config)

	if img := evt.Message.GetImageMessage(); img != nil {
		mycli.processMedia(img, img.GetMimetype(), ".jpg",
			downloadTimeoutImage, isIncoming, chatJID,
			evt.Info.ID, s3cfg, st.postmap, nil)
	}

	if audio := evt.Message.GetAudioMessage(); audio != nil {
		mycli.processMedia(audio, audio.GetMimetype(), ".ogg",
			downloadTimeoutAudio, isIncoming, chatJID,
			evt.Info.ID, s3cfg, st.postmap, nil)
	}

	if doc := evt.Message.GetDocumentMessage(); doc != nil {
		ext := ".bin"
		if doc.FileName != nil {
			ext = filepath.Ext(*doc.FileName)
		}
		mycli.processMedia(doc, doc.GetMimetype(), ext,
			downloadTimeoutDocument, isIncoming, chatJID,
			evt.Info.ID, s3cfg, st.postmap, nil)
	}

	if video := evt.Message.GetVideoMessage(); video != nil {
		mycli.processMedia(video, video.GetMimetype(), ".mp4",
			downloadTimeoutVideo, isIncoming, chatJID,
			evt.Info.ID, s3cfg, st.postmap, nil)
	}

	if sticker := evt.Message.GetStickerMessage(); sticker != nil {
		mycli.processMedia(sticker, sticker.GetMimetype(), ".webp",
			downloadTimeoutSticker, isIncoming, chatJID,
			evt.Info.ID, s3cfg, st.postmap, map[string]interface{}{
				"isSticker":       true,
				"stickerAnimated": sticker.GetIsAnimated(),
			})
	}
}

func (mycli *MyClient) saveMessageHistory(evt *events.Message, st *eventState) {
	// Get user's history setting from cache
	var historyLimit int
	userinfo, found := appCtx.UserInfoCache.Get(mycli.Token)
	if found {
		historyStr := userinfo.(Values).Get("History")
		historyLimit, _ = strconv.Atoi(historyStr)
	} else {
		log.Warn().Str("userID", mycli.UserID).Msg("User info not found in cache, skipping history")
		historyLimit = 0
	}

	if historyLimit <= 0 {
		return
	}

	messageType := "text"
	textContent := ""
	mediaLink := ""
	caption := ""
	replyToMessageID := ""

	// Check for delete messages first
	if protocolMsg := evt.Message.GetProtocolMessage(); protocolMsg != nil && protocolMsg.GetType() == 0 {
		messageType = "delete"
		if protocolMsg.GetKey() != nil {
			textContent = protocolMsg.GetKey().GetID() // Store the deleted message ID
		}
		log.Info().Str("deletedMessageID", textContent).Str("messageID", evt.Info.ID).Msg("Delete message detected")
		// Check for reactions
	} else if reaction := evt.Message.GetReactionMessage(); reaction != nil {
		messageType = "reaction"
		replyToMessageID = reaction.GetKey().GetID()
		textContent = reaction.GetText() // This will be the emoji
	} else if img := evt.Message.GetImageMessage(); img != nil {
		messageType = "image"
		caption = img.GetCaption()
	} else if video := evt.Message.GetVideoMessage(); video != nil {
		messageType = "video"
		caption = video.GetCaption()
	} else if audio := evt.Message.GetAudioMessage(); audio != nil {
		messageType = "audio"
	} else if doc := evt.Message.GetDocumentMessage(); doc != nil {
		messageType = "document"
		caption = doc.GetCaption()
	} else if sticker := evt.Message.GetStickerMessage(); sticker != nil {
		messageType = "sticker"
	} else if contact := evt.Message.GetContactMessage(); contact != nil {
		messageType = "contact"
		textContent = contact.GetDisplayName()
	} else if location := evt.Message.GetLocationMessage(); location != nil {
		messageType = "location"
		textContent = location.GetName()
	}

	// Extract text content for non-reaction and non-delete messages
	if messageType != "reaction" && messageType != "delete" {
		if conv := evt.Message.GetConversation(); conv != "" {
			textContent = conv
		} else if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
			textContent = ext.GetText()
			// Check if this is a reply to another message
			if contextInfo := ext.GetContextInfo(); contextInfo != nil && contextInfo.GetStanzaID() != "" {
				replyToMessageID = contextInfo.GetStanzaID()
			}
		} else {
			textContent = caption
		}

		// Set default text content for media messages without captions
		if textContent == "" {
			textContent = defaultHistoryTextFor(messageType, textContent)
		}
	}

	// Check for replies in regular conversation messages too.
	// For regular text messages, reply detection currently relies on
	// ExtendedTextMessage handled above; plain Conversation messages
	// carry no reply context in the WhatsApp message structure, so
	// there is nothing further to do here.

	// Try to get media link from S3 data if available
	if s3Data, ok := st.postmap["s3"].(map[string]interface{}); ok {
		if url, ok := s3Data["url"].(string); ok {
			mediaLink = url
		}
	}

	// Only save if there's meaningful content (including delete messages)
	if textContent != "" || mediaLink != "" || (messageType != "text" && messageType != "reaction") || messageType == "delete" {
		// Serializar evt para JSON
		evtJSON, err := json.Marshal(evt)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal event to JSON")
			evtJSON = []byte("{}")
		}

		err = saveMessageToHistory(
			mycli.DB,
			mycli.UserID,
			evt.Info.Chat.String(),
			evt.Info.Sender.String(),
			evt.Info.ID,
			messageType,
			textContent,
			mediaLink,
			replyToMessageID,
			string(evtJSON),
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to save message to history")
		} else {
			err = trimMessageHistory(mycli.DB, mycli.UserID, evt.Info.Chat.String(), historyLimit)
			if err != nil {
				log.Error().Err(err).Msg("Failed to trim message history")
			}
		}
	} else {
		log.Debug().Str("messageType", messageType).Str("messageID", evt.Info.ID).Msg("Skipping empty message from history")
	}
}

// defaultHistoryTextFor é o `switch messageType` que preenchia textContent
// quando a mídia vinha sem legenda. Os dois `if textContent == ""` aninhados
// (contact e location) foram mantidos literalmente: o switch inteiro já só
// roda com textContent vazio, então eles são sempre verdadeiros, mas removê-los
// seria alterar o código e não movê-lo.
func defaultHistoryTextFor(messageType, textContent string) string {
	switch messageType {
	case "image":
		return ":image:"
	case "video":
		return ":video:"
	case "audio":
		return ":audio:"
	case "document":
		return ":document:"
	case "sticker":
		return ":sticker:"
	case "contact":
		if textContent == "" {
			return ":contact:"
		}
	case "location":
		if textContent == "" {
			return ":location:"
		}
	}
	return textContent
}

func (mycli *MyClient) handleReceipt(evt *events.Receipt, st *eventState) bool {
	st.postmap["type"] = "ReadReceipt"
	st.dowebhook = 1
	switch evt.Type {
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		log.Info().Strs("id", evt.MessageIDs).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%v", evt.Timestamp)).Msg("Message was read")
		if evt.Type == types.ReceiptTypeRead {
			st.postmap["state"] = "Read"
		} else {
			st.postmap["state"] = "ReadSelf"
		}
	case types.ReceiptTypeDelivered:
		st.postmap["state"] = "Delivered"
		log.Info().Str("id", evt.MessageIDs[0]).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%v", evt.Timestamp)).Msg("Message delivered")
	default:
		// Discard webhooks for inactive or other delivery types
		return false
	}
	return true
}

func (mycli *MyClient) handleUndecryptableMessage(evt *events.UndecryptableMessage, st *eventState) {
	st.postmap["type"] = "UndecryptableMessage"
	st.dowebhook = 1
	log.Warn().Str("info", evt.Info.SourceString()).Msg("Undecryptable message received")
}

func (mycli *MyClient) handleMediaRetry(evt *events.MediaRetry, st *eventState) {
	st.postmap["type"] = "MediaRetry"
	st.dowebhook = 1
	log.Info().Str("messageID", evt.MessageID).Msg("Media retry event")
}

func (mycli *MyClient) handleFBMessage(evt *events.FBMessage, st *eventState) {
	st.postmap["type"] = "FBMessage"
	st.dowebhook = 1
	log.Info().Str("info", evt.Info.SourceString()).Msg("Facebook message received")
}

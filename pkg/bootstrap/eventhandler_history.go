package bootstrap

import (
	"context"
	"encoding/json"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"

	"github.com/rs/zerolog/log"
)

// Eventos de sincronização de histórico: o blob de conversas antigas que o
// telefone envia depois do pareamento, e os avisos de fim de sincronização
// offline.

func (evh *UserEventHandler) handleHistorySync(evt *events.HistorySync, st *eventState) {
	st.postmap["type"] = "HistorySync"
	st.dowebhook = 1

	// Save HistorySync messages to message_history table
	if evt.Data != nil && evt.Data.Conversations != nil {
		go evh.persistHistorySync(evt.Data.Conversations)
	}
}

// persistHistorySync era o corpo da goroutine anônima do ramo HistorySync.
// Continua rodando em goroutine própria — quem a chama é
// `go evh.persistHistorySync(...)` — e a única coisa que a closure
// capturava além de evh, evt.Data.Conversations, virou parâmetro.
func (evh *UserEventHandler) persistHistorySync(conversations []*waHistorySync.Conversation) {
	// Get the account owner's JID for messages sent by the instance
	accountOwnerJID := ""
	if evh.WAClient.Store != nil && evh.WAClient.Store.ID != nil {
		accountOwnerJID = evh.WAClient.Store.ID.ToNonAD().String()
	}

	savedCount := 0
	for _, conv := range conversations {
		savedCount += evh.persistHistorySyncConversation(conv, accountOwnerJID)
	}

	if savedCount > 0 {
		log.Info().
			Str("userID", evh.UserID).
			Int("savedCount", savedCount).
			Msg("Saved HistorySync messages to message_history")
	}
}

// persistHistorySyncConversation era o corpo do laço externo `for _, conv :=
// range conversations`. Devolve quantas mensagens daquela conversa foram
// gravadas — os dois `continue` do corpo original viraram `return 0`, que é o
// mesmo que não somar nada ao total.
func (evh *UserEventHandler) persistHistorySyncConversation(conv *waHistorySync.Conversation, accountOwnerJID string) int {
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
		if evh.persistHistorySyncMessage(chatJID, accountOwnerJID, msg) {
			saved++
		}
	}
	return saved
}

// persistHistorySyncMessage era o corpo do laço interno `for _, msg := range
// conv.Messages`. Devolve true exatamente quando o corpo original executava
// `savedCount++`; todos os `continue` viraram `return false`, e a saída normal
// do laço sem gravação também.
func (evh *UserEventHandler) persistHistorySyncMessage(chatJID types.JID, accountOwnerJID string, msg *waHistorySync.HistorySyncMsg) bool {
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

	// If senderJID is still empty, skip this message.
	//
	// So' acontece na combinacao: nao e' fromMe, o chat e' grupo ou
	// broadcast, e GetParticipant() veio vazio. Mensagem direta nunca cai
	// aqui, porque usa o proprio chatJID.
	//
	// Medido em 4 pareamentos reais: 6 descartes em 40.786 mensagens
	// gravadas (0,007%). O chatJID entra no log porque sem ele o descarte e'
	// indiagnosticavel — sabia-se que existia, nao de onde vinha.
	if senderJID == "" {
		log.Warn().
			Str("messageID", messageID).
			Str("chatJID", chatJID.String()).
			Bool("isGroup", chatJID.Server == types.GroupServer).
			Msg("Cannot determine sender JID, skipping message")
		return false
	}

	// Get message content and unwrap FutureProofMessage wrappers (F188).
	//
	// The real-time path gets messages already unwrapped by the library's
	// UnwrapRaw (events/message.go:119). The sync path receives raw proto
	// from WebMessageInfo, so we unwrap here to match. Without this, an
	// edited, ephemeral, or view-once message arriving via history sync
	// would reach classifyMessage still wrapped, no branch would match,
	// and the message would be discarded — the same loss the F188
	// measured in the real-time path for edits.
	rawMessage := msg.Message.GetMessage()
	if rawMessage == nil {
		return false
	}
	unwrapped := unwrapFutureProof(rawMessage)
	message := unwrapped.Message

	// A classificação vive em message_classify.go, PARTILHADA com o caminho de
	// tempo real (F187). As duas cadeias divergiram nas DUAS direções ao longo
	// do dia; a única forma de pararem é não haver duas.
	classificacao := classifyMessage(message)
	messageType := classificacao.Type
	textContent := classificacao.Text
	quotedMessageID := classificacao.QuotedID
	mediaLink := ""

	// A ÚNICA diferença que sobrevive de propósito: este caminho chama
	// "unknown" ao que não reconhece, e o de tempo real chama "text".
	//
	// Não as alinhei porque isto é dado GRAVADO: há 689 linhas com
	// `message_type = "unknown"` na tabela, e mudar o rótulo agora tornaria a
	// coluna inconsistente entre o que já lá está e o que entrar a seguir —
	// trocaria uma divergência de código, invisível ao cliente, por uma
	// divergência de DADO, que ele vê.
	//
	// Alinhá-las é decisão de produto com migração, não limpeza de código.
	if messageType == messageTypeText && textContent == "" {
		messageType = "unknown"
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

	pushName := evh.resolverPushName(msg, isFromMe, senderJIDForInfo)

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

	// Create events.Message-like structure for datajson.
	// The Is* flags come from unwrapFutureProof, matching what
	// UnwrapRaw sets in the real-time path.
	messageEvent := map[string]interface{}{
		"Info":                  messageInfo,
		"Message":               message,
		"IsEphemeral":           unwrapped.IsEphemeral,
		"IsViewOnce":            unwrapped.IsViewOnce,
		"IsViewOnceV2":          unwrapped.IsViewOnceV2,
		"IsViewOnceV2Extension": unwrapped.IsViewOnceV2Extension,
		"IsDocumentWithCaption": unwrapped.IsDocumentWithCaption,
		"IsLottieSticker":       unwrapped.IsLottieSticker,
		"IsBotInvoke":           unwrapped.IsBotInvoke,
		"IsEdit":                unwrapped.IsEdit,
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
		err = saveMessageToHistory(evh.DB,
			evh.UserID,
			chatJID.String(),
			senderJID,
			messageID,
			messageType,
			textContent,
			mediaLink,
			quotedMessageID,
			string(evtJSON),
			pushName,
		)
		if err != nil {
			log.Error().Err(err).
				Str("userID", evh.UserID).
				Str("chatJID", chatJID.String()).
				Str("messageID", messageID).
				Msg("Failed to save HistorySync message to history")
		} else {
			return true
		}
	}
	return false
}

func (evh *UserEventHandler) handleOfflineSyncCompleted(evt *events.OfflineSyncCompleted, st *eventState) {
	st.postmap["type"] = "OfflineSyncCompleted"
	st.dowebhook = 1
	log.Info().Msg("Offline sync completed")
}

func (evh *UserEventHandler) handleOfflineSyncPreview(evt *events.OfflineSyncPreview, st *eventState) {
	st.postmap["type"] = "OfflineSyncPreview"
	st.dowebhook = 1
	log.Info().Msg("Offline sync preview")
}

// resolverPushName decide o nome de exibição do remetente de uma mensagem do
// HistorySync.
//
// A ORDEM é a correção da F84. Até ela, esta função consultava SÓ o roster
// local — e para identidades `@lid` o roster está vazio na esmagadora maioria
// dos casos, então gravávamos string vazia em toda conversa nova. Medido no
// banco real: 261 de 1717 conversas individuais tinham nome.
//
// O `WebMessageInfo` que o HistorySync entrega JÁ CARREGA o pushName (campo
// 19 do protobuf). O WhatsApp nos manda o nome em toda mensagem, e nós o
// descartávamos em favor de uma consulta que falha.
//
// A guarda contra vazio vem da Evolution API (issue #2426), que teve o mesmo
// sintoma por causa diferente: o upsert de contato sobrescrevia pushName com
// string vazia a cada mensagem enviada, porque não tinha a guarda que o
// caminho de Chat.name já tinha. A lição é a mesma aqui — nunca deixar o
// vazio vencer um nome que existe.
//
// O roster continua servindo de segunda camada, como o guia de LID do
// Baileys (issue #2414) recomenda: cache de contatos é fonte legítima, mas
// não confiável SOZINHA, porque só eventos de upsert o alimentam.
func (evh *UserEventHandler) resolverPushName(
	msg *waHistorySync.HistorySyncMsg,
	isFromMe bool,
	sender types.JID,
) string {
	// Mensagem própria não tem pushName de remetente a resolver: quem a
	// enviou é o dono da sessão.
	if isFromMe || sender.User == "" {
		return ""
	}

	// Camada 1 — o que veio no protobuf. É o dado mais fresco que existe:
	// chega junto da mensagem, sem depender de sincronização de roster.
	if msg != nil && msg.Message != nil {
		if nome := msg.Message.GetPushName(); nome != "" {
			return nome
		}
	}

	// Camada 2 — o roster local. Só entra quando o protobuf não trouxe nada,
	// e nunca sobrescreve o que veio dele.
	if evh.WAClient == nil || evh.WAClient.Store == nil || evh.WAClient.Store.Contacts == nil {
		return ""
	}
	contact, err := evh.WAClient.Store.Contacts.GetContact(context.Background(), sender)
	if err != nil {
		log.Warn().Err(err).Str("sender", sender.String()).
			Msg("roster indisponivel ao resolver pushName; mensagem segue sem nome")
		return ""
	}
	return contact.PushName
}

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
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// eventState carrega o estado que os ramos do type-switch de myEventHandler
// compartilhavam entre si quando eram todos o corpo de uma função só. Cada
// handler extraído recebe um *eventState e o preenche exatamente como o ramo
// correspondente preenchia as variáveis locais antes da Fase 7.
//
// Handlers que devolvem bool traduzem o `return` que o ramo original tinha:
// false significa "aborte sem disparar webhook", que é o que aquele `return`
// fazia ao sair de myEventHandler antes do bloco final.
type eventState struct {
	txtid     string
	postmap   map[string]interface{}
	dowebhook int
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	st := &eventState{
		txtid:   mycli.UserID,
		postmap: map[string]interface{}{"event": rawEvt},
	}
	// `path` nunca é escrito por ramo nenhum do switch — sempre chega vazio em
	// sendEventWithWebHook. Mantido como estava, e por isso fora de
	// eventState: um campo que ninguém escreve seria pior que a variável.
	path := ""

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		mycli.handleAppStateSyncComplete(evt, st)
	case *events.Connected, *events.PushNameSetting:
		if !mycli.handleConnected(st) {
			return
		}
	case *events.PairSuccess:
		if !mycli.handlePairSuccess(evt, st) {
			return
		}
	case *events.StreamReplaced:
		if !mycli.handleStreamReplaced(evt, st) {
			return
		}
	case *events.Message:

		var s3Config struct {
			Enabled       string `db:"s3_enabled"`
			MediaDelivery string `db:"media_delivery"`
		}

		appCtx.LastMessageCache.Set(mycli.UserID, &evt.Info, cache.DefaultExpiration)
		myuserinfo, found := appCtx.UserInfoCache.Get(mycli.Token)
		if !found {
			err := mycli.DB.Get(&s3Config, "SELECT CASE WHEN s3_enabled = 1 THEN 'true' ELSE 'false' END AS s3_enabled, media_delivery FROM users WHERE id = $1", st.txtid)
			if err != nil {
				log.Error().Err(err).Msg("onMessage Failed to get S3 config from DB as it was not on cache")
				s3Config.Enabled = "false"
				s3Config.MediaDelivery = "base64"
			}
		} else {
			s3Config.Enabled = myuserinfo.(Values).Get("S3Enabled")
			s3Config.MediaDelivery = myuserinfo.(Values).Get("MediaDelivery")
		}

		// Lazy init S3 client if needed (handles reconnect-after-restart when connectOnStartup skipped this user)
		if s3Config.Enabled == "true" && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
			ensureS3ClientForUser(st.txtid)
		}

		st.postmap["type"] = "Message"
		st.dowebhook = 1
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

		// If this is a poll vote, decrypt the E2E-encrypted payload so the
		// webhook can expose which options were selected. Votes arrive as
		// SHA-256 hashes of the option text; we match those back to the
		// plaintext options remembered at send time (see SendPoll in
		// handlers.go). If the session was restarted between send and vote
		// we cannot resolve plaintext; hashes are still emitted so the
		// consumer can perform matching itself if it has stored options.
		if evt.Message.GetPollUpdateMessage() != nil {
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

		if !*skipMedia {

			isIncoming := !evt.Info.IsFromMe
			chatJID := evt.Info.Sender.String()
			if evt.Info.IsGroup {
				chatJID = evt.Info.Chat.String()
			}

			s3cfg := mediaS3Config{
				Enabled:       s3Config.Enabled,
				MediaDelivery: s3Config.MediaDelivery,
			}

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

		// Save message to history regardless of skipMedia setting
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

		if historyLimit > 0 {
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
						if textContent == "" {
							textContent = ":contact:"
						}
					case "location":
						if textContent == "" {
							textContent = ":location:"
						}
					}
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

	case *events.Receipt:
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
			return
		}
	case *events.Presence:
		mycli.handlePresence(evt, st)
	case *events.HistorySync:
		st.postmap["type"] = "HistorySync"
		st.dowebhook = 1

		// Save HistorySync messages to message_history table
		if evt.Data != nil && evt.Data.Conversations != nil {
			go func() {

				// Get the account owner's JID for messages sent by the instance
				accountOwnerJID := ""
				if mycli.WAClient.Store != nil && mycli.WAClient.Store.ID != nil {
					accountOwnerJID = mycli.WAClient.Store.ID.ToNonAD().String()
				}

				savedCount := 0
				for _, conv := range evt.Data.Conversations {
					if conv == nil || conv.ID == nil || conv.Messages == nil {
						continue
					}

					chatJID, err := types.ParseJID(*conv.ID)
					if err != nil {
						log.Warn().Err(err).Str("convID", *conv.ID).Msg("Failed to parse conversation JID in HistorySync")
						continue
					}

					for _, msg := range conv.Messages {
						if msg == nil || msg.Message == nil {
							continue
						}

						// Extract message data
						messageKey := msg.Message.GetKey()
						if messageKey == nil {
							continue
						}

						messageID := messageKey.GetID()
						if messageID == "" {
							continue
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
							continue
						}

						// Get message content
						message := msg.Message.GetMessage()
						if message == nil {
							continue
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
								savedCount++
							}
						}
					}
				}

				if savedCount > 0 {
					log.Info().
						Str("userID", mycli.UserID).
						Int("savedCount", savedCount).
						Msg("Saved HistorySync messages to message_history")
				}
			}()
		}

	case *events.AppState:
		mycli.handleAppState(evt, st)
	case *events.LoggedOut:
		// O `defer` fica AQUI, e nao dentro de handleLoggedOut: defer adia ate
		// o fim da funcao, entao no arquivo original o sinal saia DEPOIS de
		// sendEventWithWebHook. Move-lo para o handler o anteciparia.
		defer func() {
			// Use a non-blocking send to prevent a deadlock if the receiver has already terminated.
			appCtx.KillChannel.Signal(mycli.UserID)
		}()
		if !mycli.handleLoggedOut(evt, st) {
			return
		}
	case *events.ChatPresence:
		mycli.handleChatPresence(evt, st)
	case *events.CallOffer:
		mycli.handleCallOffer(evt, st)
	case *events.CallAccept:
		mycli.handleCallAccept(evt, st)
	case *events.CallTerminate:
		mycli.handleCallTerminate(evt, st)
	case *events.CallOfferNotice:
		mycli.handleCallOfferNotice(evt, st)
	case *events.CallRelayLatency:
		mycli.handleCallRelayLatency(evt, st)
	case *events.Disconnected:
		mycli.handleDisconnected(evt, st)
	case *events.ConnectFailure:
		mycli.handleConnectFailure(evt, st)
	case *events.UndecryptableMessage:
		st.postmap["type"] = "UndecryptableMessage"
		st.dowebhook = 1
		log.Warn().Str("info", evt.Info.SourceString()).Msg("Undecryptable message received")
	case *events.MediaRetry:
		st.postmap["type"] = "MediaRetry"
		st.dowebhook = 1
		log.Info().Str("messageID", evt.MessageID).Msg("Media retry event")
	case *events.GroupInfo:
		mycli.handleGroupInfo(evt, st)
	case *events.JoinedGroup:
		mycli.handleJoinedGroup(evt, st)
	case *events.Picture:
		mycli.handlePicture(evt, st)
	case *events.BlocklistChange:
		mycli.handleBlocklistChange(evt, st)
	case *events.Blocklist:
		mycli.handleBlocklist(evt, st)
	case *events.KeepAliveRestored:
		mycli.handleKeepAliveRestored(evt, st)
	case *events.KeepAliveTimeout:
		mycli.handleKeepAliveTimeout(evt, st)
	case *events.ClientOutdated:
		mycli.handleClientOutdated(evt, st)
	case *events.TemporaryBan:
		mycli.handleTemporaryBan(evt, st)
	case *events.StreamError:
		mycli.handleStreamError(evt, st)
	case *events.PairError:
		mycli.handlePairError(evt, st)
	case *events.PrivacySettings:
		mycli.handlePrivacySettings(evt, st)
	case *events.UserAbout:
		mycli.handleUserAbout(evt, st)
	case *events.OfflineSyncCompleted:
		st.postmap["type"] = "OfflineSyncCompleted"
		st.dowebhook = 1
		log.Info().Msg("Offline sync completed")
	case *events.OfflineSyncPreview:
		st.postmap["type"] = "OfflineSyncPreview"
		st.dowebhook = 1
		log.Info().Msg("Offline sync preview")
	case *events.IdentityChange:
		mycli.handleIdentityChange(evt, st)
	case *events.NewsletterJoin:
		mycli.handleNewsletterJoin(evt, st)
	case *events.NewsletterLeave:
		mycli.handleNewsletterLeave(evt, st)
	case *events.NewsletterMuteChange:
		mycli.handleNewsletterMuteChange(evt, st)
	case *events.NewsletterLiveUpdate:
		mycli.handleNewsletterLiveUpdate(evt, st)
	case *events.FBMessage:
		st.postmap["type"] = "FBMessage"
		st.dowebhook = 1
		log.Info().Str("info", evt.Info.SourceString()).Msg("Facebook message received")
	default:
		log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
	}

	if st.dowebhook == 1 {
		sendEventWithWebHook(mycli, st.postmap, path)
	}
}

package message

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/libsignal/signalerror"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/capabilities/send"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// DecryptMessages percorre os filhos <enc> de um <message> e decifra cada um.
//
// Extracao MECANICA de `(*Client).decryptMessages`. Vale a pena registrar o que
// NAO mudou, porque cada um destes e' observavel:
//
//   - a ordem dos ramos de tipo de <enc> (pkmsg/msg, depois skmsg so' em grupo,
//     depois msmsg so' de bot);
//   - quais erros sao ignorados em silencio e com qual nivel de log
//     (ErrEventAlreadyProcessed em Debug, ErrOldCounter em Warn, ambos com
//     `continue`, sem retry receipt);
//   - o `return` — e nao `continue` — no caminho de erro de decifragem, que faz
//     um <message> com varios <enc> parar no primeiro que falha;
//   - o ack ser mandado no `backgroundIfAsyncAck` do fim, mesmo depois de um
//     `continue`, mas NAO depois de um `return`;
//   - `containsDirectMsg` so' e' setado no ramo DM, e e' lido no ramo de erro
//     para decidir se a falta de sender key vira "unavailable".
func DecryptMessages(ctx context.Context, t Transport, info *types.MessageInfo, node *waBinary.Node) {
	unavailableNode, ok := node.GetOptionalChildByTag("unavailable")
	if ok && len(node.GetChildrenByTag(send.EncNodeTag)) == 0 {
		uType := events.UnavailableType(unavailableNode.AttrGetter().String("type"))
		t.Log().Warnf("Unavailable message %s from %s (type: %q)", info.ID, info.SourceString(), uType)
		t.BackgroundIfAsyncAck(func() {
			t.ImmediateRequestMessageFromPhone(ctx, info)
			t.SendAck(ctx, node, 0)
		})
		t.DispatchEvent(&events.UndecryptableMessage{Info: *info, IsUnavailable: true, UnavailableType: uType})
		return
	}

	children := node.GetChildren()
	t.Log().Debugf("Decrypting message from %s", info.SourceString())
	containsDirectMsg := false
	senderEncryptionJID := info.Sender
	if info.Sender.Server == types.DefaultUserServer && !info.Sender.IsBot() {
		if info.SenderAlt.Server == types.HiddenUserServer {
			senderEncryptionJID = info.SenderAlt
			MigrateSessionStore(ctx, t, info.Sender, info.SenderAlt)
		} else if lid, err := t.Store().LIDs.GetLIDForPN(ctx, info.Sender); err != nil {
			t.Log().Errorf("Failed to get LID for %s: %v", info.Sender, err)
		} else if !lid.IsEmpty() {
			MigrateSessionStore(ctx, t, info.Sender, lid)
			senderEncryptionJID = lid
			info.SenderAlt = lid
		} else {
			t.Log().Warnf("No LID found for %s", info.Sender)
		}
	}
	var recognizedStanza, protobufFailed bool
	for _, child := range children {
		if child.Tag != send.EncNodeTag {
			continue
		}
		recognizedStanza = true
		ag := child.AttrGetter()
		encType, ok := ag.GetString(send.EncAttrType, false)
		if !ok {
			continue
		}
		var decrypted []byte
		var ciphertextHash *[32]byte
		var err error
		if encType == send.EncTypePreKeyMsg || encType == send.EncTypeMsg {
			decrypted, ciphertextHash, err = DecryptDM(ctx, t, &child, senderEncryptionJID, encType == send.EncTypePreKeyMsg, info.Timestamp)
			containsDirectMsg = true
		} else if info.IsGroup && encType == send.EncTypeSenderKey {
			decrypted, ciphertextHash, err = DecryptGroupMsg(ctx, t, &child, senderEncryptionJID, info.Chat, info.Timestamp)
		} else if encType == EncTypeMsgSecret && info.Sender.IsBot() {
			targetSenderJID := info.MsgMetaInfo.TargetSender
			if targetSenderJID.User == "" {
				if info.Sender.Server == types.BotServer {
					targetSenderJID = t.OwnLID()
				} else {
					targetSenderJID = t.OwnID()
				}
			}
			var decryptMessageID string
			if info.MsgBotInfo.EditType == types.EditTypeInner || info.MsgBotInfo.EditType == types.EditTypeLast {
				decryptMessageID = info.MsgBotInfo.EditTargetID
			} else {
				decryptMessageID = info.ID
			}
			var msMsg waE2E.MessageSecretMessage
			var messageSecret []byte
			// DecryptDM/DecryptGroupMsg checam o tipo do Content antes de usar;
			// este ramo nao checava e um <enc type="msmsg"> com filhos (ou sem
			// conteudo) derrubava o cliente inteiro por type assertion.
			msSecretContent, msSecretContentOK := child.Content.([]byte)
			if !msSecretContentOK {
				err = fmt.Errorf("message content is not a byte slice")
			} else if messageSecret, _, err = t.Store().MsgSecrets.GetMessageSecret(ctx, info.Chat, targetSenderJID, info.MsgMetaInfo.TargetID); err != nil {
				err = fmt.Errorf("failed to get message secret for %s: %v", info.MsgMetaInfo.TargetID, err)
			} else if messageSecret == nil {
				err = fmt.Errorf("message secret for %s not found", info.MsgMetaInfo.TargetID)
			} else if err = proto.Unmarshal(msSecretContent, &msMsg); err != nil {
				err = fmt.Errorf("failed to unmarshal MessageSecretMessage protobuf: %v", err)
			} else {
				decrypted, err = DecryptBotMessage(t, messageSecret, &msMsg, decryptMessageID, targetSenderJID, info)
			}
		} else {
			t.Log().Warnf("Unhandled encrypted message (type %s) from %s", encType, info.SourceString())
			continue
		}

		if errors.Is(err, ErrEventAlreadyProcessed) {
			t.Log().Debugf("Ignoring message %s from %s: %v", info.ID, info.SourceString(), err)
			continue
		} else if errors.Is(err, signalerror.ErrOldCounter) {
			t.Log().Warnf("Ignoring message %s from %s: %v", info.ID, info.SourceString(), err)
			continue
		} else if err != nil {
			t.Log().Warnf("Error decrypting message %s from %s: %v", info.ID, info.SourceString(), err)
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}
			isUnavailable := encType == send.EncTypeSenderKey && !containsDirectMsg && errors.Is(err, signalerror.ErrNoSenderKeyForUser)
			if encType == EncTypeMsgSecret {
				t.BackgroundIfAsyncAck(func() {
					t.SendAck(ctx, node, t.Nacks().MissingMessageSecret)
				})
			} else if t.SynchronousAck() {
				t.SendRetryReceipt(ctx, node, info, isUnavailable)
				// TODO this probably isn't supposed to ack
				t.SendAck(ctx, node, 0)
			} else {
				go t.SendRetryReceipt(context.WithoutCancel(ctx), node, info, isUnavailable)
				go t.SendAck(ctx, node, 0)
			}
			t.DispatchEvent(&events.UndecryptableMessage{
				Info:            *info,
				IsUnavailable:   isUnavailable,
				DecryptFailMode: events.DecryptFailMode(ag.OptionalString(send.EncAttrDecryptFail)),
			})
			return
		}
		retryCount := ag.OptionalInt("count")
		t.CancelDelayedRequestFromPhone(info.ID)

		var msg waE2E.Message
		var handlerFailed bool
		switch ag.Int(send.EncAttrVersion) {
		case 2:
			err = proto.Unmarshal(decrypted, &msg)
			if err != nil {
				t.Log().Warnf("Error unmarshaling decrypted message from %s: %v", info.SourceString(), err)
				protobufFailed = true
				continue
			}
			protobufFailed = false
			handlerFailed = HandleDecrypted(ctx, t, info, &msg, retryCount)
		case 3:
			handlerFailed, protobufFailed = t.HandleDecryptedArmadillo(ctx, info, decrypted, retryCount)
		default:
			t.Log().Warnf("Unknown version %d in decrypted message from %s", ag.Int(send.EncAttrVersion), info.SourceString())
		}
		if handlerFailed {
			t.Log().Warnf("Handler for %s failed", info.ID)
			return
		}
		if ciphertextHash != nil && t.EnableDecryptedEventBuffer() {
			// Use the context passed to DecryptMessages
			err = t.Store().EventBuffer.ClearBufferedEventPlaintext(ctx, *ciphertextHash)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Hex("ciphertext_hash", ciphertextHash[:]).
					Str("message_id", info.ID).
					Msg("Failed to clear buffered event plaintext")
			} else {
				zerolog.Ctx(ctx).Debug().
					Hex("ciphertext_hash", ciphertextHash[:]).
					Str("message_id", info.ID).
					Msg("Deleted event plaintext from buffer")
			}

			// O original era
			// `if time.Since(cli.lastDecryptedBufferClear) > interval && ctx.Err() == nil`,
			// com a atribuicao de `time.Now()` dentro do corpo. ShouldClear junta o
			// teste e a atribuicao; o teste de contexto veio para a FRENTE de
			// proposito, para que o curto-circuito preserve exatamente o caso em
			// que o original NAO atualizava o marcador (contexto cancelado).
			if ctx.Err() == nil && t.DecryptBuffer().ShouldClear() {
				go func() {
					err := t.Store().EventBuffer.DeleteOldBufferedHashes(context.WithoutCancel(ctx))
					if err != nil {
						zerolog.Ctx(ctx).Err(err).Msg("Failed to delete old buffered hashes")
					}
				}()
			}
		}
	}
	t.BackgroundIfAsyncAck(func() {
		if !recognizedStanza {
			t.SendAck(ctx, node, t.Nacks().UnrecognizedStanza)
		} else if protobufFailed {
			t.SendAck(ctx, node, t.Nacks().InvalidProtobuf)
		} else {
			t.SendMessageReceipt(ctx, info, node)
		}
	})
}

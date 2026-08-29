package message

import (
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waCommon"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// EditWindow especifica por quanto tempo uma mensagem pode ser editada depois
// de enviada.
const EditWindow = 20 * time.Minute

// BuildKey monta a MessageKey usada para referenciar mensagens anteriores em
// respostas, revogacoes e reacoes.
//
// `ownID`/`ownLID` sao os dois JIDs da sessao; a versao em *Client consultava
// `cli.getOwnID()`/`cli.getOwnLID()` no mesmo ponto, e a fachada da raiz
// continua fazendo isso.
func BuildKey(ownID, ownLID, chat, sender types.JID, id types.MessageID) *waCommon.MessageKey {
	key := &waCommon.MessageKey{
		FromMe:    proto.Bool(true),
		ID:        proto.String(id),
		RemoteJID: proto.String(chat.String()),
	}
	if !sender.IsEmpty() && sender.User != ownID.User && sender.User != ownLID.User {
		key.FromMe = proto.Bool(false)
		if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.MessengerServer {
			key.Participant = proto.String(sender.ToNonAD().String())
		}
	}
	return key
}

// BuildRevoke monta uma mensagem de revogacao.
func BuildRevoke(ownID, ownLID, chat, sender types.JID, id types.MessageID) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  BuildKey(ownID, ownLID, chat, sender, id),
		},
	}
}

// BuildReaction monta uma mensagem de reacao.
func BuildReaction(ownID, ownLID, chat, sender types.JID, id types.MessageID, reaction string) *waE2E.Message {
	return &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key:               BuildKey(ownID, ownLID, chat, sender, id),
			Text:              proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
}

// BuildUnavailableRequest monta o pedido ao dispositivo primario para reenviar
// uma mensagem que este cliente nao conseguiu decifrar.
func BuildUnavailableRequest(ownID, ownLID, chat, sender types.JID, id string) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_PLACEHOLDER_MESSAGE_RESEND.Enum(),
				PlaceholderMessageResendRequest: []*waE2E.PeerDataOperationRequestMessage_PlaceholderMessageResendRequest{{
					MessageKey: BuildKey(ownID, ownLID, chat, sender, id),
				}},
			},
		},
	}
}

// BuildHistorySyncRequest monta o pedido de mais historico ao dispositivo
// primario.
func BuildHistorySyncRequest(lastKnownMessageInfo *types.MessageInfo, count int) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND.Enum(),
				HistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_HistorySyncOnDemandRequest{
					ChatJID:          proto.String(lastKnownMessageInfo.Chat.String()),
					OldestMsgID:      proto.String(lastKnownMessageInfo.ID),
					OldestMsgFromMe:  proto.Bool(lastKnownMessageInfo.IsFromMe),
					OnDemandMsgCount: proto.Int32(int32(count)),
					// Apesar do nome do campo dizer "MS", isto deve conter segundos.
					OldestMsgTimestampMS: proto.Int64(lastKnownMessageInfo.Timestamp.Unix()),
				},
			},
		},
	}
}

// BuildEdit monta uma mensagem de edicao.
//
// Nao usa BuildKey de proposito: a chave da edicao e' SEMPRE `FromMe: true` e
// nunca carrega participant, mesmo em grupo. Era assim antes da extracao, e
// unificar com BuildKey mudaria o que vai para o fio.
func BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message {
	return &waE2E.Message{
		EditedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ProtocolMessage: &waE2E.ProtocolMessage{
					Key: &waCommon.MessageKey{
						FromMe:    proto.Bool(true),
						ID:        proto.String(id),
						RemoteJID: proto.String(chat.String()),
					},
					Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
					EditedMessage: newContent,
					TimestampMS:   proto.Int64(time.Now().UnixMilli()),
				},
			},
		},
	}
}

// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/waclient/proto/waCommon"
	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/types"
)

// BuildMessageKey builds a MessageKey object, which is used to refer to previous messages
// for things such as replies, revocations and reactions.
func (cli *Client) BuildMessageKey(chat, sender types.JID, id types.MessageID) *waCommon.MessageKey {
	key := &waCommon.MessageKey{
		FromMe:    proto.Bool(true),
		ID:        proto.String(id),
		RemoteJID: proto.String(chat.String()),
	}
	if !sender.IsEmpty() && sender.User != cli.getOwnID().User && sender.User != cli.getOwnLID().User {
		key.FromMe = proto.Bool(false)
		if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.MessengerServer {
			key.Participant = proto.String(sender.ToNonAD().String())
		}
	}
	return key
}

// BuildRevoke builds a message revocation message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
// To revoke your own messages, pass your JID or an empty JID as the second parameter (sender).
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildRevoke(chat, types.EmptyJID, originalMessageID)
//
// To revoke someone else's messages when you are group admin, pass the message sender's JID as the second parameter.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildRevoke(chat, senderJID, originalMessageID)
func (cli *Client) BuildRevoke(chat, sender types.JID, id types.MessageID) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  cli.BuildMessageKey(chat, sender, id),
		},
	}
}

// BuildReaction builds a message reaction message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildReaction(chat, senderJID, targetMessageID, "🐈️")
//
// Note that for newsletter messages, you need to use NewsletterSendReaction instead of BuildReaction + SendMessage.
func (cli *Client) BuildReaction(chat, sender types.JID, id types.MessageID, reaction string) *waE2E.Message {
	return &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key:               cli.BuildMessageKey(chat, sender, id),
			Text:              proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
}

// BuildUnavailableMessageRequest builds a message to request the user's primary device to send
// the copy of a message that this client was unable to decrypt.
//
// The built message can be sent using Client.SendPeerMessage.
// The full response will come as a ProtocolMessage with type `PEER_DATA_OPERATION_REQUEST_RESPONSE_MESSAGE`.
// The response events will also be dispatched as normal *events.Message's with UnavailableRequestID set to the request message ID.
func (cli *Client) BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_PLACEHOLDER_MESSAGE_RESEND.Enum(),
				PlaceholderMessageResendRequest: []*waE2E.PeerDataOperationRequestMessage_PlaceholderMessageResendRequest{{
					MessageKey: cli.BuildMessageKey(chat, sender, id),
				}},
			},
		},
	}
}

// BuildHistorySyncRequest builds a message to request additional history from the user's primary device.
//
// The built message can be sent using Client.SendPeerMessage.
// The response will come as an *events.HistorySync with type `ON_DEMAND`.
//
// The response will contain to `count` messages immediately before the given message.
// The recommended number of messages to request at a time is 50.
func (cli *Client) BuildHistorySyncRequest(lastKnownMessageInfo *types.MessageInfo, count int) *waE2E.Message {
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
					// Despite the field name saying "MS", this is actually supposed to contain seconds
					OldestMsgTimestampMS: proto.Int64(lastKnownMessageInfo.Timestamp.Unix()),
				},
			},
		},
	}
}

// EditWindow specifies how long a message can be edited for after it was sent.
const EditWindow = 20 * time.Minute

// BuildEdit builds a message edit message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildEdit(chat, originalMessageID, &waE2E.Message{
//		Conversation: proto.String("edited message"),
//	})
func (cli *Client) BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message {
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

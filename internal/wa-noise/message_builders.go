// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"wa-api/internal/wa-noise/message"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

// Os construtores de mensagem vivem em internal/wa-noise/message/ desde a Fase
// F/G lote 9. Os metodos abaixo continuam na raiz porque sao API PUBLICA do
// pacote (chamadores externos usam `cli.BuildRevoke(...)`) e porque a versao
// livre precisa dos dois JIDs da sessao, que so' a raiz sabe consultar.

// EditWindow specifies how long a message can be edited for after it was sent.
const EditWindow = message.EditWindow

// BuildMessageKey builds a MessageKey object, which is used to refer to previous messages
// for things such as replies, revocations and reactions.
func (cli *Client) BuildMessageKey(chat, sender types.JID, id types.MessageID) *waCommon.MessageKey {
	return message.BuildKey(cli.getOwnID(), cli.getOwnLID(), chat, sender, id)
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
	return message.BuildRevoke(cli.getOwnID(), cli.getOwnLID(), chat, sender, id)
}

// BuildReaction builds a message reaction message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildReaction(chat, senderJID, targetMessageID, "🐈️")
//
// Note that for newsletter messages, you need to use NewsletterSendReaction instead of BuildReaction + SendMessage.
func (cli *Client) BuildReaction(chat, sender types.JID, id types.MessageID, reaction string) *waE2E.Message {
	return message.BuildReaction(cli.getOwnID(), cli.getOwnLID(), chat, sender, id, reaction)
}

// BuildUnavailableMessageRequest builds a message to request the user's primary device to send
// the copy of a message that this client was unable to decrypt.
//
// The built message can be sent using Client.SendPeerMessage.
// The full response will come as a ProtocolMessage with type `PEER_DATA_OPERATION_REQUEST_RESPONSE_MESSAGE`.
// The response events will also be dispatched as normal *events.Message's with UnavailableRequestID set to the request message ID.
func (cli *Client) BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message {
	return message.BuildUnavailableRequest(cli.getOwnID(), cli.getOwnLID(), chat, sender, id)
}

// BuildHistorySyncRequest builds a message to request additional history from the user's primary device.
//
// The built message can be sent using Client.SendPeerMessage.
// The response will come as an *events.HistorySync with type `ON_DEMAND`.
//
// The response will contain to `count` messages immediately before the given message.
// The recommended number of messages to request at a time is 50.
func (cli *Client) BuildHistorySyncRequest(lastKnownMessageInfo *types.MessageInfo, count int) *waE2E.Message {
	return message.BuildHistorySyncRequest(lastKnownMessageInfo, count)
}

// BuildEdit builds a message edit message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildEdit(chat, originalMessageID, &waE2E.Message{
//		Conversation: proto.String("edited message"),
//	})
func (cli *Client) BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message {
	return message.BuildEdit(chat, id, newContent)
}

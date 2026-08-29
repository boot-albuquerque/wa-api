package core

import (
	"context"
	"errors"

	"wa-api/internal/noise/capabilities/send"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// A logica deste dominio vive em internal/wa-noise/send/. O que sobra aqui sao
// fachadas: elas guardam o contrato historico (nomes, assinaturas e o receptor
// *Client) e delegam. Ver PATCHES.md, "Fase F/G — lote 8".

// SendMessage sends the given message.
//
// This method will wait for the server to acknowledge the message before returning.
// The return value is the timestamp of the message from the server.
//
// Optional parameters like the message ID can be specified with the SendRequestExtra struct.
// Only one extra parameter is allowed, put all necessary parameters in the same struct.
//
// The message itself can contain anything you want (within the protobuf schema).
// e.g. for a simple text message, use the Conversation field:
//
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{
//		Conversation: proto.String("Hello, World!"),
//	})
//
// Things like replies, mentioning users and the "forwarded" flag are stored in ContextInfo,
// which can be put in ExtendedTextMessage and any of the media message types.
//
// For uploading and sending media/attachments, see the Upload method.
//
// For other message types, you'll have to figure it out yourself. Looking at the protobuf schema
// in binary/proto/def.proto may be useful to find out all the allowed fields. Printing the RawMessage
// field in incoming message events to figure out what it contains is also a good way to learn how to
// send the same kind of message.
func (cli *Client) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...SendRequestExtra) (resp SendResponse, err error) {
	if cli == nil {
		err = ErrClientIsNil
		return
	}
	var req SendRequestExtra
	if len(extra) > 1 {
		err = errors.New("only one extra parameter may be provided to SendMessage")
		return
	} else if len(extra) == 1 {
		req = extra[0]
	}
	return send.Message(ctx, cli.sendT(), to, message, req)
}

func (cli *Client) SendPeerMessage(ctx context.Context, message *waE2E.Message) (SendResponse, error) {
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		return SendResponse{}, ErrNotLoggedIn
	}
	return cli.SendMessage(ctx, ownID, message, SendRequestExtra{Peer: true})
}

// RevokeMessage deletes the given message from everyone in the chat.
//
// This method will wait for the server to acknowledge the revocation message before returning.
// The return value is the timestamp of the message from the server.
//
// Deprecated: This method is deprecated in favor of BuildRevoke
func (cli *Client) RevokeMessage(ctx context.Context, chat types.JID, id types.MessageID) (SendResponse, error) {
	return cli.SendMessage(ctx, chat, cli.BuildRevoke(chat, types.EmptyJID, id))
}

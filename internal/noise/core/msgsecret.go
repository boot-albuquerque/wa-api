package core

import (
	"context"

	"wa-api/internal/noise/capabilities/message"
	"wa-api/internal/noise/protocol/proto/waCommon"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// O dominio de segredo de mensagem vive em internal/noise/message/ desde a
// Fase F/G lote 9. As guardas `cli == nil -> ErrClientIsNil` continuam AQUI,
// nas fachadas, e nao atravessaram a fronteira: um *Client nil nao produz
// Transport, entao a checagem so' pode existir na raiz. Mesmo racional dos
// lotes 1-8.

func (cli *Client) decryptMsgSecret(ctx context.Context, msg *events.Message, useCase MsgSecretType, encrypted messageEncryptedSecret, origMsgKey *waCommon.MessageKey) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.DecryptSecret(ctx, cli.msgT(), msg, useCase, encrypted, origMsgKey)
}

func (cli *Client) encryptMsgSecret(ctx context.Context, ownID, chat, origSender types.JID, origMsgID types.MessageID, useCase MsgSecretType, plaintext []byte) (ciphertext, iv []byte, err error) {
	if cli == nil {
		return nil, nil, ErrClientIsNil
	}
	return message.EncryptSecret(ctx, cli.msgT(), ownID, chat, origSender, origMsgID, useCase, plaintext)
}

// decryptBotMessage mantem o parametro de contexto, sem uso, porque
// internals.go (gerado) copia a assinatura. Ele ja' era ignorado antes da
// extracao — a derivacao e o AES-GCM nao tocam em I/O.
func (cli *Client) decryptBotMessage(_ context.Context, messageSecret []byte, msMsg messageEncryptedSecret, messageID types.MessageID, targetSenderJID types.JID, info *types.MessageInfo) ([]byte, error) {
	return message.DecryptBotMessage(cli.msgT(), messageSecret, msMsg, messageID, targetSenderJID, info)
}

// DecryptReaction decrypts a reaction message in a community announcement group.
//
//	if evt.Message.GetEncReactionMessage() != nil {
//		reaction, err := cli.DecryptReaction(evt)
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		fmt.Printf("Reaction message: %+v\n", reaction)
//	}
func (cli *Client) DecryptReaction(ctx context.Context, reaction *events.Message) (*waE2E.ReactionMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.DecryptReaction(ctx, cli.msgT(), reaction)
}

// DecryptComment decrypts a reply/comment message in a community announcement group.
//
//	if evt.Message.GetEncCommentMessage() != nil {
//		comment, err := cli.DecryptComment(evt)
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		fmt.Printf("Comment message: %+v\n", comment)
//	}
func (cli *Client) DecryptComment(ctx context.Context, comment *events.Message) (*waE2E.Message, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.DecryptComment(ctx, cli.msgT(), comment)
}

// DecryptSecretEncryptedMessage decrypts a message encrypted with a message secret.
func (cli *Client) DecryptSecretEncryptedMessage(ctx context.Context, evt *events.Message) (*waE2E.Message, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.DecryptSecretEncrypted(ctx, cli.msgT(), evt)
}

// EncryptComment encrypts a comment for the given root message.
func (cli *Client) EncryptComment(ctx context.Context, rootMsgInfo *types.MessageInfo, comment *waE2E.Message) (*waE2E.Message, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.EncryptComment(ctx, cli.msgT(), rootMsgInfo, comment)
}

// EncryptReaction encrypts a reaction for the given root message.
func (cli *Client) EncryptReaction(ctx context.Context, rootMsgInfo *types.MessageInfo, reaction *waE2E.ReactionMessage) (*waE2E.EncReactionMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return message.EncryptReaction(ctx, cli.msgT(), rootMsgInfo, reaction)
}

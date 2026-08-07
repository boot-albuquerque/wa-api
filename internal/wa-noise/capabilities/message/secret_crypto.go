package message

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/util/random"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	gcmutil "wa-api/internal/wa-noise/security/gcm"
)

// DecryptSecret decifra um payload protegido por segredo de mensagem.
//
// O ramo de retentativa com `storedOrigSender` NAO foi mexido: e' o hack que
// tenta os dois remetentes possiveis (o do evento novo e aquele de quem
// recebemos a chave) enquanto o WhatsApp nao termina a migracao para LID. Ele
// e' disparado por comparacao de SUBSTRING na mensagem de erro
// ("message authentication failed"), o que e' fragil, mas trocar isso mudaria
// quais mensagens conseguem ser lidas. Registrado, nao alterado.
func DecryptSecret(ctx context.Context, t Transport, msg *events.Message, useCase SecretType, encrypted EncryptedSecret, origMsgKey *waCommon.MessageKey) ([]byte, error) {
	origSender, err := OrigSenderFromKey(msg, origMsgKey)
	if err != nil {
		return nil, err
	}
	baseEncKey, storedOrigSender, err := t.Store().MsgSecrets.GetMessageSecret(ctx, msg.Info.Chat, origSender, origMsgKey.GetID())
	if err != nil {
		return nil, fmt.Errorf("failed to get original message secret key: %w", err)
	}
	if baseEncKey == nil {
		return nil, ErrOriginalMessageSecretNotFound
	}
	secretKey, additionalData := GenerateSecretKey(useCase, msg.Info.Sender, origMsgKey.GetID(), origSender, baseEncKey)
	plaintext, err := gcmutil.Decrypt(secretKey, encrypted.GetEncIV(), encrypted.GetEncPayload(), additionalData)
	if err != nil {
		// Hack for trying both the original sender in the new message and the one who we received the secret key from.
		// This will hopefully become unnecessary when WhatsApp fully finishes their migration to LIDs.
		if origSender != storedOrigSender && strings.Contains(err.Error(), "message authentication failed") {
			secretKey, additionalData = GenerateSecretKey(useCase, msg.Info.Sender, origMsgKey.GetID(), storedOrigSender, baseEncKey)
			plaintext, err = gcmutil.Decrypt(secretKey, encrypted.GetEncIV(), encrypted.GetEncPayload(), additionalData)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret message: %w (sender: %s, orig sender: %s and %s)", err, msg.Info.Sender, origSender, storedOrigSender)
		}
	}
	return plaintext, nil
}

// EncryptSecret cifra um payload com a chave derivada do segredo de uma
// mensagem original.
//
// Note que `origSender` e' REATRIBUIDO pelo retorno do store antes da
// derivacao: o store devolve o remetente sob o qual o segredo foi de fato
// gravado, que pode diferir do que o chamador passou (PN vs LID). Isso e'
// deliberado e vem de antes da extracao.
func EncryptSecret(ctx context.Context, t Transport, ownID, chat, origSender types.JID, origMsgID types.MessageID, useCase SecretType, plaintext []byte) (ciphertext, iv []byte, err error) {
	if ownID.IsEmpty() {
		return nil, nil, t.Errors().NotLoggedIn
	}

	baseEncKey, origSender, err := t.Store().MsgSecrets.GetMessageSecret(ctx, chat, origSender, origMsgID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get original message secret key: %w", err)
	} else if baseEncKey == nil {
		return nil, nil, ErrOriginalMessageSecretNotFound
	}
	secretKey, additionalData := GenerateSecretKey(useCase, ownID, origMsgID, origSender, baseEncKey)

	iv = random.Bytes(msgSecretIVSize)
	ciphertext, err = gcmutil.Encrypt(secretKey, iv, plaintext, additionalData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encrypt secret message: %w", err)
	}
	return ciphertext, iv, nil
}

// DecryptBotMessage decifra o payload de uma mensagem de bot (<enc type="msmsg">).
//
// Difere de DecryptSecret em dois pontos, ambos preservados literalmente: o
// segredo passa por ApplyBotMessageHKDF antes, e o rotulo de caso de uso e' a
// string VAZIA — que, em GenerateSecretKey, e' justamente um dos tres casos que
// produzem dado adicional autenticado.
func DecryptBotMessage(t Transport, messageSecret []byte, msMsg EncryptedSecret, messageID types.MessageID, targetSenderJID types.JID, info *types.MessageInfo) ([]byte, error) {
	newKey, additionalData := GenerateSecretKey("", info.Sender, messageID, targetSenderJID, ApplyBotMessageHKDF(messageSecret))

	plaintext, err := gcmutil.Decrypt(newKey, msMsg.GetEncIV(), msMsg.GetEncPayload(), additionalData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret message: %w", err)
	}

	return plaintext, nil
}

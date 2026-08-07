// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"fmt"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waMmsRetry"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/internal/wa-noise/security/gcm"
	"wa-api/internal/wa-noise/security/hkdf"
)

const (
	// mediaRetryKeyInfo e' o rotulo HKDF que deriva a chave do receipt de
	// retry de midia a partir da mediaKey da mensagem original.
	mediaRetryKeyInfo   = "WhatsApp Media Retry Notification"
	mediaRetryKeyLength = 32
	// mediaRetryIVLength e' o tamanho do nonce AES-GCM do receipt.
	mediaRetryIVLength = 12
	// mediaRetryErrCodeNotAvailable e' o codigo que o telefone devolve quando
	// nao tem mais a midia para reenviar.
	mediaRetryErrCodeNotAvailable = 2
)

// RetryKey deriva, da mediaKey da mensagem original, a chave usada para cifrar
// e decifrar o receipt de retry de midia.
func RetryKey(mediaKey []byte) (cipherKey []byte) {
	return hkdfutil.SHA256(mediaKey, nil, []byte(mediaRetryKeyInfo), mediaRetryKeyLength)
}

// EncryptRetryReceipt monta e cifra o corpo do receipt de retry de midia para a
// mensagem dada.
func EncryptRetryReceipt(messageID types.MessageID, mediaKey []byte) (ciphertext, iv []byte, err error) {
	receipt := &waMmsRetry.ServerErrorReceipt{
		StanzaID: proto.String(messageID),
	}
	var plaintext []byte
	plaintext, err = proto.Marshal(receipt)
	if err != nil {
		err = fmt.Errorf("failed to marshal payload: %w", err)
		return
	}
	iv = random.Bytes(mediaRetryIVLength)
	ciphertext, err = gcmutil.Encrypt(RetryKey(mediaKey), iv, plaintext, []byte(messageID))
	return
}

// DecryptRetryNotification decifra uma notificacao de retry de midia usando a
// mediaKey da mensagem original.
func DecryptRetryNotification(evt *events.MediaRetry, mediaKey []byte) (*waMmsRetry.MediaRetryNotification, error) {
	var notif waMmsRetry.MediaRetryNotification
	if evt.Error != nil && evt.Ciphertext == nil {
		if evt.Error.Code == mediaRetryErrCodeNotAvailable {
			return nil, ErrMediaNotAvailableOnPhone
		}
		return nil, fmt.Errorf("%w (code: %d)", ErrUnknownMediaRetryError, evt.Error.Code)
	} else if plaintext, err := gcmutil.Decrypt(RetryKey(mediaKey), evt.IV, evt.Ciphertext, []byte(evt.MessageID)); err != nil {
		return nil, fmt.Errorf("failed to decrypt notification: %w", err)
	} else if err = proto.Unmarshal(plaintext, &notif); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification (invalid encryption key?): %w", err)
	} else {
		return &notif, nil
	}
}

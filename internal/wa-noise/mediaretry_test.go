// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waMmsRetry"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/util/gcmutil"
	waLog "wa-api/internal/wa-noise/util/log"
)

// Golden da chave de retry: ela precisa continuar derivando do mesmo rotulo e
// tamanho, senao nenhuma notificacao de retry ja' emitida decripta.
func TestGetMediaRetryKeyGolden(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x11}, mediaKeyLength)
	key := getMediaRetryKey(mediaKey)
	if len(key) != mediaRetryKeyLength {
		t.Fatalf("len(key) = %d, esperado %d", len(key), mediaRetryKeyLength)
	}
	const want = "7ab595184e21c4059f35584e68d6eca44cf5968a98a3d90498bd0dfb49da2bd1"
	if got := hex.EncodeToString(key); got != want {
		t.Errorf("chave = %s, golden %s", got, want)
	}
}

func TestEncryptMediaRetryReceiptEDecriptavel(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x12}, mediaKeyLength)
	const messageID types.MessageID = "MSG-ID-1"

	ciphertext, iv, err := encryptMediaRetryReceipt(messageID, mediaKey)
	if err != nil {
		t.Fatalf("encryptMediaRetryReceipt devolveu erro: %v", err)
	}
	if len(iv) != mediaRetryIVLength {
		t.Fatalf("len(iv) = %d, esperado %d", len(iv), mediaRetryIVLength)
	}

	plaintext, err := gcmutil.Decrypt(getMediaRetryKey(mediaKey), iv, ciphertext, []byte(messageID))
	if err != nil {
		t.Fatalf("falha ao decriptar o receipt: %v", err)
	}
	var receipt waMmsRetry.ServerErrorReceipt
	if err = proto.Unmarshal(plaintext, &receipt); err != nil {
		t.Fatalf("falha ao desserializar o receipt: %v", err)
	}
	if receipt.GetStanzaID() != string(messageID) {
		t.Errorf("StanzaID = %q, esperado %q", receipt.GetStanzaID(), messageID)
	}

	// O messageID entra como additional data do GCM: outro ID nao autentica.
	if _, err = gcmutil.Decrypt(getMediaRetryKey(mediaKey), iv, ciphertext, []byte("OUTRO-ID")); err == nil {
		t.Error("decriptou com additional data errado")
	}
}

func TestDecryptMediaRetryNotification(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x13}, mediaKeyLength)
	const messageID types.MessageID = "MSG-ID-2"

	notif := &waMmsRetry.MediaRetryNotification{
		StanzaID:   proto.String(string(messageID)),
		DirectPath: proto.String("/v/novo-caminho"),
		Result:     waMmsRetry.MediaRetryNotification_SUCCESS.Enum(),
	}
	plaintext, err := proto.Marshal(notif)
	if err != nil {
		t.Fatalf("falha ao serializar a notificacao: %v", err)
	}
	iv := bytes.Repeat([]byte{0x14}, mediaRetryIVLength)
	ciphertext, err := gcmutil.Encrypt(getMediaRetryKey(mediaKey), iv, plaintext, []byte(messageID))
	if err != nil {
		t.Fatalf("falha ao cifrar a notificacao: %v", err)
	}

	t.Run("caminho feliz", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ciphertext}
		got, err := DecryptMediaRetryNotification(evt, mediaKey)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if got.GetDirectPath() != "/v/novo-caminho" {
			t.Errorf("DirectPath = %q", got.GetDirectPath())
		}
	})

	t.Run("midia indisponivel no telefone", func(t *testing.T) {
		evt := &events.MediaRetry{
			MessageID: messageID,
			Error:     &events.MediaRetryError{Code: mediaRetryErrCodeNotAvailable},
		}
		if _, err := DecryptMediaRetryNotification(evt, mediaKey); !errors.Is(err, ErrMediaNotAvailableOnPhone) {
			t.Fatalf("erro = %v, esperado ErrMediaNotAvailableOnPhone", err)
		}
	})

	t.Run("outro codigo de erro", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, Error: &events.MediaRetryError{Code: 42}}
		if _, err := DecryptMediaRetryNotification(evt, mediaKey); !errors.Is(err, ErrUnknownMediaRetryError) {
			t.Fatalf("erro = %v, esperado ErrUnknownMediaRetryError", err)
		}
	})

	t.Run("chave errada", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ciphertext}
		if _, err := DecryptMediaRetryNotification(evt, bytes.Repeat([]byte{0x99}, mediaKeyLength)); err == nil {
			t.Fatal("decriptou com a mediaKey errada")
		}
	})
}

func retryNotificationNode(children ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag:     "notification",
		Attrs:   waBinary.Attrs{"t": "1754500000", "id": "MSG-ID-3"},
		Content: children,
	}
}

func TestParseMediaRetryNotification(t *testing.T) {
	chatJID := types.NewJID("5511999999999", types.DefaultUserServer)
	senderJID := types.NewJID("5511888888888", types.DefaultUserServer)
	rmr := waBinary.Node{
		Tag: "rmr",
		Attrs: waBinary.Attrs{
			"jid":         chatJID,
			"from_me":     "true",
			"participant": senderJID,
		},
	}

	t.Run("notificacao cifrada completa", func(t *testing.T) {
		node := retryNotificationNode(rmr, waBinary.Node{
			Tag: "encrypt",
			Content: []waBinary.Node{
				{Tag: "enc_p", Content: []byte("cifra")},
				{Tag: "enc_iv", Content: []byte("iv")},
			},
		})
		evt, err := parseMediaRetryNotification(node)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if evt.MessageID != "MSG-ID-3" {
			t.Errorf("MessageID = %q", evt.MessageID)
		}
		if evt.ChatID != chatJID || evt.SenderID != senderJID || !evt.FromMe {
			t.Errorf("atributos do <rmr> nao foram lidos: %+v", evt)
		}
		if string(evt.Ciphertext) != "cifra" || string(evt.IV) != "iv" {
			t.Errorf("ciphertext/iv nao foram lidos: %+v", evt)
		}
		if evt.Error != nil {
			t.Errorf("Error deveria ser nil: %+v", evt.Error)
		}
	})

	t.Run("notificacao de erro", func(t *testing.T) {
		node := retryNotificationNode(rmr, waBinary.Node{
			Tag:   "error",
			Attrs: waBinary.Attrs{"code": strconv.Itoa(mediaRetryErrCodeNotAvailable)},
		})
		evt, err := parseMediaRetryNotification(node)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if evt.Error == nil || evt.Error.Code != mediaRetryErrCodeNotAvailable {
			t.Fatalf("Error = %+v, esperado codigo %d", evt.Error, mediaRetryErrCodeNotAvailable)
		}
		if evt.Ciphertext != nil {
			t.Error("notificacao de erro nao deveria trazer ciphertext")
		}
	})

	t.Run("sem <rmr>", func(t *testing.T) {
		node := retryNotificationNode()
		_, err := parseMediaRetryNotification(node)
		var missing *ElementMissingError
		if !errors.As(err, &missing) || missing.Tag != "rmr" {
			t.Fatalf("erro = %v, esperado ElementMissingError de <rmr>", err)
		}
	})

	t.Run("sem enc_p", func(t *testing.T) {
		node := retryNotificationNode(rmr, waBinary.Node{Tag: "encrypt"})
		_, err := parseMediaRetryNotification(node)
		var missing *ElementMissingError
		if !errors.As(err, &missing) || missing.Tag != "enc_p" {
			t.Fatalf("erro = %v, esperado ElementMissingError de <enc_p>", err)
		}
	})
}

// Uma notificacao malformada nao pode derrubar o handler nem despachar evento.
func TestHandleMediaRetryNotificationIgnoraNodeInvalido(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	cli.handleMediaRetryNotification(context.Background(), retryNotificationNode())
}

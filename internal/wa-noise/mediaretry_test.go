// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/media"
	"wa-api/internal/wa-noise/proto/waMmsRetry"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/security/gcm"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// A cripto do retry (derivacao da chave, cifragem do receipt e decifragem da
// notificacao) e' testada em internal/wa-noise/media/retry_test.go. Aqui fica o
// que so' existe no pacote raiz.

const testMediaRetryErrCodeNotAvailable = 2

// O wrapper exportado do pacote raiz precisa continuar delegando para o pacote
// media e devolvendo os mesmos sentinelas de erro.
func TestDecryptMediaRetryNotificationDelegaParaMedia(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x13}, 32)
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
	iv := bytes.Repeat([]byte{0x14}, 12)
	ciphertext, err := gcmutil.Encrypt(media.RetryKey(mediaKey), iv, plaintext, []byte(messageID))
	if err != nil {
		t.Fatalf("falha ao cifrar a notificacao: %v", err)
	}

	evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ciphertext}
	got, err := DecryptMediaRetryNotification(evt, mediaKey)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got.GetDirectPath() != "/v/novo-caminho" {
		t.Errorf("DirectPath = %q", got.GetDirectPath())
	}

	semMidia := &events.MediaRetry{
		MessageID: messageID,
		Error:     &events.MediaRetryError{Code: testMediaRetryErrCodeNotAvailable},
	}
	if _, err = DecryptMediaRetryNotification(semMidia, mediaKey); !errors.Is(err, ErrMediaNotAvailableOnPhone) {
		t.Fatalf("erro = %v, esperado ErrMediaNotAvailableOnPhone", err)
	}
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
			Attrs: waBinary.Attrs{"code": strconv.Itoa(testMediaRetryErrCodeNotAvailable)},
		})
		evt, err := parseMediaRetryNotification(node)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if evt.Error == nil || evt.Error.Code != testMediaRetryErrCodeNotAvailable {
			t.Fatalf("Error = %+v, esperado codigo %d", evt.Error, testMediaRetryErrCodeNotAvailable)
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

func TestSendMediaRetryReceiptRecusaClientNil(t *testing.T) {
	var cli *Client
	err := cli.SendMediaRetryReceipt(context.Background(), &types.MessageInfo{}, nil)
	if !errors.Is(err, ErrClientIsNil) {
		t.Fatalf("erro = %v, esperado ErrClientIsNil", err)
	}
}

// Sem JID no store, o receipt nao pode ser montado.
func TestSendMediaRetryReceiptExigeLogin(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	err := cli.SendMediaRetryReceipt(context.Background(), &types.MessageInfo{}, nil)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("erro = %v, esperado ErrNotLoggedIn", err)
	}
}

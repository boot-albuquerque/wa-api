// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"time"

	"go.mau.fi/libsignal/keys/prekey"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/prekeys"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Passos preparatorios de HandleReceipt (handle.go), separados dele por causa
// do teto de 300 linhas por arquivo do ADR-0004. Nenhum e' chamado de fora
// deste pacote.

// buildGroupOrSelfExtras prepara os anexos especificos de grupo (mensagem de
// distribuicao de chave) ou de mensagem para si mesmo (DeviceSentMessage).
//
// Para o protocolo WA o resultado e' escrito DENTRO de msg.WA (mutacao de
// entrada, como no original); para o FB volta pelos dois retornos. Falha ao
// criar a SKDM e' logada e ignorada, nao aborta o retry — comportamento do
// upstream.
func buildGroupOrSelfExtras(
	ctx context.Context,
	t Transport,
	receipt *events.Receipt,
	msg *RecentMessage,
	messageID types.MessageID,
) (
	*waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	*waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
) {
	if receipt.IsGroup {
		skdm, err := t.CreateSKDM(ctx, receipt.Chat)
		if err != nil {
			t.Log().Warnf("Failed to create sender key distribution message to include in retry of %s in %s to %s: %v", messageID, receipt.Chat, receipt.Sender, err)
			return nil, nil
		}
		if msg.WA != nil {
			msg.WA.SenderKeyDistributionMessage = &waE2E.SenderKeyDistributionMessage{
				GroupID:                             proto.String(receipt.Chat.String()),
				AxolotlSenderKeyDistributionMessage: skdm,
			}
			return nil, nil
		}
		return &waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage{
			GroupID:                             proto.String(receipt.Chat.String()),
			AxolotlSenderKeyDistributionMessage: skdm,
		}, nil
	}
	if receipt.IsFromMe {
		if msg.WA != nil {
			msg.WA = &waE2E.Message{
				DeviceSentMessage: &waE2E.DeviceSentMessage{
					DestinationJID: proto.String(receipt.Chat.String()),
					Message:        msg.WA,
				},
			}
			return nil, nil
		}
		return nil, &waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage{
			DestinationJID: proto.String(receipt.Chat.String()),
		}
	}
	return nil, nil
}

// marshalForRetry serializa a mensagem a reenviar. Para o protocolo FB tambem
// calcula o franking tag (HMAC-SHA256 do payload com a franking key), que vai
// num no separado.
func marshalForRetry(msg *RecentMessage) (plaintext, frankingTag []byte, err error) {
	if msg.WA != nil {
		plaintext, err = proto.Marshal(msg.WA)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal message: %w", err)
		}
		return plaintext, nil, nil
	}
	plaintext, err = proto.Marshal(msg.FB)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal consumer message: %w", err)
	}
	frankingHash := hmac.New(sha256.New, msg.FB.GetMetadata().GetFrankingKey())
	frankingHash.Write(plaintext)
	return plaintext, frankingHash.Sum(nil), nil
}

// resolveBundle decide qual bundle de prekey usar para recifrar: o que veio no
// proprio recibo (<keys>), um buscado do servidor quando a sessao precisa ser
// recriada, ou nenhum (nil, sessao existente serve).
func resolveBundle(
	ctx context.Context,
	t Transport,
	receipt *events.Receipt,
	node *waBinary.Node,
	retryCount int,
) (*prekey.Bundle, error) {
	if _, hasKeys := node.GetOptionalChildByTag("keys"); hasKeys {
		bundle, err := prekeys.NodeToBundle(uint32(receipt.Sender.Device), *node)
		if err != nil {
			return nil, fmt.Errorf("failed to read prekey bundle in retry receipt: %w", err)
		}
		return bundle, nil
	}
	reason, recreate := ShouldRecreateSession(ctx, t, retryCount, receipt.Sender)
	if !recreate {
		return nil, nil
	}
	t.Log().Debugf("Fetching prekeys for %s for handling retry receipt with no prekey bundle because %s", receipt.Sender, reason)
	keys, err := t.FetchPreKeys(ctx, []types.JID{receipt.Sender})
	if err != nil {
		return nil, err
	}
	bundle, err := keys[receipt.Sender].Bundle, keys[receipt.Sender].Err
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prekeys: %w", err)
	} else if bundle == nil {
		return nil, fmt.Errorf("didn't get prekey bundle for %s (response size: %d)", receipt.Sender, len(keys))
	}
	return bundle, nil
}

// buildRetryMessageAttrs monta os atributos do <message> de retry, copiando do
// no original os tres opcionais que o servidor pode ter mandado.
//
// device_fanout=false so' entra fora de grupo, como no original.
func buildRetryMessageAttrs(
	node *waBinary.Node,
	receipt *events.Receipt,
	msgType string,
	messageID types.MessageID,
	timestamp time.Time,
) waBinary.Attrs {
	attrs := waBinary.Attrs{
		"to":   node.Attrs["from"],
		"type": msgType,
		"id":   messageID,
		"t":    timestamp.Unix(),
	}
	if !receipt.IsGroup {
		attrs["device_fanout"] = false
	}
	for _, key := range []string{"participant", "recipient", "edit"} {
		if v, ok := node.Attrs[key]; ok {
			attrs[key] = v
		}
	}
	return attrs
}

// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"encoding/binary"

	"go.mau.fi/libsignal/ecc"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/prekeys"
	"wa-api/internal/wa-noise/protocol/types"
)

// SendReceipt envia um recibo de retry para uma mensagem RECEBIDA que nao
// conseguimos decifrar, pedindo ao remetente que a mande de novo.
//
// Ordem de decisao, verbatim do upstream:
//
//  1. Contabiliza a tentativa (BumpMessageRetries, que ja' trata o caso do
//     processo ter reiniciado no meio de uma cadeia de retries).
//  2. Passando de MaxOutgoingReceipts, desiste — sem enviar nada.
//  3. Na PRIMEIRA tentativa, tambem pede a mensagem ao proprio telefone; de
//     forma sincrona quando SynchronousAck esta' ligado, em goroutine adiada
//     caso contrario.
//  4. Monta e envia o <receipt type="retry">, incluindo o bloco <keys> a partir
//     da segunda tentativa (ou quando forceIncludeIdentity manda).
//
// Erros de envio sao logados e engolidos: a funcao nao devolve nada, como
// antes — quem chama esta' no caminho de decriptacao e nao tem o que fazer com
// a falha.
func SendReceipt(ctx context.Context, t Transport, node *waBinary.Node, info MessageRef, forceIncludeIdentity bool) {
	id, _ := node.Attrs["id"].(string)
	children := node.GetChildren()
	var retryCountInMsg int
	if len(children) == 1 && children[0].Tag == "enc" {
		retryCountInMsg = children[0].AttrGetter().OptionalInt("count")
	}

	retryCount := t.State().BumpMessageRetries(id, retryCountInMsg)
	if retryCount >= MaxOutgoingReceipts {
		t.Log().Warnf("Not sending any more retry receipts for %s", id)
		return
	}
	if retryCount == 1 {
		if t.SynchronousAck() {
			ImmediateRequestFromPhone(ctx, t, info)
		} else {
			go DelayedRequestFromPhone(t, info)
		}
	}

	// Mesmo campo (Store.RegistrationID) e mesma codificacao big-endian do
	// upload de prekeys, entao reutiliza a constante de la' em vez de declarar
	// um segundo 4.
	var registrationIDBytes [prekeys.RegistrationIDLength]byte
	binary.BigEndian.PutUint32(registrationIDBytes[:], t.Store().RegistrationID)
	attrs := t.BuildBaseReceipt(info.ID, node)
	attrs["type"] = string(types.ReceiptTypeRetry)
	if info.Type == "peer_msg" && info.IsFromMe {
		attrs["category"] = "peer"
	}
	payload := waBinary.Node{
		Tag:   "receipt",
		Attrs: attrs,
		Content: []waBinary.Node{
			{Tag: "retry", Attrs: waBinary.Attrs{
				"count": retryCount,
				"id":    id,
				"t":     node.Attrs["t"],
				"v":     ReceiptVersion,
			}},
			{Tag: "registration", Content: registrationIDBytes[:]},
		},
	}
	if retryCount > 1 || forceIncludeIdentity {
		if key, err := t.Store().PreKeys.GenOnePreKey(ctx); err != nil {
			// Falha ao gerar prekey NAO aborta: o recibo sai sem <keys>. Falha
			// ao serializar a conta ABORTA. A assimetria e' do upstream.
			t.Log().Errorf("Failed to get prekey for retry receipt: %v", err)
		} else if deviceIdentity, err := proto.Marshal(t.Store().Account); err != nil {
			t.Log().Errorf("Failed to marshal account info: %v", err)
			return
		} else {
			payload.Content = append(payload.GetChildren(), waBinary.Node{
				Tag: "keys",
				Content: []waBinary.Node{
					{Tag: "type", Content: []byte{ecc.DjbType}},
					{Tag: "identity", Content: t.Store().IdentityKey.Pub[:]},
					prekeys.ToNode(key),
					prekeys.ToNode(t.Store().SignedPreKey),
					{Tag: "device-identity", Content: deviceIdentity},
				},
			})
		}
	}
	err := t.SendNode(ctx, payload)
	if err != nil {
		t.Log().Errorf("Failed to send retry receipt for %s: %v", id, err)
	}
}

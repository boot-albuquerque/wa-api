// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/msgattrs"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// FBApplicationVersion e' o `version` do SubProtocol de aplicacao FB.
//
// A constante mora aqui e a raiz passou a defini-la como
// `const FBMessageApplicationVersion = retry.FBApplicationVersion` — ou seja,
// ha uma fonte unica, e a API publica da raiz continua existindo com o mesmo
// valor e o mesmo tipo (constante sem tipo). Duplicar o literal 2 nos dois
// lados seria a alternativa, e um so' deles poderia mudar sem quebrar o build.
const FBApplicationVersion = 2

// ShouldRecreateSession decide se a sessao Signal com jid deve ser recriada
// antes de responder a um retry, e devolve o motivo para o log.
//
// O corpo INTEIRO roda sob o lock de recriacao de sessao, do primeiro
// statement ao ultimo — inclusive a consulta ContainsSession ao store —,
// exatamente como o shouldRecreateSession da raiz rodava. Nao e' o desenho
// ideal (segurar um mutex durante ida ao banco), mas muda-lo seria mudanca de
// comportamento, e este lote e' extracao.
//
// Erro do store devolve (\"\", false): na duvida, nao recria. Comportamento do
// upstream.
func ShouldRecreateSession(ctx context.Context, t Transport, retryCount int, jid types.JID) (reason string, recreate bool) {
	s := t.State()
	s.LockSessionRecreate()
	defer s.UnlockSessionRecreate()
	if contains, err := t.Store().ContainsSession(ctx, jid.SignalAddress()); err != nil {
		return "", false
	} else if !contains {
		s.MarkSessionRecreated(jid, time.Now())
		return "we don't have a Signal session with them", true
	} else if retryCount < MinCountForSessionRecreate {
		return "", false
	}
	prevTime, ok := s.LastSessionRecreate(jid)
	if !ok || prevTime.Add(recreateSessionTimeout).Before(time.Now()) {
		s.MarkSessionRecreated(jid, time.Now())
		return "retry count > 1 and over an hour since last recreation", true
	}
	return "", false
}

// TryHandleReceipt embrulha HandleReceipt com recover, com o semaforo de
// paralelismo e com o log de erro.
//
// O recover existe porque o call site original a chamava com `go`: um panic
// aqui derrubaria o processo inteiro, sem ninguem para pega-lo.
func TryHandleReceipt(ctx context.Context, t Transport, receipt *events.Receipt, node *waBinary.Node) {
	defer func() {
		err := recover()
		if err != nil {
			t.Log().Errorf("Retry receipt handler panicked: %v\n%s", err, debug.Stack())
		}
	}()
	if sema := t.State().Sema(); sema != nil {
		err := sema.Acquire(ctx, 1)
		if err != nil {
			return
		}
		defer sema.Release(1)
	}
	err := HandleReceipt(ctx, t, receipt, node)
	if err != nil {
		// MessageIDs vem de parseReceipt, que sempre devolve pelo menos um ID,
		// mas o indexar cru aqui e' um panic esperando um chamador futuro que
		// nao respeite isso — e este e' o caminho de *erro*, o pior lugar para
		// morrer. Ver PATCHES.md (Fase E, lote 5).
		var firstID types.MessageID
		if len(receipt.MessageIDs) > 0 {
			firstID = receipt.MessageIDs[0]
		}
		t.Log().Errorf("Failed to handle retry receipt for %s/%s from %s: %v", receipt.Chat, firstID, receipt.Sender, err)
	}
}

// HandleReceipt atende um recibo de retry de uma mensagem que NOS enviamos:
// recupera a mensagem original, recifra e reenvia.
func HandleReceipt(ctx context.Context, t Transport, receipt *events.Receipt, node *waBinary.Node) error {
	retryChild, ok := node.GetOptionalChildByTag("retry")
	if !ok {
		return t.ElementMissing("retry", "retry receipt")
	}
	ag := retryChild.AttrGetter()
	messageID := ag.String("id")
	timestamp := ag.UnixTime("t")
	retryCount := ag.Int("count")
	if !ag.OK() {
		return ag.Error()
	}
	msg, err := GetForRetry(ctx, t, receipt, messageID)
	if err != nil {
		return err
	} else if msg == nil {
		return fmt.Errorf("couldn't find message %s", messageID)
	}
	var fbConsumerMsg *waConsumerApplication.ConsumerApplication
	if msg.FB != nil {
		subProto, ok := msg.FB.GetPayload().GetSubProtocol().GetSubProtocol().(*waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage)
		if ok {
			fbConsumerMsg, err = subProto.Decode()
			if err != nil {
				return fmt.Errorf("failed to decode consumer message for retry: %w", err)
			}
		}
	}

	internalCounter := t.State().IncrementIncoming(IncomingKey{receipt.Sender, messageID})
	if internalCounter >= MaxIncomingRequests {
		t.Log().Warnf("Dropping retry request from %s for %s: internal retry counter is %d", receipt.Sender, messageID, internalCounter)
		return nil
	}

	fbSKDM, fbDSM := buildGroupOrSelfExtras(ctx, t, receipt, msg, messageID)

	// TODO pre-retry callback for fb
	if !t.PreRetryAllowed(receipt, messageID, retryCount, msg.WA) {
		t.Log().Debugf("Cancelled retry receipt in PreRetryCallback")
		return nil
	}

	plaintext, frankingTag, err := marshalForRetry(msg)
	if err != nil {
		return err
	}
	bundle, err := resolveBundle(ctx, t, receipt, node, retryCount)
	if err != nil {
		return err
	}

	encAttrs := waBinary.Attrs{}
	var msgAttrs msgattrs.MessageAttrs
	if msg.WA != nil {
		msgAttrs.MediaType = msgattrs.GetMediaTypeFromMessage(msg.WA)
		msgAttrs.Type = msgattrs.GetTypeFromMessage(msg.WA)
	} else if fbConsumerMsg != nil {
		msgAttrs = msgattrs.GetAttrsFromFBMessage(fbConsumerMsg)
	} else {
		msgAttrs.Type = "text"
	}
	if msgAttrs.MediaType != "" {
		encAttrs["mediatype"] = msgAttrs.MediaType
	}
	var encrypted *waBinary.Node
	var includeDeviceIdentity bool
	if msg.WA != nil {
		encryptionIdentity := receipt.Sender
		if receipt.Sender.Server == types.DefaultUserServer {
			lidForPN, err := t.Store().LIDs.GetLIDForPN(ctx, receipt.Sender)
			if err != nil {
				t.Log().Warnf("Failed to get LID for %s: %v", receipt.Sender, err)
			} else if !lidForPN.IsEmpty() {
				t.MigrateSessionStore(ctx, receipt.Sender, lidForPN)
				encryptionIdentity = lidForPN
			}
		}
		encrypted, includeDeviceIdentity, err = t.EncryptForDevice(ctx, plaintext, encryptionIdentity, bundle, encAttrs)
	} else {
		encrypted, err = t.EncryptForDeviceV3(ctx, &waMsgTransport.MessageTransport_Payload{
			ApplicationPayload: &waCommon.SubProtocol{
				Payload: plaintext,
				Version: proto.Int32(FBApplicationVersion),
			},
			FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
		}, fbSKDM, fbDSM, receipt.Sender, bundle, encAttrs)
	}
	if err != nil {
		return fmt.Errorf("failed to encrypt message for retry: %w", err)
	}
	encrypted.Attrs["count"] = retryCount

	attrs := buildRetryMessageAttrs(node, receipt, msgAttrs.Type, messageID, timestamp)
	var content []waBinary.Node
	if msg.WA != nil {
		content = t.MessageContent(*encrypted, msg.WA, attrs, includeDeviceIdentity)
	} else {
		content = []waBinary.Node{
			*encrypted,
			{Tag: "franking", Content: []waBinary.Node{{Tag: "franking_tag", Content: frankingTag}}},
		}
	}
	err = t.SendNode(ctx, waBinary.Node{
		Tag:     "message",
		Attrs:   attrs,
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("failed to send retry message: %w", err)
	}
	t.Log().Debugf("Sent retry #%d for %s/%s to %s", retryCount, receipt.Chat, messageID, receipt.Sender)
	return nil
}

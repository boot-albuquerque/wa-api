// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"context"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// HandleProtocolMessage trata as partes de protocolo de uma mensagem ja'
// decifrada: history sync, migracao LID, respostas de operacao peer-to-peer e
// compartilhamento de chave de app state.
//
// Devolve `ok = false` quando algum handler chamado por ela falhou.
func HandleProtocolMessage(ctx context.Context, t Transport, info *types.MessageInfo, msg *waE2E.Message) (ok bool) {
	ok = true
	protoMsg := msg.GetProtocolMessage()

	if !info.IsFromMe {
		return
	}

	if protoMsg.GetHistorySyncNotification() != nil {
		if !t.ManualHistorySyncDownload() {
			EnqueueHistorySync(t, protoMsg.HistorySyncNotification)
		}
		if !(t.ManualHistorySyncDownload() && t.DisableManualHistorySyncReceipt()) {
			go func() {
				err := SendProtocolReceipt(ctx, t, info.ID, types.ReceiptTypeHistorySync)
				if err != nil {
					t.Log().Warnf("Failed to send acknowledgement for protocol message %s: %v", info.ID, err)
				}
			}()
		}
	}

	if protoMsg.GetLidMigrationMappingSyncMessage() != nil {
		StoreLIDSyncMessage(ctx, t, protoMsg.GetLidMigrationMappingSyncMessage().GetEncodedMappingPayload())
	}

	if info.Sender.Device == 0 {
		peerResp := protoMsg.GetPeerDataOperationRequestResponseMessage()
		switch peerResp.GetPeerDataOperationRequestType() {
		case waE2E.PeerDataOperationRequestType_PLACEHOLDER_MESSAGE_RESEND:
			ok = HandlePlaceholderResendResponse(t, peerResp) && ok
		case waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY:
			ok = t.HandleAppStateRecovery(ctx, peerResp.GetStanzaID(), peerResp.GetPeerDataOperationResult()) && ok
		}
	}

	if protoMsg.GetAppStateSyncKeyShare() != nil {
		go HandleAppStateSyncKeyShare(context.WithoutCancel(ctx), t, protoMsg.AppStateSyncKeyShare)
	}

	if info.Category == send.MsgCategoryPeer {
		go func() {
			err := SendProtocolReceipt(ctx, t, info.ID, types.ReceiptTypePeerMsg)
			if err != nil {
				t.Log().Warnf("Failed to send acknowledgement for protocol message %s: %v", info.ID, err)
			}
		}()
	}
	return
}

// ProcessProtocolParts grava o segredo de mensagem, processa a sender key
// distribuida e delega as partes de protocolo.
func ProcessProtocolParts(ctx context.Context, t Transport, info *types.MessageInfo, msg *waE2E.Message) (ok bool) {
	ok = true
	StoreSecret(ctx, t, info, msg)
	// Hopefully sender key distribution messages and protocol messages can't be inside ephemeral messages
	if msg.GetDeviceSentMessage().GetMessage() != nil {
		msg = msg.GetDeviceSentMessage().GetMessage()
	}
	if msg.GetSenderKeyDistributionMessage() != nil {
		if !info.IsGroup {
			t.Log().Warnf("Got sender key distribution message in non-group chat from %s", info.Sender)
		} else {
			encryptionIdentity := info.Sender
			if encryptionIdentity.Server == types.DefaultUserServer && info.SenderAlt.Server == types.HiddenUserServer {
				encryptionIdentity = info.SenderAlt
			}
			HandleSenderKeyDistributionMessage(ctx, t, info.Chat, encryptionIdentity, msg.SenderKeyDistributionMessage.AxolotlSenderKeyDistributionMessage)
		}
	}
	// N.B. Edits are protocol messages, but they're also wrapped inside EditedMessage,
	// which is only unwrapped after ProcessProtocolParts, so this won't trigger for edits.
	if msg.GetProtocolMessage() != nil {
		ok = HandleProtocolMessage(ctx, t, info, msg) && ok
	}
	return
}

// HandleDecrypted processa e despacha uma mensagem waE2E ja' decifrada.
func HandleDecrypted(ctx context.Context, t Transport, info *types.MessageInfo, msg *waE2E.Message, retryCount int) (handlerFailed bool) {
	ok := ProcessProtocolParts(ctx, t, info, msg)
	if !ok {
		return false
	}
	evt := &events.Message{Info: *info, RawMessage: msg, RetryCount: retryCount}
	return t.DispatchEvent(evt.UnwrapRaw())
}

// SendProtocolReceipt manda um recibo de protocol message de volta ao telefone.
func SendProtocolReceipt(ctx context.Context, t Transport, id types.MessageID, msgType types.ReceiptType) error {
	if len(id) == 0 {
		return nil
	}
	err := t.SendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"id":   string(id),
			"type": string(msgType),
			"to":   t.OwnID().ToNonAD(),
		},
		Content: nil,
	})
	if err != nil {
		return err
	}
	return nil
}

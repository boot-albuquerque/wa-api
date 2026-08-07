// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) handleProtocolMessage(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) (ok bool) {
	ok = true
	protoMsg := msg.GetProtocolMessage()

	if !info.IsFromMe {
		return
	}

	if protoMsg.GetHistorySyncNotification() != nil {
		if !cli.ManualHistorySyncDownload {
			cli.historySyncNotifications <- protoMsg.HistorySyncNotification
			if cli.historySyncHandlerStarted.CompareAndSwap(false, true) {
				go cli.handleHistorySyncNotificationLoop()
			}
		}
		if !(cli.ManualHistorySyncDownload && cli.DisableManualHistorySyncReceipt) {
			go func() {
				err := cli.SendProtocolMessageReceipt(ctx, info.ID, types.ReceiptTypeHistorySync)
				if err != nil {
					cli.Log.Warnf("Failed to send acknowledgement for protocol message %s: %v", info.ID, err)
				}
			}()
		}
	}

	if protoMsg.GetLidMigrationMappingSyncMessage() != nil {
		cli.storeLIDSyncMessage(ctx, protoMsg.GetLidMigrationMappingSyncMessage().GetEncodedMappingPayload())
	}

	if info.Sender.Device == 0 {
		peerResp := protoMsg.GetPeerDataOperationRequestResponseMessage()
		switch peerResp.GetPeerDataOperationRequestType() {
		case waE2E.PeerDataOperationRequestType_PLACEHOLDER_MESSAGE_RESEND:
			ok = cli.handlePlaceholderResendResponse(peerResp) && ok
		case waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY:
			ok = cli.handleAppStateRecovery(ctx, peerResp.GetStanzaID(), peerResp.GetPeerDataOperationResult()) && ok
		}
	}

	if protoMsg.GetAppStateSyncKeyShare() != nil {
		go cli.handleAppStateSyncKeyShare(context.WithoutCancel(ctx), protoMsg.AppStateSyncKeyShare)
	}

	if info.Category == msgCategoryPeer {
		go func() {
			err := cli.SendProtocolMessageReceipt(ctx, info.ID, types.ReceiptTypePeerMsg)
			if err != nil {
				cli.Log.Warnf("Failed to send acknowledgement for protocol message %s: %v", info.ID, err)
			}
		}()
	}
	return
}

func (cli *Client) processProtocolParts(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) (ok bool) {
	ok = true
	cli.storeMessageSecret(ctx, info, msg)
	// Hopefully sender key distribution messages and protocol messages can't be inside ephemeral messages
	if msg.GetDeviceSentMessage().GetMessage() != nil {
		msg = msg.GetDeviceSentMessage().GetMessage()
	}
	if msg.GetSenderKeyDistributionMessage() != nil {
		if !info.IsGroup {
			cli.Log.Warnf("Got sender key distribution message in non-group chat from %s", info.Sender)
		} else {
			encryptionIdentity := info.Sender
			if encryptionIdentity.Server == types.DefaultUserServer && info.SenderAlt.Server == types.HiddenUserServer {
				encryptionIdentity = info.SenderAlt
			}
			cli.handleSenderKeyDistributionMessage(ctx, info.Chat, encryptionIdentity, msg.SenderKeyDistributionMessage.AxolotlSenderKeyDistributionMessage)
		}
	}
	// N.B. Edits are protocol messages, but they're also wrapped inside EditedMessage,
	// which is only unwrapped after processProtocolParts, so this won't trigger for edits.
	if msg.GetProtocolMessage() != nil {
		ok = cli.handleProtocolMessage(ctx, info, msg) && ok
	}
	return
}

func (cli *Client) handleDecryptedMessage(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message, retryCount int) (handlerFailed bool) {
	ok := cli.processProtocolParts(ctx, info, msg)
	if !ok {
		return false
	}
	evt := &events.Message{Info: *info, RawMessage: msg, RetryCount: retryCount}
	return cli.dispatchEvent(evt.UnwrapRaw())
}

// SendProtocolMessageReceipt sends a receipt for a protocol message back to the phone.
func (cli *Client) SendProtocolMessageReceipt(ctx context.Context, id types.MessageID, msgType types.ReceiptType) error {
	if len(id) == 0 {
		return nil
	}
	err := cli.sendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"id":   string(id),
			"type": string(msgType),
			"to":   cli.getOwnID().ToNonAD(),
		},
		Content: nil,
	})
	if err != nil {
		return err
	}
	return nil
}

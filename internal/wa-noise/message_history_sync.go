// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"runtime/debug"
	"time"

	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/media"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/proto/waWeb"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) handleSenderKeyDistributionMessage(ctx context.Context, chat, from types.JID, axolotlSKDM []byte) {
	builder := groups.NewGroupSessionBuilder(cli.Store, pbSerializer)
	senderKeyName := protocol.NewSenderKeyName(chat.String(), from.SignalAddress())
	sdkMsg, err := protocol.NewSenderKeyDistributionMessageFromBytes(axolotlSKDM, pbSerializer.SenderKeyDistributionMessage)
	if err != nil {
		cli.Log.Errorf("Failed to parse sender key distribution message from %s for %s: %v", from, chat, err)
		return
	}
	err = builder.Process(ctx, senderKeyName, sdkMsg)
	if err != nil {
		cli.Log.Errorf("Failed to process sender key distribution message from %s for %s: %v", from, chat, err)
		return
	}
	cli.Log.Debugf("Processed sender key distribution message from %s in %s", senderKeyName.Sender().String(), senderKeyName.GroupID())
}

func (cli *Client) handleHistorySyncNotificationLoop() {
	defer func() {
		cli.historySyncHandlerStarted.Store(false)
		err := recover()
		if err != nil {
			cli.Log.Errorf("History sync handler panicked: %v\n%s", err, debug.Stack())
		}

		// Check in case something new appeared in the channel between the loop stopping
		// and the atomic variable being updated. If yes, restart the loop.
		if len(cli.historySyncNotifications) > 0 && cli.historySyncHandlerStarted.CompareAndSwap(false, true) {
			cli.Log.Warnf("New history sync notifications appeared after loop stopped, restarting loop...")
			go cli.handleHistorySyncNotificationLoop()
		}
	}()
	ctx := cli.BackgroundEventCtx
	for {
		select {
		case notif := <-cli.historySyncNotifications:
			blob, err := cli.DownloadHistorySync(ctx, notif, false)
			if err != nil {
				cli.Log.Errorf("Failed to download history sync: %v", err)
			} else {
				cli.dispatchEvent(&events.HistorySync{Data: blob})
				err = cli.DeleteMedia(ctx, MediaHistory, notif.GetDirectPath(), notif.GetFileEncSHA256(), notif.GetEncHandle())
				if err != nil {
					cli.Log.Warnf("Failed to delete history sync media from server: %v", err)
				}
			}
		case <-time.After(historySyncLoopIdleTimeout):
			return
		}
	}
}

// SendHistorySyncServerErrorReceipt sends a history sync server-error receipt, which
// asks the phone to re-upload the referenced history sync payload.
func (cli *Client) SendHistorySyncServerErrorReceipt(ctx context.Context, msgID types.MessageID, mediaKey []byte) error {
	ciphertext, iv, err := media.EncryptRetryReceipt(msgID, mediaKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt history sync server-error receipt: %w", err)
	}
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		return ErrNotLoggedIn
	}
	err = cli.sendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"id":       string(msgID),
			"type":     "server-error",
			"to":       ownID,
			"category": "peer",
		},
		Content: []waBinary.Node{
			{Tag: "encrypt", Content: []waBinary.Node{
				{Tag: "enc_p", Content: ciphertext},
				{Tag: "enc_iv", Content: iv},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to send history sync server-error receipt: %w", err)
	}
	return nil
}

// DownloadHistorySync will download and parse the history sync blob from the given history sync notification.
//
// You only need to call this manually if you set [Client.ManualHistorySyncDownload] to true.
// By default, whatsmeow will call this automatically and dispatch an [events.HistorySync] with the parsed data.
func (cli *Client) DownloadHistorySync(ctx context.Context, notif *waE2E.HistorySyncNotification, synchronousStorage bool) (*waHistorySync.HistorySync, error) {
	var data []byte
	var err error
	if notif.InitialHistBootstrapInlinePayload != nil {
		data = notif.InitialHistBootstrapInlinePayload
	} else if data, err = cli.Download(ctx, notif); err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	var historySync waHistorySync.HistorySync
	if reader, err := zlib.NewReader(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("failed to prepare to decompress: %w", err)
	} else if rawData, err := io.ReadAll(reader); err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	} else if err = proto.Unmarshal(rawData, &historySync); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}
	cli.Log.Debugf("Received history sync (type %s, chunk %d, progress %d)", historySync.GetSyncType(), historySync.GetChunkOrder(), historySync.GetProgress())
	doStorage := func(ctx context.Context) {
		if err := cli.storeNCTSalt(ctx, historySync.GetNctSalt()); err != nil {
			cli.Log.Warnf("Failed to store NCT salt from history sync: %v", err)
		}
		if historySync.GetSyncType() == waHistorySync.HistorySync_PUSH_NAME {
			cli.handleHistoricalPushNames(ctx, historySync.GetPushnames())
		} else if len(historySync.GetConversations()) > 0 {
			cli.storeHistoricalMessageSecrets(ctx, historySync.GetConversations())
		}
		if len(historySync.GetPhoneNumberToLidMappings()) > 0 {
			cli.storeHistoricalPNLIDMappings(ctx, historySync.GetPhoneNumberToLidMappings())
		}
		if historySync.GlobalSettings != nil {
			cli.storeGlobalSettings(ctx, historySync.GlobalSettings)
		}
	}
	if synchronousStorage {
		doStorage(ctx)
	} else {
		go doStorage(context.WithoutCancel(ctx))
	}
	return &historySync, nil
}

func (cli *Client) handleAppStateSyncKeyShare(ctx context.Context, keys *waE2E.AppStateSyncKeyShare) {
	onlyResyncIfNotSynced := true

	cli.Log.Debugf("Got %d new app state keys", len(keys.GetKeys()))
	// O lock de leitura fica segurado pelo laco inteiro, incluindo as gravacoes
	// no store — e' o que o RLock/RUnlock manual daqui fazia antes da extracao
	// do dominio para internal/wa-noise/appstatesync.
	cli.appStateSync.ReadKeyRequests(func(wasRequested func(string) bool) {
		for _, key := range keys.GetKeys() {
			marshaledFingerprint, err := proto.Marshal(key.GetKeyData().GetFingerprint())
			if err != nil {
				cli.Log.Errorf("Failed to marshal fingerprint of app state sync key %X", key.GetKeyID().GetKeyID())
				continue
			}
			if wasRequested(hex.EncodeToString(key.GetKeyID().GetKeyID())) {
				onlyResyncIfNotSynced = false
			}
			err = cli.Store.AppStateKeys.PutAppStateSyncKey(ctx, key.GetKeyID().GetKeyID(), store.AppStateSyncKey{
				Data:        key.GetKeyData().GetKeyData(),
				Fingerprint: marshaledFingerprint,
				Timestamp:   key.GetKeyData().GetTimestamp(),
			})
			if err != nil {
				cli.Log.Errorf("Failed to store app state sync key %X: %v", key.GetKeyID().GetKeyID(), err)
				continue
			}
			cli.Log.Debugf("Received app state sync key %X (ts: %d)", key.GetKeyID().GetKeyID(), key.GetKeyData().GetTimestamp())
		}
	})

	for _, name := range appstate.AllPatchNames {
		err := cli.FetchAppState(ctx, name, false, onlyResyncIfNotSynced)
		if err != nil {
			cli.Log.Errorf("Failed to do initial fetch of app state %s: %v", name, err)
		}
	}
}

func (cli *Client) handlePlaceholderResendResponse(msg *waE2E.PeerDataOperationRequestResponseMessage) (ok bool) {
	reqID := msg.GetStanzaID()
	parts := msg.GetPeerDataOperationResult()
	cli.Log.Debugf("Handling response to placeholder resend request %s with %d items", reqID, len(parts))
	ok = true
	for i, part := range parts {
		var webMsg waWeb.WebMessageInfo
		if resp := part.GetPlaceholderMessageResendResponse(); resp == nil {
			cli.Log.Warnf("Missing response in item #%d of response to %s", i+1, reqID)
		} else if err := proto.Unmarshal(resp.GetWebMessageInfoBytes(), &webMsg); err != nil {
			cli.Log.Warnf("Failed to unmarshal protobuf web message in item #%d of response to %s: %v", i+1, reqID, err)
		} else if msgEvt, err := cli.ParseWebMessage(types.EmptyJID, &webMsg); err != nil {
			cli.Log.Warnf("Failed to parse web message info in item #%d of response to %s: %v", i+1, reqID, err)
		} else {
			msgEvt.UnavailableRequestID = reqID
			ok = !cli.dispatchEvent(msgEvt) && ok
		}
	}
	return
}

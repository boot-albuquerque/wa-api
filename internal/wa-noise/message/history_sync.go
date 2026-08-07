// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

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

// HandleSenderKeyDistributionMessage processa a SKDM recebida em um grupo,
// registrando a sender key do remetente para aquele grupo.
func HandleSenderKeyDistributionMessage(ctx context.Context, t Transport, chat, from types.JID, axolotlSKDM []byte) {
	builder := groups.NewGroupSessionBuilder(t.Store(), store.SignalProtobufSerializer)
	senderKeyName := protocol.NewSenderKeyName(chat.String(), from.SignalAddress())
	sdkMsg, err := protocol.NewSenderKeyDistributionMessageFromBytes(axolotlSKDM, store.SignalProtobufSerializer.SenderKeyDistributionMessage)
	if err != nil {
		t.Log().Errorf("Failed to parse sender key distribution message from %s for %s: %v", from, chat, err)
		return
	}
	err = builder.Process(ctx, senderKeyName, sdkMsg)
	if err != nil {
		t.Log().Errorf("Failed to process sender key distribution message from %s for %s: %v", from, chat, err)
		return
	}
	t.Log().Debugf("Processed sender key distribution message from %s in %s", senderKeyName.Sender().String(), senderKeyName.GroupID())
}

// HandleHistorySyncNotificationLoop consome a fila de notificacoes de history
// sync ate' ficar `historySyncLoopIdleTimeout` sem novidade.
//
// O `defer` com recover e o religamento do loop foram preservados literalmente:
// ao sair, zera o flag e, se algo entrou na fila entre a saida do select e o
// Store(false), religa. Os dois passos continuam sendo um CompareAndSwap.
func HandleHistorySyncNotificationLoop(t Transport) {
	q := t.HistorySync()
	defer func() {
		q.handlerActive.Store(false)
		err := recover()
		if err != nil {
			t.Log().Errorf("History sync handler panicked: %v\n%s", err, debug.Stack())
		}

		// Confere caso algo novo tenha aparecido no canal entre o loop parar e a
		// variavel atomica ser atualizada. Se sim, religa o loop.
		if q.Len() > 0 && q.handlerActive.CompareAndSwap(false, true) {
			t.Log().Warnf("New history sync notifications appeared after loop stopped, restarting loop...")
			go HandleHistorySyncNotificationLoop(t)
		}
	}()
	ctx := t.BackgroundEventCtx()
	for {
		select {
		case notif := <-q.notifications:
			blob, err := DownloadHistorySync(ctx, t, notif, false)
			if err != nil {
				t.Log().Errorf("Failed to download history sync: %v", err)
			} else {
				t.DispatchEvent(&events.HistorySync{Data: blob})
				err = t.DeleteHistorySyncMedia(ctx, notif.GetDirectPath(), notif.GetFileEncSHA256(), notif.GetEncHandle())
				if err != nil {
					t.Log().Warnf("Failed to delete history sync media from server: %v", err)
				}
			}
		case <-time.After(historySyncLoopIdleTimeout):
			return
		}
	}
}

// EnqueueHistorySync poe a notificacao na fila e liga o loop se ele nao
// estiver rodando. E' a metade "produtora" que antes vivia solta em
// handleProtocolMessage.
func EnqueueHistorySync(t Transport, notif *waE2E.HistorySyncNotification) {
	q := t.HistorySync()
	q.notifications <- notif
	if q.handlerActive.CompareAndSwap(false, true) {
		go HandleHistorySyncNotificationLoop(t)
	}
}

// SendHistorySyncServerErrorReceipt manda um recibo de server-error de history
// sync, que pede ao telefone para reenviar o payload referenciado.
func SendHistorySyncServerErrorReceipt(ctx context.Context, t Transport, msgID types.MessageID, mediaKey []byte) error {
	ciphertext, iv, err := media.EncryptRetryReceipt(msgID, mediaKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt history sync server-error receipt: %w", err)
	}
	ownID := t.OwnID().ToNonAD()
	if ownID.IsEmpty() {
		return t.Errors().NotLoggedIn
	}
	err = t.SendNode(ctx, waBinary.Node{
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

// DownloadHistorySync baixa e desserializa o blob de history sync apontado pela
// notificacao.
//
// `synchronousStorage` decide se a gravacao derivada (salt de cstoken, push
// names, segredos, mapeamentos LID, configuracoes globais) roda no mesmo
// goroutine ou em um solto com contexto destacado — preservado literalmente.
func DownloadHistorySync(ctx context.Context, t Transport, notif *waE2E.HistorySyncNotification, synchronousStorage bool) (*waHistorySync.HistorySync, error) {
	var data []byte
	var err error
	if notif.InitialHistBootstrapInlinePayload != nil {
		data = notif.InitialHistBootstrapInlinePayload
	} else if data, err = t.DownloadHistorySyncBlob(ctx, notif); err != nil {
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
	t.Log().Debugf("Received history sync (type %s, chunk %d, progress %d)", historySync.GetSyncType(), historySync.GetChunkOrder(), historySync.GetProgress())
	doStorage := func(ctx context.Context) {
		if err := t.StoreNCTSalt(ctx, historySync.GetNctSalt()); err != nil {
			t.Log().Warnf("Failed to store NCT salt from history sync: %v", err)
		}
		if historySync.GetSyncType() == waHistorySync.HistorySync_PUSH_NAME {
			t.HandleHistoricalPushNames(ctx, historySync.GetPushnames())
		} else if len(historySync.GetConversations()) > 0 {
			StoreHistoricalSecrets(ctx, t, historySync.GetConversations())
		}
		if len(historySync.GetPhoneNumberToLidMappings()) > 0 {
			StoreHistoricalPNLIDMappings(ctx, t, historySync.GetPhoneNumberToLidMappings())
		}
		if historySync.GlobalSettings != nil {
			StoreGlobalSettings(ctx, t, historySync.GlobalSettings)
		}
	}
	if synchronousStorage {
		doStorage(ctx)
	} else {
		go doStorage(context.WithoutCancel(ctx))
	}
	return &historySync, nil
}

// HandleAppStateSyncKeyShare grava as chaves de app state recebidas e dispara a
// busca inicial de todos os patches.
//
// O lock de leitura fica segurado pelo laco inteiro, incluindo as gravacoes no
// store — e' o que o RLock/RUnlock manual daqui fazia antes da extracao do
// dominio para internal/wa-noise/appstatesync. `appstate.AllPatchNames` e'
// importado direto por ser DADO.
func HandleAppStateSyncKeyShare(ctx context.Context, t Transport, keys *waE2E.AppStateSyncKeyShare) {
	onlyResyncIfNotSynced := true

	t.Log().Debugf("Got %d new app state keys", len(keys.GetKeys()))
	t.AppStateSync().ReadKeyRequests(func(wasRequested func(string) bool) {
		for _, key := range keys.GetKeys() {
			marshaledFingerprint, err := proto.Marshal(key.GetKeyData().GetFingerprint())
			if err != nil {
				t.Log().Errorf("Failed to marshal fingerprint of app state sync key %X", key.GetKeyID().GetKeyID())
				continue
			}
			if wasRequested(hex.EncodeToString(key.GetKeyID().GetKeyID())) {
				onlyResyncIfNotSynced = false
			}
			err = t.Store().AppStateKeys.PutAppStateSyncKey(ctx, key.GetKeyID().GetKeyID(), store.AppStateSyncKey{
				Data:        key.GetKeyData().GetKeyData(),
				Fingerprint: marshaledFingerprint,
				Timestamp:   key.GetKeyData().GetTimestamp(),
			})
			if err != nil {
				t.Log().Errorf("Failed to store app state sync key %X: %v", key.GetKeyID().GetKeyID(), err)
				continue
			}
			t.Log().Debugf("Received app state sync key %X (ts: %d)", key.GetKeyID().GetKeyID(), key.GetKeyData().GetTimestamp())
		}
	})

	for _, name := range appstate.AllPatchNames {
		err := t.FetchAppState(ctx, name, false, onlyResyncIfNotSynced)
		if err != nil {
			t.Log().Errorf("Failed to do initial fetch of app state %s: %v", name, err)
		}
	}
}

// HandlePlaceholderResendResponse trata a resposta ao pedido de reenvio de uma
// mensagem que nao deu para decifrar.
func HandlePlaceholderResendResponse(t Transport, msg *waE2E.PeerDataOperationRequestResponseMessage) (ok bool) {
	reqID := msg.GetStanzaID()
	parts := msg.GetPeerDataOperationResult()
	t.Log().Debugf("Handling response to placeholder resend request %s with %d items", reqID, len(parts))
	ok = true
	for i, part := range parts {
		var webMsg waWeb.WebMessageInfo
		if resp := part.GetPlaceholderMessageResendResponse(); resp == nil {
			t.Log().Warnf("Missing response in item #%d of response to %s", i+1, reqID)
		} else if err := proto.Unmarshal(resp.GetWebMessageInfoBytes(), &webMsg); err != nil {
			t.Log().Warnf("Failed to unmarshal protobuf web message in item #%d of response to %s: %v", i+1, reqID, err)
		} else if msgEvt, err := t.ParseWebMessage(types.EmptyJID, &webMsg); err != nil {
			t.Log().Warnf("Failed to parse web message info in item #%d of response to %s: %v", i+1, reqID, err)
		} else {
			msgEvt.UnavailableRequestID = reqID
			ok = !t.DispatchEvent(msgEvt) && ok
		}
	}
	return
}

// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

func TestBuildFatalAppStateExceptionNotification(t *testing.T) {
	t.Parallel()
	before := time.Now().UnixMilli()
	msg := BuildFatalAppStateExceptionNotification(
		appstate.WAPatchCriticalBlock,
		appstate.WAPatchRegularHigh,
	)
	after := time.Now().UnixMilli()

	pm := msg.GetProtocolMessage()
	if pm.GetType() != waE2E.ProtocolMessage_APP_STATE_FATAL_EXCEPTION_NOTIFICATION {
		t.Errorf("Type = %v", pm.GetType())
	}
	notif := pm.GetAppStateFatalExceptionNotification()
	got := notif.GetCollectionNames()
	want := []string{string(appstate.WAPatchCriticalBlock), string(appstate.WAPatchRegularHigh)}
	if len(got) != len(want) {
		t.Fatalf("CollectionNames = %v, esperado %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CollectionNames[%d] = %q, esperado %q", i, got[i], want[i])
		}
	}
	// O timestamp desta notificacao e' em milissegundos.
	if ts := notif.GetTimestamp(); ts < before || ts > after {
		t.Errorf("Timestamp = %d, fora da janela [%d, %d] em ms", ts, before, after)
	}
}

func TestBuildFatalAppStateExceptionNotificationSemColecoes(t *testing.T) {
	t.Parallel()
	notif := BuildFatalAppStateExceptionNotification().GetProtocolMessage().GetAppStateFatalExceptionNotification()
	if len(notif.GetCollectionNames()) != 0 {
		t.Errorf("CollectionNames = %v, esperado vazio", notif.GetCollectionNames())
	}
}

func TestBuildAppStateRecoveryRequest(t *testing.T) {
	t.Parallel()
	before := time.Now().Unix()
	msg := BuildAppStateRecoveryRequest(appstate.WAPatchCriticalUnblockLow)
	after := time.Now().Unix()

	pm := msg.GetProtocolMessage()
	if pm.GetType() != waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE {
		t.Errorf("Type = %v", pm.GetType())
	}
	req := pm.GetPeerDataOperationRequestMessage()
	if req.GetPeerDataOperationRequestType() != waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY {
		t.Errorf("PeerDataOperationRequestType = %v", req.GetPeerDataOperationRequestType())
	}
	recovery := req.GetSyncdCollectionFatalRecoveryRequest()
	if recovery.GetCollectionName() != string(appstate.WAPatchCriticalUnblockLow) {
		t.Errorf("CollectionName = %q, esperado %q", recovery.GetCollectionName(), appstate.WAPatchCriticalUnblockLow)
	}
	// Diferente da notificacao fatal acima, este timestamp e' em segundos.
	if ts := recovery.GetTimestamp(); ts < before || ts > after {
		t.Errorf("Timestamp = %d, fora da janela [%d, %d] em segundos", ts, before, after)
	}
}

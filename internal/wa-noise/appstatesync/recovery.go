// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// HandleRecovery processa a resposta do dispositivo primario a um
// BuildAppStateRecoveryRequest: um snapshot nao criptografado da colecao.
//
// Devolve false apenas quando algum handler de evento falhou (o chamador usa
// isso para nao dar ack na mensagem). Todo o resto — dado ausente, blob
// corrompido, versao velha — e' logado e devolve true, porque nao ha' o que
// reprocessar: reenviar a mesma resposta daria o mesmo resultado.
func HandleRecovery(
	ctx context.Context,
	t Transport,
	reqID types.MessageID,
	result []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult,
) bool {
	if len(result) == 0 || result[0].GetSyncdSnapshotFatalRecoveryResponse() == nil {
		t.Log().Warnf("No app state recovery data received for %s", reqID)
		return true
	} else if len(result) > 1 {
		t.Log().Warnf("Unexpected number of app state recovery results for %s: %d", reqID, len(result))
	}
	var eventsToDispatch []any
	eventsToDispatchPtr := &eventsToDispatch
	if !t.EmitEventsOnFullSync() {
		eventsToDispatchPtr = nil
	}
	snapshot, err := appstate.ParseRecovery(result[0].GetSyncdSnapshotFatalRecoveryResponse())
	if err != nil {
		t.Log().Warnf("Failed to parse app state recovery blob for %s: %v", reqID, err)
		return true
	}
	name := appstate.WAPatchName(snapshot.GetCollectionName())
	version := snapshot.GetVersion().GetVersion()
	currentVersion, _, err := t.Store().AppState.GetAppStateVersion(ctx, string(name))
	if err != nil {
		t.Log().Errorf("Failed to get current app state %s version for %s: %v", name, reqID, err)
		return true
	} else if currentVersion >= version {
		t.Log().Infof("Ignoring app state recovery response for %s as current version %d is newer than or equal to recovery version %d", reqID, currentVersion, snapshot.GetVersion().GetVersion())
		return true
	}
	t.Log().Debugf("Handling app state recovery response for %s", reqID)
	mutations, err := t.Proc().ProcessRecovery(ctx, snapshot)
	if err != nil {
		t.Log().Warnf("Failed to parse app state recovery blob for %s: %v", reqID, err)
		return true
	}
	err = CollectEvents(ctx, t, name, mutations, true, eventsToDispatchPtr)
	if err != nil {
		t.Log().Warnf("Failed to collect app state events for %s: %v", reqID, err)
		return true
	}
	eventsToDispatch = append(eventsToDispatch, &events.AppStateSyncComplete{Name: name, Version: version, Recovery: true})
	for _, evt := range eventsToDispatch {
		handlerFailed := t.DispatchEvent(evt)
		if handlerFailed {
			return false
		}
	}
	t.Log().Debugf("Finished handling app state recovery response for %s (%s to v%d)", reqID, name, version)
	return true
}

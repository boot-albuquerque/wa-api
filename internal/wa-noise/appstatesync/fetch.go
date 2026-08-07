// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"errors"
	"fmt"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// Fetch busca as atualizacoes do tipo de app state dado e devolve os eventos
// que o chamador deve despachar. Se fullSync for true, o estado em cache e'
// descartado e todos os patches sao rebuscados do servidor.
//
// Devolver os eventos em vez de despacha-los e' o desenho de origem: o despacho
// acontece fora do lock de sincronizacao, que esta segurado por toda esta
// funcao.
func Fetch(ctx context.Context, t Transport, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) ([]any, error) {
	t.State().LockSync()
	defer t.State().UnlockSync()
	if fullSync {
		err := t.Store().AppState.DeleteAppStateVersion(ctx, string(name))
		if err != nil {
			return nil, fmt.Errorf("failed to reset app state %s version: %w", name, err)
		}
	}
	version, hash, err := t.Store().AppState.GetAppStateVersion(ctx, string(name))
	if err != nil {
		return nil, fmt.Errorf("failed to get app state %s version: %w", name, err)
	}
	if version == 0 {
		fullSync = true
	} else if onlyIfNotSynced {
		return nil, nil
	}

	state := appstate.HashState{Version: version, Hash: hash}

	hasMore := true
	wantSnapshot := fullSync
	var eventsToDispatch []any
	eventsToDispatchPtr := &eventsToDispatch
	if fullSync && !t.EmitEventsOnFullSync() {
		eventsToDispatchPtr = nil
	}
	for hasMore {
		patches, err := FetchPatches(ctx, t, name, state.Version, wantSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch app state %s patches: %w", name, err)
		} else if !wantSnapshot && patches.Snapshot != nil {
			return nil, fmt.Errorf("server unexpectedly returned snapshot for %s without asking", name)
		} else if patches.Snapshot != nil && state != (appstate.HashState{}) {
			return nil, fmt.Errorf("unexpected non-empty input state (v%d) for %s when applying snapshot", state.Version, name)
		}
		wantSnapshot = false
		hasMore = patches.HasMorePatches
		state, err = ApplyPatches(ctx, t, name, state, patches, fullSync, eventsToDispatchPtr)
		if err != nil {
			return nil, err
		}
	}
	if fullSync {
		t.Log().Debugf("Full sync of app state %s completed. Current version: %d", name, state.Version)
		eventsToDispatch = append(eventsToDispatch, &events.AppStateSyncComplete{Name: name, Version: state.Version})
	} else {
		t.Log().Debugf("Synced app state %s from version %d to %d", name, version, state.Version)
	}
	return eventsToDispatch, nil
}

// ApplyPatches decodifica os patches sobre o estado dado e coleta os eventos
// resultantes. Quando a decodificacao falha por chave ausente, dispara em
// background o pedido das chaves que faltam em vez de emitir AppStateSyncError:
// o sync sera' refeito quando as chaves chegarem.
func ApplyPatches(
	ctx context.Context,
	t Transport,
	name appstate.WAPatchName,
	state appstate.HashState,
	patches *appstate.PatchList,
	fullSync bool,
	eventsToDispatch *[]any,
) (appstate.HashState, error) {
	mutations, newState, err := t.Proc().DecodePatches(ctx, patches, state, true)
	if err != nil {
		if errors.Is(err, appstate.ErrKeyNotFound) {
			go RequestMissingKeys(context.WithoutCancel(ctx), t, patches)
		} else {
			t.DispatchEvent(&events.AppStateSyncError{Name: name, FullSync: fullSync, Error: err})
		}
		return state, fmt.Errorf("failed to decode app state %s patches: %w", name, err)
	}
	return newState, CollectEvents(ctx, t, name, mutations, fullSync, eventsToDispatch)
}

// FetchPatches monta e envia o `<iq><sync><collection>` que pede os patches a
// partir de fromVersion, ou o snapshot completo quando snapshot e' true.
func FetchPatches(ctx context.Context, t Transport, name appstate.WAPatchName, fromVersion uint64, snapshot bool) (*appstate.PatchList, error) {
	attrs := waBinary.Attrs{
		attrName:           string(name),
		attrReturnSnapshot: snapshot,
	}
	if !snapshot {
		attrs[attrVersion] = fromVersion
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: namespace,
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: syncTag,
			Content: []waBinary.Node{{
				Tag:   collectionTag,
				Attrs: attrs,
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	collection, ok := resp.GetOptionalChildByTag(syncTag, collectionTag)
	if !ok {
		return nil, t.ElementMissing(collectionTag, fetchErrContext)
	}
	return appstate.ParsePatchList(ctx, &collection, t.DownloadExternalBlob)
}

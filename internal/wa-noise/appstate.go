// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// FetchAppState fetches updates to the given type of app state. If fullSync is true, the current
// cached state will be removed and all app state patches will be re-fetched from the server.
func (cli *Client) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	eventsToDispatch, err := cli.fetchAppState(ctx, name, fullSync, onlyIfNotSynced)
	if err != nil {
		return err
	}
	for _, evt := range eventsToDispatch {
		cli.dispatchEvent(evt)
	}
	return nil
}

func (cli *Client) fetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) ([]any, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	cli.appStateSyncLock.Lock()
	defer cli.appStateSyncLock.Unlock()
	if fullSync {
		err := cli.Store.AppState.DeleteAppStateVersion(ctx, string(name))
		if err != nil {
			return nil, fmt.Errorf("failed to reset app state %s version: %w", name, err)
		}
	}
	version, hash, err := cli.Store.AppState.GetAppStateVersion(ctx, string(name))
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
	if fullSync && !cli.EmitAppStateEventsOnFullSync {
		eventsToDispatchPtr = nil
	}
	for hasMore {
		patches, err := cli.fetchAppStatePatches(ctx, name, state.Version, wantSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch app state %s patches: %w", name, err)
		} else if !wantSnapshot && patches.Snapshot != nil {
			return nil, fmt.Errorf("server unexpectedly returned snapshot for %s without asking", name)
		} else if patches.Snapshot != nil && state != (appstate.HashState{}) {
			return nil, fmt.Errorf("unexpected non-empty input state (v%d) for %s when applying snapshot", state.Version, name)
		}
		wantSnapshot = false
		hasMore = patches.HasMorePatches
		state, err = cli.applyAppStatePatches(ctx, name, state, patches, fullSync, eventsToDispatchPtr)
		if err != nil {
			return nil, err
		}
	}
	if fullSync {
		cli.Log.Debugf("Full sync of app state %s completed. Current version: %d", name, state.Version)
		eventsToDispatch = append(eventsToDispatch, &events.AppStateSyncComplete{Name: name, Version: state.Version})
	} else {
		cli.Log.Debugf("Synced app state %s from version %d to %d", name, version, state.Version)
	}
	return eventsToDispatch, nil
}

func (cli *Client) handleAppStateRecovery(
	ctx context.Context,
	reqID types.MessageID,
	result []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult,
) bool {
	if len(result) == 0 || result[0].GetSyncdSnapshotFatalRecoveryResponse() == nil {
		cli.Log.Warnf("No app state recovery data received for %s", reqID)
		return true
	} else if len(result) > 1 {
		cli.Log.Warnf("Unexpected number of app state recovery results for %s: %d", reqID, len(result))
	}
	var eventsToDispatch []any
	eventsToDispatchPtr := &eventsToDispatch
	if !cli.EmitAppStateEventsOnFullSync {
		eventsToDispatchPtr = nil
	}
	snapshot, err := appstate.ParseRecovery(result[0].GetSyncdSnapshotFatalRecoveryResponse())
	if err != nil {
		cli.Log.Warnf("Failed to parse app state recovery blob for %s: %v", reqID, err)
		return true
	}
	name := appstate.WAPatchName(snapshot.GetCollectionName())
	version := snapshot.GetVersion().GetVersion()
	currentVersion, _, err := cli.Store.AppState.GetAppStateVersion(ctx, string(name))
	if err != nil {
		cli.Log.Errorf("Failed to get current app state %s version for %s: %v", name, reqID, err)
		return true
	} else if currentVersion >= version {
		cli.Log.Infof("Ignoring app state recovery response for %s as current version %d is newer than or equal to recovery version %d", reqID, currentVersion, snapshot.GetVersion().GetVersion())
		return true
	}
	cli.Log.Debugf("Handling app state recovery response for %s", reqID)
	mutations, err := cli.appStateProc.ProcessRecovery(ctx, snapshot)
	if err != nil {
		cli.Log.Warnf("Failed to parse app state recovery blob for %s: %v", reqID, err)
		return true
	}
	err = cli.collectEventsToDispatch(ctx, name, mutations, true, eventsToDispatchPtr)
	if err != nil {
		cli.Log.Warnf("Failed to collect app state events for %s: %v", reqID, err)
		return true
	}
	eventsToDispatch = append(eventsToDispatch, &events.AppStateSyncComplete{Name: name, Version: version, Recovery: true})
	for _, evt := range eventsToDispatch {
		handlerFailed := cli.dispatchEvent(evt)
		if handlerFailed {
			return false
		}
	}
	cli.Log.Debugf("Finished handling app state recovery response for %s (%s to v%d)", reqID, name, version)
	return true
}

func (cli *Client) applyAppStatePatches(
	ctx context.Context,
	name appstate.WAPatchName,
	state appstate.HashState,
	patches *appstate.PatchList,
	fullSync bool,
	eventsToDispatch *[]any,
) (appstate.HashState, error) {
	mutations, newState, err := cli.appStateProc.DecodePatches(ctx, patches, state, true)
	if err != nil {
		if errors.Is(err, appstate.ErrKeyNotFound) {
			go cli.requestMissingAppStateKeys(context.WithoutCancel(ctx), patches)
		} else {
			cli.dispatchEvent(&events.AppStateSyncError{Name: name, FullSync: fullSync, Error: err})
		}
		return state, fmt.Errorf("failed to decode app state %s patches: %w", name, err)
	}
	return newState, cli.collectEventsToDispatch(ctx, name, mutations, fullSync, eventsToDispatch)
}

func (cli *Client) downloadExternalAppStateBlob(ctx context.Context, ref *waServerSync.ExternalBlobReference) ([]byte, error) {
	return cli.Download(ctx, ref)
}

func (cli *Client) fetchAppStatePatches(ctx context.Context, name appstate.WAPatchName, fromVersion uint64, snapshot bool) (*appstate.PatchList, error) {
	attrs := waBinary.Attrs{
		appStateAttrName:           string(name),
		appStateAttrReturnSnapshot: snapshot,
	}
	if !snapshot {
		attrs[appStateAttrVersion] = fromVersion
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: appStateNamespace,
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: appStateSyncTag,
			Content: []waBinary.Node{{
				Tag:   appStateCollectionTag,
				Attrs: attrs,
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	collection, ok := resp.GetOptionalChildByTag(appStateSyncTag, appStateCollectionTag)
	if !ok {
		return nil, &ElementMissingError{Tag: appStateCollectionTag, In: appStateFetchErrContext}
	}
	return appstate.ParsePatchList(ctx, &collection, cli.downloadExternalAppStateBlob)
}

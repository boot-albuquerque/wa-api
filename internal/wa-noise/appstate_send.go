// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exslices"
	"go.mau.fi/util/ptr"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

// SendAppState sends the given app state patch, then triggers a background resync of that app state type
// to update local caches and send events for the updates.
//
// You can use the Build methods in the appstate package to build the parameter for this method, e.g.
//
//	cli.SendAppState(ctx, appstate.BuildMute(targetJID, true, 24 * time.Hour))
func (cli *Client) SendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	return cli.sendAppState(ctx, patch, true)
}

func (cli *Client) sendAppState(ctx context.Context, patch appstate.PatchInfo, allowRetry bool) error {
	if cli == nil {
		return ErrClientIsNil
	}
	version, hash, err := cli.Store.AppState.GetAppStateVersion(ctx, string(patch.Type))
	if err != nil {
		return err
	}
	// TODO create new key instead of reusing the primary client's keys
	latestKeyID, err := cli.Store.AppStateKeys.GetLatestAppStateSyncKeyID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest app state key ID: %w", err)
	} else if latestKeyID == nil {
		return fmt.Errorf("no app state keys found, creating app state keys is not yet supported")
	}

	state := appstate.HashState{Version: version, Hash: hash}

	encodedPatch, err := cli.appStateProc.EncodePatch(ctx, latestKeyID, state, patch)
	if err != nil {
		return err
	}

	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: appStateNamespace,
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: appStateSyncTag,
			Content: []waBinary.Node{{
				Tag: appStateCollectionTag,
				Attrs: waBinary.Attrs{
					appStateAttrName:           string(patch.Type),
					appStateAttrVersion:        version,
					appStateAttrReturnSnapshot: false,
				},
				Content: []waBinary.Node{{
					Tag:     appStatePatchTag,
					Content: encodedPatch,
				}},
			}},
		}},
	})
	if err != nil {
		return err
	}

	respCollection, ok := resp.GetOptionalChildByTag(appStateSyncTag, appStateCollectionTag)
	if !ok {
		return &ElementMissingError{Tag: appStateCollectionTag, In: appStateSendErrContext}
	}
	respCollectionAttr := respCollection.AttrGetter()
	if respCollectionAttr.OptionalString(appStateAttrType) == appStateRespTypeError {
		errorTag, ok := respCollection.GetOptionalChildByTag(appStateErrorTag)

		mainErr := fmt.Errorf("%w: %s", ErrAppStateUpdate, respCollection.XMLString())
		if ok {
			mainErr = fmt.Errorf("%w (%s): %s", ErrAppStateUpdate, patch.Type, errorTag.XMLString())
		}
		if ok && errorTag.AttrGetter().Int(appStateAttrCode) == appStateConflictCode && allowRetry {
			zerolog.Ctx(ctx).Warn().Err(mainErr).Msg("Failed to update app state, trying to apply conflicts and retry")
			var eventsToDispatch []any
			patches, err := appstate.ParsePatchList(ctx, &respCollection, cli.downloadExternalAppStateBlob)
			if err != nil {
				return fmt.Errorf("%w (also, parsing patches in the response failed: %w)", mainErr, err)
			} else if state, err = cli.applyAppStatePatches(ctx, patch.Type, state, patches, false, &eventsToDispatch); err != nil {
				return fmt.Errorf("%w (also, applying patches in the response failed: %w)", mainErr, err)
			} else {
				zerolog.Ctx(ctx).Debug().Msg("Retrying app state send after applying conflicting patches")
				go func() {
					for _, evt := range eventsToDispatch {
						cli.dispatchEvent(evt)
					}
				}()
				return cli.sendAppState(ctx, patch, false)
			}
		}
		return mainErr
	}
	eventsToDispatch, err := cli.fetchAppState(ctx, patch.Type, false, false)
	if err != nil {
		return fmt.Errorf("failed to fetch app state after sending update: %w", err)
	}
	go func() {
		for _, evt := range eventsToDispatch {
			cli.dispatchEvent(evt)
		}
	}()

	return nil
}

func (cli *Client) MarkNotDirty(ctx context.Context, cleanType string, ts time.Time) error {
	_, err := cli.sendIQ(ctx, infoQuery{
		Namespace: dirtyNamespace,
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: dirtyCleanTag,
			Attrs: waBinary.Attrs{
				dirtyCleanAttrType:      cleanType,
				dirtyCleanAttrTimestamp: ts.Unix(),
			},
		}},
	})
	return err
}

// BuildFatalAppStateExceptionNotification builds a message to request the user's primary device
// to reset specific app state collections. This will cause all linked devices to be logged out.
//
// The built message can be sent using Client.SendPeerMessage.
// There is no response, as the client will get logged out.
func BuildFatalAppStateExceptionNotification(collections ...appstate.WAPatchName) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_APP_STATE_FATAL_EXCEPTION_NOTIFICATION.Enum(),
			AppStateFatalExceptionNotification: &waE2E.AppStateFatalExceptionNotification{
				CollectionNames: exslices.CastToString[string](collections),
				Timestamp:       ptr.Ptr(time.Now().UnixMilli()),
			},
		},
	}
}

// BuildAppStateRecoveryRequest builds a message to request the user's primary device to send
// an unencrypted copy of the given app state collection.
//
// The built message can be sent using Client.SendPeerMessage.
// The response will come as a ProtocolMessage with type `PEER_DATA_OPERATION_RESPONSE_MESSAGE`.
func BuildAppStateRecoveryRequest(collection appstate.WAPatchName) *waE2E.Message {
	return &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY.Enum(),
				SyncdCollectionFatalRecoveryRequest: &waE2E.PeerDataOperationRequestMessage_SyncDCollectionFatalRecoveryRequest{
					CollectionName: (*string)(&collection),
					Timestamp:      ptr.Ptr(time.Now().Unix()),
				},
			},
		},
	}
}

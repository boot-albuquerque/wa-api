package core

import (
	"context"
	"time"

	"go.mau.fi/util/exslices"
	"go.mau.fi/util/ptr"

	"wa-api/internal/noise/capabilities/appstatesync"
	"wa-api/internal/noise/protocol/appstate"
	"wa-api/internal/noise/protocol/proto/waE2E"
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
	return appstatesync.Send(ctx, cli.appStateT(), patch, allowRetry)
}

// MarkNotDirty marca uma colecao como "nao suja" no servidor.
func (cli *Client) MarkNotDirty(ctx context.Context, cleanType string, ts time.Time) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return appstatesync.MarkNotDirty(ctx, cli.appStateT(), cleanType, ts)
}

// As duas funcoes abaixo nao dependem de *Client nem do Transport: montam
// mensagens a partir dos argumentos e so'. Ficam na raiz porque sao API publica
// do fork e funcoes (ao contrario de tipos) nao podem ser reexportadas por
// apelido — move-las quebraria todo chamador externo.

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

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/types"
)

// O history sync vive em internal/wa-noise/message/ desde a Fase F/G lote 9.
// DownloadHistorySync e SendHistorySyncServerErrorReceipt continuam na raiz por
// serem API publica; as demais sao fachadas para internals.go (gerado) e para
// armadillomessage.go.

func (cli *Client) handleSenderKeyDistributionMessage(ctx context.Context, chat, from types.JID, axolotlSKDM []byte) {
	message.HandleSenderKeyDistributionMessage(ctx, cli.msgT(), chat, from, axolotlSKDM)
}

func (cli *Client) handleHistorySyncNotificationLoop() {
	message.HandleHistorySyncNotificationLoop(cli.msgT())
}

// SendHistorySyncServerErrorReceipt sends a history sync server-error receipt, which
// asks the phone to re-upload the referenced history sync payload.
func (cli *Client) SendHistorySyncServerErrorReceipt(ctx context.Context, msgID types.MessageID, mediaKey []byte) error {
	return message.SendHistorySyncServerErrorReceipt(ctx, cli.msgT(), msgID, mediaKey)
}

// DownloadHistorySync will download and parse the history sync blob from the given history sync notification.
//
// You only need to call this manually if you set [Client.ManualHistorySyncDownload] to true.
// By default, whatsmeow will call this automatically and dispatch an [events.HistorySync] with the parsed data.
func (cli *Client) DownloadHistorySync(ctx context.Context, notif *waE2E.HistorySyncNotification, synchronousStorage bool) (*waHistorySync.HistorySync, error) {
	return message.DownloadHistorySync(ctx, cli.msgT(), notif, synchronousStorage)
}

func (cli *Client) handleAppStateSyncKeyShare(ctx context.Context, keys *waE2E.AppStateSyncKeyShare) {
	message.HandleAppStateSyncKeyShare(ctx, cli.msgT(), keys)
}

func (cli *Client) handlePlaceholderResendResponse(msg *waE2E.PeerDataOperationRequestResponseMessage) (ok bool) {
	return message.HandlePlaceholderResendResponse(cli.msgT(), msg)
}

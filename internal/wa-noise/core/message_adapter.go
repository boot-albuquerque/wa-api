package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/appstatesync"
	"wa-api/internal/wa-noise/capabilities/message"
	waLog "wa-api/internal/wa-noise/observability/log"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// messageTransport adapta *Client a message.Transport. Existe para que o pacote
// internal/wa-noise/message possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 9".
type messageTransport struct {
	cli *Client
}

var _ message.Transport = messageTransport{}

// msgT devolve o adaptador de recepcao de mensagem deste cliente.
func (cli *Client) msgT() message.Transport {
	return messageTransport{cli}
}

func (t messageTransport) Store() *store.Device { return t.cli.Store }

func (t messageTransport) Log() waLog.Logger { return t.cli.Log }

func (t messageTransport) OwnID() types.JID { return t.cli.getOwnID() }

func (t messageTransport) OwnLID() types.JID { return t.cli.getOwnLID() }

func (t messageTransport) IsMessenger() bool { return t.cli.MessengerConfig != nil }

// Errors entrega o MESMO ponteiro de sentinela da raiz. Ver o doc de
// message.Errors para por que isso importa.
func (t messageTransport) Errors() message.Errors {
	return message.Errors{NotLoggedIn: ErrNotLoggedIn}
}

// Nacks entrega os codigos de nack de receipt.go. Ver o doc de message.Nacks.
func (t messageTransport) Nacks() message.Nacks {
	return message.Nacks{
		UnrecognizedStanza:   NackUnrecognizedStanza,
		InvalidProtobuf:      NackInvalidProtobuf,
		MissingMessageSecret: NackMissingMessageSecret,
	}
}

func (t messageTransport) DispatchEvent(evt any) bool { return t.cli.dispatchEvent(evt) }

func (t messageTransport) SendNode(ctx context.Context, node waBinary.Node) error {
	return t.cli.sendNode(ctx, node)
}

func (t messageTransport) AutoTrustIdentity() bool { return t.cli.AutoTrustIdentity }

func (t messageTransport) EnableDecryptedEventBuffer() bool {
	return t.cli.EnableDecryptedEventBuffer
}

func (t messageTransport) SynchronousAck() bool { return t.cli.SynchronousAck }

func (t messageTransport) ManualHistorySyncDownload() bool {
	return t.cli.ManualHistorySyncDownload
}

func (t messageTransport) DisableManualHistorySyncReceipt() bool {
	return t.cli.DisableManualHistorySyncReceipt
}

func (t messageTransport) BackgroundEventCtx() context.Context { return t.cli.BackgroundEventCtx }

func (t messageTransport) BackgroundIfAsyncAck(fn func()) { t.cli.backgroundIfAsyncAck(fn) }

func (t messageTransport) MaybeDeferredAck(ctx context.Context, node *waBinary.Node) func(cancelled ...*bool) {
	return t.cli.maybeDeferredAck(ctx, node)
}

func (t messageTransport) SendAck(ctx context.Context, node *waBinary.Node, errorCode int) {
	t.cli.sendAck(ctx, node, errorCode)
}

func (t messageTransport) SendMessageReceipt(ctx context.Context, info *types.MessageInfo, node *waBinary.Node) {
	t.cli.sendMessageReceipt(ctx, info, node)
}

func (t messageTransport) SendRetryReceipt(ctx context.Context, node *waBinary.Node, info *types.MessageInfo, forceIncludeIdentity bool) {
	t.cli.sendRetryReceipt(ctx, node, info, forceIncludeIdentity)
}

func (t messageTransport) ImmediateRequestMessageFromPhone(ctx context.Context, info *types.MessageInfo) {
	t.cli.immediateRequestMessageFromPhone(ctx, info)
}

func (t messageTransport) CancelDelayedRequestFromPhone(id types.MessageID) {
	t.cli.cancelDelayedRequestFromPhone(id)
}

func (t messageTransport) UpdateBusinessName(ctx context.Context, jid, jidAlt types.JID, info *types.MessageInfo, name string) {
	t.cli.updateBusinessName(ctx, jid, jidAlt, info, name)
}

func (t messageTransport) UpdatePushName(ctx context.Context, jid, jidAlt types.JID, info *types.MessageInfo, name string) {
	t.cli.updatePushName(ctx, jid, jidAlt, info, name)
}

func (t messageTransport) HandleHistoricalPushNames(ctx context.Context, names []*waHistorySync.Pushname) {
	t.cli.handleHistoricalPushNames(ctx, names)
}

func (t messageTransport) StoreLIDPNMapping(ctx context.Context, first, second types.JID) {
	t.cli.StoreLIDPNMapping(ctx, first, second)
}

func (t messageTransport) AppStateSync() *appstatesync.State { return &t.cli.appStateSync }

func (t messageTransport) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	return t.cli.FetchAppState(ctx, name, fullSync, onlyIfNotSynced)
}

func (t messageTransport) HandleAppStateRecovery(ctx context.Context, reqID types.MessageID, result []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult) bool {
	return t.cli.handleAppStateRecovery(ctx, reqID, result)
}

func (t messageTransport) HandleDecryptedArmadillo(ctx context.Context, info *types.MessageInfo, decrypted []byte, retryCount int) (bool, bool) {
	return t.cli.handleDecryptedArmadillo(ctx, info, decrypted, retryCount)
}

func (t messageTransport) ParseWebMessage(chatJID types.JID, webMsg *waWeb.WebMessageInfo) (*events.Message, error) {
	return t.cli.ParseWebMessage(chatJID, webMsg)
}

func (t messageTransport) DownloadHistorySyncBlob(ctx context.Context, notif *waE2E.HistorySyncNotification) ([]byte, error) {
	return t.cli.Download(ctx, notif)
}

func (t messageTransport) DeleteHistorySyncMedia(ctx context.Context, directPath string, encFileHash []byte, encHandle string) error {
	return t.cli.DeleteMedia(ctx, MediaHistory, directPath, encFileHash, encHandle)
}

func (t messageTransport) StoreNCTSalt(ctx context.Context, salt []byte) error {
	return t.cli.storeNCTSalt(ctx, salt)
}

func (t messageTransport) HistorySync() *message.HistorySyncQueue { return t.cli.historySync }

func (t messageTransport) DecryptBuffer() *message.DecryptBufferState {
	return &t.cli.decryptBuffer
}

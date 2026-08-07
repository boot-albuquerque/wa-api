// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/appstatesync"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

var (
	testOwnJID   = types.NewJID("5511999999999", types.DefaultUserServer)
	testOwnLID   = types.NewJID("11223344556677", types.HiddenUserServer)
	testOtherJID = types.NewJID("5511888888888", types.DefaultUserServer)
	testGroupJID = types.NewJID("123456789-987654321", types.GroupServer)
)

var testSecret = bytes.Repeat([]byte{0xAB}, send.MessageSecretSize)

// errNotLoggedIn e' o duble do ErrNotLoggedIn da raiz. Existe porque o
// sentinela mora la' e atravessa a interface (ver message.Errors).
var errNotLoggedIn = errors.New("the store doesn't contain a device JID")

// Os tres codigos de nack sao os valores reais de receipt.go (raiz). Estao
// aqui como dado de teste porque e' exatamente assim que eles atravessam a
// interface em producao.
const (
	testNackUnrecognizedStanza   = 488
	testNackInvalidProtobuf      = 491
	testNackMissingMessageSecret = 495
)

// stubMsgSecretStore devolve sempre o mesmo segredo e o mesmo remetente
// original gravado. `secret == nil` reproduz "segredo nao encontrado", que e' o
// que o sqlstore devolve para uma mensagem que nunca vimos.
type stubMsgSecretStore struct {
	secret     []byte
	origSender types.JID
	err        error

	putChat   types.JID
	putSender types.JID
	putID     types.MessageID
	putSecret []byte
	putMany   []store.MessageSecretInsert
}

func (s *stubMsgSecretStore) PutMessageSecrets(_ context.Context, inserts []store.MessageSecretInsert) error {
	s.putMany = append(s.putMany, inserts...)
	return s.err
}

func (s *stubMsgSecretStore) PutMessageSecret(_ context.Context, chat, sender types.JID, id types.MessageID, secret []byte) error {
	s.putChat, s.putSender, s.putID, s.putSecret = chat, sender, id, secret
	return s.err
}

func (s *stubMsgSecretStore) GetMessageSecret(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
	return s.secret, s.origSender, s.err
}

type stubPrivacyTokenStore struct {
	tokens []store.PrivacyToken
}

func (s *stubPrivacyTokenStore) PutPrivacyTokens(_ context.Context, tokens ...store.PrivacyToken) error {
	s.tokens = append(s.tokens, tokens...)
	return nil
}

func (s *stubPrivacyTokenStore) GetPrivacyToken(context.Context, types.JID) (*store.PrivacyToken, error) {
	return nil, nil
}

func (s *stubPrivacyTokenStore) DeleteExpiredPrivacyTokens(context.Context, time.Time) (int64, error) {
	return 0, nil
}

// fakeTransport e' o duble de message.Transport usado por todos os testes deste
// pacote. Substitui o *Client real que os testes usavam antes da extracao:
// nenhum socket, nenhuma sessao Noise, nenhum banco.
//
// O mutex protege os campos de registro porque o caminho de recepcao dispara
// goroutines de producao (updatePushName, updateBusinessName, os dois recibos
// de protocol message, o ack assincrono) — comportamento preservado, nao do
// duble. Foi o `-race` que exigiu isso.
type fakeTransport struct {
	mu sync.Mutex

	log     waLog.Logger
	dev     *store.Device
	ownID   types.JID
	ownLID  types.JID
	msgr    bool
	autoTID bool
	buffer  bool
	syncAck bool

	manualHistorySync   bool
	disableManualRcpt   bool
	backgroundCtx       context.Context
	appSync             appstatesync.State
	histSync            *HistorySyncQueue
	decBuf              DecryptBufferState
	fetchedAppStates    []appstate.WAPatchName
	fetchAppStateErr    error
	appStateRecoveryRet bool

	sentNodes    []waBinary.Node
	sendErr      error
	events       []any
	handlerFails bool

	acks              []int
	msgReceipts       int
	retryReceipts     int
	immediateRequests int
	cancelledRequests []types.MessageID

	pushNames     []string
	businessNames []string
	lidMappings   [][2]types.JID

	armadilloHandlerFailed bool
	armadilloProtoFailed   bool

	webMsgEvent *events.Message
	webMsgErr   error

	downloadData []byte
	downloadErr  error
	deletedMedia int
	nctSalt      []byte
}

var _ Transport = (*fakeTransport)(nil)

// noopContainer satisfaz store.DeviceContainer sem banco. E' necessario porque
// StoreLIDSyncMessage/StoreGlobalSettings chamam Device.Save.
type noopContainer struct{ puts int }

func (c *noopContainer) PutDevice(context.Context, *store.Device) error    { c.puts++; return nil }
func (c *noopContainer) DeleteDevice(context.Context, *store.Device) error { return nil }

// permissiveDevice devolve um *store.Device cujos sub-stores todos aceitam
// tudo e devolvem zero (store.NoopStore com Error nil).
func permissiveDevice() *store.Device {
	dev := &store.Device{Container: &noopContainer{}}
	dev.SetAllStores(&store.NoopStore{})
	dev.LIDs = &store.NoopStore{}
	return dev
}

func newFakeTransport() *fakeTransport {
	ownID := testOwnJID
	dev := permissiveDevice()
	dev.ID = &ownID
	dev.LID = testOwnLID
	return &fakeTransport{
		log:                 waLog.Noop,
		dev:                 dev,
		ownID:               ownID,
		ownLID:              testOwnLID,
		backgroundCtx:       context.Background(),
		histSync:            NewHistorySyncQueue(4),
		appStateRecoveryRet: true,
	}
}

// withSecrets troca o store de segredos por um stub e devolve o transporte.
func (f *fakeTransport) withSecrets(s *stubMsgSecretStore) *fakeTransport {
	f.dev.MsgSecrets = s
	return f
}

func (f *fakeTransport) Store() *store.Device { return f.dev }
func (f *fakeTransport) Log() waLog.Logger    { return f.log }
func (f *fakeTransport) OwnID() types.JID     { return f.ownID }
func (f *fakeTransport) OwnLID() types.JID    { return f.ownLID }
func (f *fakeTransport) IsMessenger() bool    { return f.msgr }

func (f *fakeTransport) Errors() Errors { return Errors{NotLoggedIn: errNotLoggedIn} }

func (f *fakeTransport) Nacks() Nacks {
	return Nacks{
		UnrecognizedStanza:   testNackUnrecognizedStanza,
		InvalidProtobuf:      testNackInvalidProtobuf,
		MissingMessageSecret: testNackMissingMessageSecret,
	}
}

func (f *fakeTransport) DispatchEvent(evt any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evt)
	return f.handlerFails
}

func (f *fakeTransport) SendNode(_ context.Context, node waBinary.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentNodes = append(f.sentNodes, node)
	return f.sendErr
}

func (f *fakeTransport) AutoTrustIdentity() bool          { return f.autoTID }
func (f *fakeTransport) EnableDecryptedEventBuffer() bool { return f.buffer }
func (f *fakeTransport) SynchronousAck() bool             { return f.syncAck }

func (f *fakeTransport) ManualHistorySyncDownload() bool { return f.manualHistorySync }

func (f *fakeTransport) DisableManualHistorySyncReceipt() bool { return f.disableManualRcpt }

func (f *fakeTransport) BackgroundEventCtx() context.Context { return f.backgroundCtx }

// BackgroundIfAsyncAck roda sempre SINCRONO no duble, mesmo quando
// SynchronousAck e' false. Isso e' deliberado: o que os testes precisam
// observar e' QUAL ack sai, nao se ele saiu em goroutine. Os caminhos que
// dependem de assincronia de verdade (o `go SendRetryReceipt` de
// DecryptMessages) continuam usando goroutine real, porque estao no codigo
// testado e nao no duble.
func (f *fakeTransport) BackgroundIfAsyncAck(fn func()) { fn() }

func (f *fakeTransport) MaybeDeferredAck(_ context.Context, node *waBinary.Node) func(...*bool) {
	return func(cancelled ...*bool) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sentNodes = append(f.sentNodes, waBinary.Node{Tag: "deferred-ack", Attrs: waBinary.Attrs{"class": node.Tag}})
	}
}

func (f *fakeTransport) SendAck(_ context.Context, _ *waBinary.Node, errorCode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks = append(f.acks, errorCode)
}

func (f *fakeTransport) SendMessageReceipt(context.Context, *types.MessageInfo, *waBinary.Node) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgReceipts++
}

func (f *fakeTransport) SendRetryReceipt(context.Context, *waBinary.Node, *types.MessageInfo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retryReceipts++
}

func (f *fakeTransport) ImmediateRequestMessageFromPhone(context.Context, *types.MessageInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.immediateRequests++
}

func (f *fakeTransport) CancelDelayedRequestFromPhone(id types.MessageID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelledRequests = append(f.cancelledRequests, id)
}

func (f *fakeTransport) UpdateBusinessName(_ context.Context, _, _ types.JID, _ *types.MessageInfo, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.businessNames = append(f.businessNames, name)
}

func (f *fakeTransport) UpdatePushName(_ context.Context, _, _ types.JID, _ *types.MessageInfo, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushNames = append(f.pushNames, name)
}

func (f *fakeTransport) HandleHistoricalPushNames(context.Context, []*waHistorySync.Pushname) {}

func (f *fakeTransport) StoreLIDPNMapping(_ context.Context, first, second types.JID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lidMappings = append(f.lidMappings, [2]types.JID{first, second})
}

func (f *fakeTransport) AppStateSync() *appstatesync.State { return &f.appSync }

func (f *fakeTransport) FetchAppState(_ context.Context, name appstate.WAPatchName, _, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchedAppStates = append(f.fetchedAppStates, name)
	return f.fetchAppStateErr
}

func (f *fakeTransport) HandleAppStateRecovery(context.Context, types.MessageID, []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult) bool {
	return f.appStateRecoveryRet
}

func (f *fakeTransport) HandleDecryptedArmadillo(context.Context, *types.MessageInfo, []byte, int) (bool, bool) {
	return f.armadilloHandlerFailed, f.armadilloProtoFailed
}

func (f *fakeTransport) ParseWebMessage(types.JID, *waWeb.WebMessageInfo) (*events.Message, error) {
	if f.webMsgErr != nil {
		return nil, f.webMsgErr
	}
	if f.webMsgEvent != nil {
		return f.webMsgEvent, nil
	}
	return &events.Message{}, nil
}

func (f *fakeTransport) DownloadHistorySyncBlob(context.Context, *waE2E.HistorySyncNotification) ([]byte, error) {
	return f.downloadData, f.downloadErr
}

func (f *fakeTransport) DeleteHistorySyncMedia(context.Context, string, []byte, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedMedia++
	return nil
}

func (f *fakeTransport) StoreNCTSalt(_ context.Context, salt []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nctSalt = salt
	return nil
}

func (f *fakeTransport) HistorySync() *HistorySyncQueue { return f.histSync }

func (f *fakeTransport) DecryptBuffer() *DecryptBufferState { return &f.decBuf }

// --- ajudantes de no' ---

func msgNode(attrs waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
	n := &waBinary.Node{Tag: "message", Attrs: attrs}
	if len(children) > 0 {
		n.Content = children
	}
	return n
}

func msgEvent(chat, sender types.JID) *events.Message {
	return &events.Message{Info: types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: sender},
	}}
}

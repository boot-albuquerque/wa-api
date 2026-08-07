package send

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

var (
	sendTestGroupJID = types.JID{User: "12345", Server: types.GroupServer}
	sendTestUserJID  = types.JID{User: "5511999999999", Server: types.DefaultUserServer}
	sendTestLIDJID   = types.JID{User: "77777777", Server: types.HiddenUserServer}
)

// errNotLoggedIn e' o duble do ErrNotLoggedIn da raiz. Existe porque o
// sentinela mora la' e atravessa a interface (ver send.Errors).
var errNotLoggedIn = errors.New("the store doesn't contain a device JID")

// fakeTransport e' o duble de send.Transport usado por todos os testes deste
// pacote. Substitui o *Client real que os testes usavam antes da extracao:
// nenhum socket, nenhuma sessao Noise, nenhum banco.
//
// Os dois "caches" sao mapas de presenca, e nao um group.Cache/user.DeviceCache
// de verdade, de proposito: o que este pacote precisa provar e' QUAL cache e'
// invalidado para qual server de JID. O comportamento interno dos dois caches
// (lock, criacao preguicosa do mapa) ja' e' travado nos lotes 6 e 7, nos
// pacotes que os definem.
type fakeTransport struct {
	log   waLog.Logger
	store *store.Device
	lock  sync.Mutex

	// mu protege waiters e issuedTokens, que sao tocados de mais de uma
	// goroutine: o teste alimenta o ack em paralelo, e DM emite o privacy
	// token novo num `go` — comportamento de producao, nao do duble.
	mu sync.Mutex

	ownID types.JID
	ownLI types.JID

	groupCache  map[types.JID]bool
	deviceCache map[types.JID]bool

	waiters map[string]chan *waBinary.Node

	groupMeta    *group.Meta
	groupMetaErr error
	broadcast    []types.JID
	broadcastErr error
	devices      []types.JID
	devicesErr   error
	userInfo     map[types.JID]types.UserInfo
	userInfoErr  error

	sentNodes []waBinary.Node
	sendData  []byte
	sendErr   error

	disconnect  bool
	retryNode   *waBinary.Node
	retryErr    error
	retryCalls  int
	recentErr   error
	generatedID types.MessageID

	reportingToken bool
	tcToken        []byte
	tcTokenErr     error
	csToken        []byte
	issuedTokens   []types.JID
}

// permissiveDevice devolve um *store.Device cujos sub-stores todos aceitam
// tudo e devolvem zero (store.NoopStore com Error nil). E' o que permite
// exercitar os caminhos de montagem de no sem banco: com lista de dispositivos
// VAZIA, WithCachedSessions devolve (nil, ctx, nil) e PutCachedSessions nao faz
// nada, entao nada de Signal e' realmente executado.
//
// O que ele NAO permite: cifrar de verdade para um dispositivo. Isso exige uma
// sessao Signal estabelecida, que nao existe sem handshake — ver PATCHES.md,
// lote 8, "O que nao da' para testar sem sessao viva".
func permissiveDevice() *store.Device {
	dev := &store.Device{Account: &waAdv.ADVSignedDeviceIdentity{}}
	dev.SetAllStores(&store.NoopStore{})
	dev.LIDs = &store.NoopStore{}
	dev.SenderKeys = newMemSenderKeys()
	return dev
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		log:         waLog.Noop,
		store:       permissiveDevice(),
		groupCache:  map[types.JID]bool{},
		deviceCache: map[types.JID]bool{},
		waiters:     map[string]chan *waBinary.Node{},
		generatedID: "GENERATED",
	}
}

func (f *fakeTransport) Store() *store.Device                 { return f.store }
func (f *fakeTransport) Log() waLog.Logger                    { return f.log }
func (f *fakeTransport) OwnID() types.JID                     { return f.ownID }
func (f *fakeTransport) OwnLID() types.JID                    { return f.ownLI }
func (f *fakeTransport) GenerateMessageID() types.MessageID   { return f.generatedID }
func (f *fakeTransport) Errors() Errors                       { return Errors{NotLoggedIn: errNotLoggedIn} }
func (f *fakeTransport) IsMessenger() bool                    { return false }
func (f *fakeTransport) AutoTrustIdentity() bool              { return false }
func (f *fakeTransport) DefaultRequestTimeout() time.Duration { return 75 * time.Second }
func (f *fakeTransport) SendLock() *sync.Mutex                { return &f.lock }

func (f *fakeTransport) WaitResponse(reqID string) chan *waBinary.Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan *waBinary.Node, 1)
	f.waiters[reqID] = ch
	return ch
}

func (f *fakeTransport) CancelResponse(reqID string, _ chan *waBinary.Node) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.waiters, reqID)
}

// waiter le o canal registrado sob o lock.
func (f *fakeTransport) waiter(reqID string) (chan *waBinary.Node, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.waiters[reqID]
	return ch, ok
}

func (f *fakeTransport) SendNodeAndGetData(_ context.Context, node waBinary.Node) ([]byte, error) {
	f.sentNodes = append(f.sentNodes, node)
	return f.sendData, f.sendErr
}

func (f *fakeTransport) IsDisconnectNode(*waBinary.Node) bool { return f.disconnect }

func (f *fakeTransport) RetryFrame(
	context.Context, string, string, []byte, *waBinary.Node, time.Duration,
) (*waBinary.Node, error) {
	f.retryCalls++
	return f.retryNode, f.retryErr
}

func (f *fakeTransport) AddRecentMessage(
	context.Context, types.JID, types.MessageID, *waE2E.Message, *waMsgApplication.MessageApplication,
) error {
	return f.recentErr
}

func (f *fakeTransport) CachedGroupData(context.Context, types.JID) (*group.Meta, error) {
	return f.groupMeta, f.groupMetaErr
}

func (f *fakeTransport) BroadcastListParticipants(context.Context, types.JID) ([]types.JID, error) {
	return f.broadcast, f.broadcastErr
}

func (f *fakeTransport) UserDevices(context.Context, []types.JID) ([]types.JID, error) {
	return f.devices, f.devicesErr
}

func (f *fakeTransport) UserInfo(context.Context, []types.JID) (map[types.JID]types.UserInfo, error) {
	return f.userInfo, f.userInfoErr
}

func (f *fakeTransport) FetchPreKeysNoError(context.Context, []types.JID) map[types.JID]*prekey.Bundle {
	return nil
}

func (f *fakeTransport) InvalidateGroupCache(jid types.JID)  { delete(f.groupCache, jid) }
func (f *fakeTransport) InvalidateDeviceCache(jid types.JID) { delete(f.deviceCache, jid) }

func (f *fakeTransport) MigrateSessionStore(context.Context, types.JID, types.JID) {}

func (f *fakeTransport) ClearUntrustedIdentity(context.Context, types.JID) error { return nil }

func (f *fakeTransport) ShouldIncludeReportingToken(*waE2E.Message) bool { return f.reportingToken }

func (f *fakeTransport) MessageReportingToken(
	[]byte, *waE2E.Message, types.JID, types.JID, types.MessageID,
) waBinary.Node {
	return waBinary.Node{Tag: "reporting_token"}
}

func (f *fakeTransport) ApplyBotMessageHKDF(secret []byte) []byte { return secret }

func (f *fakeTransport) EnsureTCToken(context.Context, types.JID) ([]byte, error) {
	return f.tcToken, f.tcTokenErr
}

func (f *fakeTransport) ResolveTCTokenStorageLID(_ context.Context, jid types.JID) types.JID {
	return jid.ToNonAD()
}

func (f *fakeTransport) TCTokenSenderTS(types.JID) time.Time { return time.Time{} }

func (f *fakeTransport) IssuePrivacyTokenAndSave(jid types.JID, _ time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issuedTokens = append(f.issuedTokens, jid)
}

func (f *fakeTransport) GenerateCsToken(context.Context, types.JID) []byte { return f.csToken }

var _ Transport = (*fakeTransport)(nil)

func ackNode(attrs waBinary.Attrs) *waBinary.Node {
	return &waBinary.Node{Tag: "ack", Attrs: attrs}
}

// failingMsgSecrets falha em toda gravacao de segredo de mensagem, para provar
// que esse erro e' apenas logado e nao aborta o envio.
type failingMsgSecrets struct{ *store.NoopStore }

func (failingMsgSecrets) PutMessageSecret(context.Context, types.JID, types.JID, types.MessageID, []byte) error {
	return errors.New("boom")
}

// feedAck responde o ack do ID dado assim que o waiter aparecer. Os testes de
// caminho completo precisam dele porque Message bloqueia em AwaitAck ate' o
// servidor responder — sem duble de servidor, o teste esperaria os 75s do
// timeout padrao.
func feedAck(f *fakeTransport, id string, attrs waBinary.Attrs) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			if ch, ok := f.waiter(id); ok {
				ch <- ackNode(attrs)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	return done
}

var errSessionBoom = errors.New("session store boom")

// explodingSessions falha em toda consulta de sessao, para provar que o erro
// do store atravessa a cifragem em vez de virar "sem sessao".
type explodingSessions struct{ *store.NoopStore }

func (explodingSessions) HasSession(context.Context, string) (bool, error) {
	return false, errSessionBoom
}

// stubManyLIDStore falha (ou responde vazio) na consulta em lote de LIDs.
type stubManyLIDStore struct {
	err error
}

func (s stubManyLIDStore) PutManyLIDMappings(context.Context, []store.LIDMapping) error { return nil }
func (s stubManyLIDStore) PutLIDMapping(context.Context, types.JID, types.JID) error    { return nil }
func (s stubManyLIDStore) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}
func (s stubManyLIDStore) GetLIDForPN(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}
func (s stubManyLIDStore) GetManyLIDsForPNs(context.Context, []types.JID) (map[types.JID]types.JID, error) {
	return nil, s.err
}

func messageContextWithSecret() *waE2E.MessageContextInfo {
	return &waE2E.MessageContextInfo{MessageSecret: []byte("0123456789abcdef0123456789abcdef")}
}

// memSenderKeys guarda as chaves de remetente em memoria. Ao contrario do
// NoopStore, ele PERSISTE o que builder.Create grava — sem isso o GroupCipher
// releria um registro vazio e falharia com "no sender key states in record".
//
// Isso torna a cifragem de GRUPO real no teste, e nao um duble: o SenderKey e'
// simetrico, gerado localmente, e nao depende de sessao com ninguem. So' a
// cifragem por DISPOSITIVO e' que precisa de sessao Signal viva.
type memSenderKeys struct {
	*store.NoopStore
	keys map[string][]byte
}

func newMemSenderKeys() *memSenderKeys {
	return &memSenderKeys{&store.NoopStore{}, map[string][]byte{}}
}

func (m *memSenderKeys) PutSenderKey(_ context.Context, group, user string, session []byte) error {
	m.keys[group+"/"+user] = session
	return nil
}

func (m *memSenderKeys) GetSenderKey(_ context.Context, group, user string) ([]byte, error) {
	return m.keys[group+"/"+user], nil
}

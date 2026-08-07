package retry

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"go.mau.fi/libsignal/keys/prekey"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/prekeys"
	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/internal/wa-noise/security/keys"
	waLog "wa-api/internal/wa-noise/observability/log"
)

var (
	testOwnJID   = types.NewJID("111", types.DefaultUserServer)
	testOwnLID   = types.NewJID("222", types.HiddenUserServer)
	testPeerJID  = types.NewJID("333", types.DefaultUserServer)
	testPeerLID  = types.NewJID("444", types.HiddenUserServer)
	testGroupJID = types.NewJID("555", types.GroupServer)
)

// fakeStores implementa os stores especificos de sessao com comportamento
// controlavel. Os metodos nao sobrescritos vem do NoopStore, que devolve erro
// — o que e' desejavel: um teste que dependa de um metodo nao previsto falha em
// vez de passar por acidente.
type fakeStores struct {
	*store.NoopStore

	mu sync.Mutex

	// EventBuffer
	outgoing        map[types.MessageID][2]any // id -> {format string, buf []byte}
	addOutErr       error
	getOutErr       error
	deleteOldCalls  int
	deleteOldErr    error
	addOutgoingCall int

	// PreKeys
	genOneKey *keys.PreKey
	genOneErr error

	// sessoes
	hasSession    bool
	hasSessionErr error
}

func newFakeStores() *fakeStores {
	return &fakeStores{
		NoopStore: &store.NoopStore{Error: errors.New("nao previsto neste teste")},
		outgoing:  make(map[types.MessageID][2]any),
	}
}

func (f *fakeStores) AddOutgoingEvent(_ context.Context, _ types.JID, id types.MessageID, format string, plaintext []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addOutgoingCall++
	if f.addOutErr != nil {
		return f.addOutErr
	}
	f.outgoing[id] = [2]any{format, plaintext}
	return nil
}

func (f *fakeStores) GetOutgoingEvent(_ context.Context, _, _ types.JID, id types.MessageID) (string, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getOutErr != nil {
		return "", nil, f.getOutErr
	}
	entry, ok := f.outgoing[id]
	if !ok {
		return "", nil, nil
	}
	return entry[0].(string), entry[1].([]byte), nil
}

func (f *fakeStores) DeleteOldOutgoingEvents(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteOldCalls++
	return f.deleteOldErr
}

func (f *fakeStores) GenOnePreKey(context.Context) (*keys.PreKey, error) {
	return f.genOneKey, f.genOneErr
}

func (f *fakeStores) HasSession(context.Context, string) (bool, error) {
	return f.hasSession, f.hasSessionErr
}

// fakeLIDs implementa store.LIDStore com um mapa de mao dupla.
type fakeLIDs struct {
	*store.NoopStore
	pnToLID map[types.JID]types.JID
	lidToPN map[types.JID]types.JID
	err     error
}

func newFakeLIDs() *fakeLIDs {
	return &fakeLIDs{
		NoopStore: &store.NoopStore{Error: errors.New("nao previsto neste teste")},
		pnToLID:   make(map[types.JID]types.JID),
		lidToPN:   make(map[types.JID]types.JID),
	}
}

func (f *fakeLIDs) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	return f.pnToLID[pn], f.err
}

func (f *fakeLIDs) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	return f.lidToPN[lid], f.err
}

// fakeTransport e' o duble de Transport. Cada campo *Err/*Result controla um
// ramo; os campos de contagem/captura registram o que o dominio pediu.
type fakeTransport struct {
	state  State
	stores *fakeStores
	lids   *fakeLIDs
	device *store.Device

	backgroundCtx context.Context

	useMessageStore   bool
	synchronousAck    bool
	rerequestEnabled  bool
	rerequestDelay    time.Duration
	preRetryAllowed   bool
	messageForRetry   *waE2E.Message
	fetchPreKeysResp  map[types.JID]prekeys.Resp
	fetchPreKeysErr   error
	skdmErr           error
	encryptErr        error
	encryptV3Err      error
	includeIdentity   bool
	requestUnavailErr error
	sendNodeErr       error

	mu           sync.Mutex
	sentNodes    []waBinary.Node
	migrated     [][2]types.JID
	unavailReqs  []types.MessageID
	skdmChats    []types.JID
	preRetryArgs []int
	encV3Payload *waMsgTransport.MessageTransport_Payload
}

var _ Transport = (*fakeTransport)(nil)

func newFakeTransport() *fakeTransport {
	stores := newFakeStores()
	// Sessao Signal existente e' o caso comum: sem isso, todo teste do caminho
	// feliz cairia no ramo de recriacao de sessao e precisaria de um bundle.
	stores.hasSession = true
	lids := newFakeLIDs()
	ownID := testOwnJID
	device := &store.Device{
		ID:             &ownID,
		LID:            testOwnLID,
		RegistrationID: 0xDEADBEEF,
		IdentityKey:    keys.NewKeyPair(),
		SignedPreKey:   &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 7, Signature: &[64]byte{}},
		Account:        &waAdv.ADVSignedDeviceIdentity{Details: []byte("detalhes")},
	}
	device.SetAllStores(stores)
	device.LIDs = lids
	return &fakeTransport{
		state:            State{},
		stores:           stores,
		lids:             lids,
		device:           device,
		backgroundCtx:    context.Background(),
		rerequestDelay:   time.Millisecond,
		preRetryAllowed:  true,
		fetchPreKeysResp: make(map[types.JID]prekeys.Resp),
	}
}

func (f *fakeTransport) Store() *store.Device            { return f.device }
func (f *fakeTransport) State() *State                   { return &f.state }
func (f *fakeTransport) Log() waLog.Logger               { return waLog.Noop }
func (f *fakeTransport) BackgroundCtx() context.Context  { return f.backgroundCtx }
func (f *fakeTransport) OwnLID() types.JID               { return testOwnLID }
func (f *fakeTransport) UseMessageStore() bool           { return f.useMessageStore }
func (f *fakeTransport) SynchronousAck() bool            { return f.synchronousAck }
func (f *fakeTransport) RerequestFromPhoneEnabled() bool { return f.rerequestEnabled }
func (f *fakeTransport) RerequestDelay() time.Duration   { return f.rerequestDelay }

func (f *fakeTransport) SendNode(_ context.Context, node waBinary.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentNodes = append(f.sentNodes, node)
	return f.sendNodeErr
}

func (f *fakeTransport) PreRetryAllowed(_ *events.Receipt, _ types.MessageID, retryCount int, _ *waE2E.Message) bool {
	f.mu.Lock()
	f.preRetryArgs = append(f.preRetryArgs, retryCount)
	f.mu.Unlock()
	return f.preRetryAllowed
}

func (f *fakeTransport) GetMessageForRetry(_, _ types.JID, _ types.MessageID) *waE2E.Message {
	return f.messageForRetry
}

func (f *fakeTransport) FetchPreKeys(context.Context, []types.JID) (map[types.JID]prekeys.Resp, error) {
	return f.fetchPreKeysResp, f.fetchPreKeysErr
}

func (f *fakeTransport) MigrateSessionStore(_ context.Context, pn, lid types.JID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.migrated = append(f.migrated, [2]types.JID{pn, lid})
}

func (f *fakeTransport) CreateSKDM(_ context.Context, chat types.JID) ([]byte, error) {
	f.mu.Lock()
	f.skdmChats = append(f.skdmChats, chat)
	f.mu.Unlock()
	if f.skdmErr != nil {
		return nil, f.skdmErr
	}
	return []byte("skdm"), nil
}

func (f *fakeTransport) EncryptForDevice(_ context.Context, _ []byte, _ types.JID, _ *prekey.Bundle, extraAttrs waBinary.Attrs) (*waBinary.Node, bool, error) {
	if f.encryptErr != nil {
		return nil, false, f.encryptErr
	}
	return &waBinary.Node{Tag: "enc", Attrs: cloneAttrs(extraAttrs)}, f.includeIdentity, nil
}

func (f *fakeTransport) EncryptForDeviceV3(
	_ context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	_ *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	_ *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	_ types.JID,
	_ *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	f.mu.Lock()
	f.encV3Payload = payload
	f.mu.Unlock()
	if f.encryptV3Err != nil {
		return nil, f.encryptV3Err
	}
	return &waBinary.Node{Tag: "enc", Attrs: cloneAttrs(extraAttrs)}, nil
}

func (f *fakeTransport) MessageContent(baseNode waBinary.Node, _ *waE2E.Message, _ waBinary.Attrs, includeIdentity bool) []waBinary.Node {
	content := []waBinary.Node{baseNode}
	if includeIdentity {
		content = append(content, waBinary.Node{Tag: "device-identity"})
	}
	return content
}

func (f *fakeTransport) BuildBaseReceipt(id string, node *waBinary.Node) waBinary.Attrs {
	return waBinary.Attrs{"id": id, "to": node.Attrs["from"]}
}

func (f *fakeTransport) RequestUnavailableMessage(_ context.Context, _, _ types.JID, id types.MessageID) error {
	f.mu.Lock()
	f.unavailReqs = append(f.unavailReqs, id)
	f.mu.Unlock()
	return f.requestUnavailErr
}

func (f *fakeTransport) ElementMissing(tag, in string) error {
	return errMissing{tag: tag, in: in}
}

type errMissing struct{ tag, in string }

func (e errMissing) Error() string { return "missing " + e.tag + " in " + e.in }

func cloneAttrs(in waBinary.Attrs) waBinary.Attrs {
	out := waBinary.Attrs{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// --- helpers de montagem ---

func waMessage(text string) *waE2E.Message {
	return &waE2E.Message{Conversation: proto.String(text)}
}

// retryNode monta o <receipt> com o filho <retry> que HandleReceipt espera.
func retryNode(id string, count int, extra waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
	attrs := waBinary.Attrs{"from": testPeerJID}
	for k, v := range extra {
		attrs[k] = v
	}
	// O decodificador do wire entrega atributos como string.
	content := append([]waBinary.Node{{Tag: "retry", Attrs: waBinary.Attrs{
		"id": id, "t": "1700000000", "count": strconv.Itoa(count),
	}}}, children...)
	return &waBinary.Node{Tag: "receipt", Attrs: attrs, Content: content}
}

func dmReceipt(id types.MessageID) *events.Receipt {
	return &events.Receipt{MessageSource: types.MessageSource{
		Chat: testPeerJID, Sender: testPeerJID,
	}, MessageIDs: []types.MessageID{id}}
}

func (f *fakeTransport) sent() []waBinary.Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]waBinary.Node(nil), f.sentNodes...)
}

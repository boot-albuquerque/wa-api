package pairing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	waLog "wa-api/internal/wa-noise/observability/log"
	"wa-api/internal/wa-noise/persistence/store"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/security/keys"
)

// missingElementError e' o duble do *wanoise.ElementMissingError da raiz. O
// tipo concreto continua la'; aqui so' interessa que a construcao atravessa a
// interface e devolve um erro identificavel.
type missingElementError struct {
	Tag string
	In  string
}

func (e *missingElementError) Error() string {
	return fmt.Sprintf("failed to find %s in %s", e.Tag, e.In)
}

// fakeIdentityStore registra as identidades gravadas durante o pareamento.
type fakeIdentityStore struct {
	mu   sync.Mutex
	put  map[string][32]byte
	err  error
	call int
}

func (f *fakeIdentityStore) PutIdentity(_ context.Context, address string, key [32]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call++
	if f.err != nil {
		return f.err
	}
	if f.put == nil {
		f.put = map[string][32]byte{}
	}
	f.put[address] = key
	return nil
}

func (f *fakeIdentityStore) DeleteAllIdentities(context.Context, string) error { return nil }
func (f *fakeIdentityStore) DeleteIdentity(context.Context, string) error      { return nil }
func (f *fakeIdentityStore) IsTrustedIdentity(context.Context, string, [32]byte) (bool, error) {
	return true, nil
}

// fakeTransport e' o duble de Transport: nenhum socket, nenhuma sessao Noise.
type fakeTransport struct {
	mu sync.Mutex

	device *store.Device
	state  State

	iqQueue []iqResult
	iqCalls []IQ

	sentNodes []waBinary.Node
	sendErrs  []error

	events []any

	configuredClientType ClientType
	prePairAllows        bool
	prePairCalls         int

	lidMappings       [][2]types.JID
	expectDisconnects int
	disconnects       int
	unifiedSessions   int
	timeOffset        int64
}

type iqResult struct {
	node *waBinary.Node
	err  error
}

// fakeDevice monta um store.Device com chaves deterministas.
func fakeDevice(t *testing.T, saveErr, deleteErr error, ids *fakeIdentityStore) *store.Device {
	t.Helper()
	dev := &store.Device{
		NoiseKey:     keys.NewKeyPair(),
		IdentityKey:  keys.NewKeyPair(),
		AdvSecretKey: []byte("adv-secret-key-para-testes-32byt"),
		Identities:   ids,
	}
	dev.Container = &fakeContainer{saveErr: saveErr, deleteErr: deleteErr}
	return dev
}

// fakeContainer e' o duble de store.DeviceContainer.
type fakeContainer struct {
	mu        sync.Mutex
	saveErr   error
	deleteErr error
	saves     int
	deletes   int
}

func (f *fakeContainer) PutDevice(context.Context, *store.Device) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	return f.saveErr
}

func (f *fakeContainer) DeleteDevice(context.Context, *store.Device) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	return f.deleteErr
}

func newFakeTransport(t *testing.T) *fakeTransport {
	t.Helper()
	ids := &fakeIdentityStore{}
	return &fakeTransport{
		device:        fakeDevice(t, nil, nil, ids),
		prePairAllows: true,
	}
}

func (f *fakeTransport) identities() *fakeIdentityStore {
	return f.device.Identities.(*fakeIdentityStore)
}

func (f *fakeTransport) enqueueIQ(node *waBinary.Node, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iqQueue = append(f.iqQueue, iqResult{node, err})
}

// enqueueSendErr empilha o erro que a proxima chamada de SendNode devolve.
func (f *fakeTransport) enqueueSendErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendErrs = append(f.sendErrs, err)
}

func (f *fakeTransport) Store() *store.Device { return f.device }
func (f *fakeTransport) State() *State        { return &f.state }
func (f *fakeTransport) Log() waLog.Logger    { return waLog.Noop }

func (f *fakeTransport) SendNode(_ context.Context, node waBinary.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentNodes = append(f.sentNodes, node)
	if len(f.sendErrs) == 0 {
		return nil
	}
	err := f.sendErrs[0]
	f.sendErrs = f.sendErrs[1:]
	return err
}

func (f *fakeTransport) SendIQ(_ context.Context, query IQ) (*waBinary.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iqCalls = append(f.iqCalls, query)
	if len(f.iqQueue) == 0 {
		return nil, errors.New("fakeTransport: no queued IQ response")
	}
	res := f.iqQueue[0]
	f.iqQueue = f.iqQueue[1:]
	return res.node, res.err
}

func (f *fakeTransport) DispatchEvent(evt any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evt)
}

func (f *fakeTransport) ConfiguredClientType() ClientType { return f.configuredClientType }

func (f *fakeTransport) PrePairAllowed(types.JID, string, string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prePairCalls++
	return f.prePairAllows
}

func (f *fakeTransport) StoreLIDPNMapping(_ context.Context, lid, pn types.JID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lidMappings = append(f.lidMappings, [2]types.JID{lid, pn})
}

func (f *fakeTransport) ExpectDisconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expectDisconnects++
}

func (f *fakeTransport) Disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects++
}

func (f *fakeTransport) SendUnifiedSession() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unifiedSessions++
}

func (f *fakeTransport) SetServerTimeOffset(offset int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeOffset = offset
}

func (f *fakeTransport) ElementMissing(tag, in string) error {
	return &missingElementError{Tag: tag, In: in}
}

// Acessores com lock, para os testes que rodam sob -race com a goroutine de
// HandleSuccessNode viva.
func (f *fakeTransport) snapshotEvents() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]any(nil), f.events...)
}

func (f *fakeTransport) snapshotNodes() []waBinary.Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]waBinary.Node(nil), f.sentNodes...)
}

func (f *fakeTransport) snapshotIQs() []IQ {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]IQ(nil), f.iqCalls...)
}

var _ Transport = (*fakeTransport)(nil)

package prekeys

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/security/keys"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// testPreKey monta uma prekey determinista (sem aleatoriedade) para que os
// goldens de wire sejam estaveis.
func testPreKey(t *testing.T, keyID uint32, signed bool) *keys.PreKey {
	t.Helper()
	key := &keys.PreKey{KeyID: keyID}
	key.Pub = (*[pubLength]byte)(bytes.Repeat([]byte{0xAB}, pubLength))
	key.Priv = (*[pubLength]byte)(bytes.Repeat([]byte{0xCD}, pubLength))
	if signed {
		key.Signature = (*[signatureLength]byte)(bytes.Repeat([]byte{0xEF}, signatureLength))
	}
	return key
}

// fakePreKeyStore e' um store.PreKeyStore em memoria. So' os tres metodos que
// este dominio usa fazem algo; os outros existem para satisfazer a interface.
type fakePreKeyStore struct {
	mu sync.Mutex

	genKeys    []*keys.PreKey
	genErr     error
	genCount   uint32
	genCalls   int
	markedUpTo uint32
	markErr    error
	markCalls  int
}

func (f *fakePreKeyStore) GetOrGenPreKeys(_ context.Context, count uint32) ([]*keys.PreKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.genCalls++
	f.genCount = count
	return f.genKeys, f.genErr
}

func (f *fakePreKeyStore) MarkPreKeysAsUploaded(_ context.Context, upToID uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls++
	f.markedUpTo = upToID
	return f.markErr
}

func (f *fakePreKeyStore) GenOnePreKey(context.Context) (*keys.PreKey, error) {
	return nil, errors.New("not used by this domain")
}

func (f *fakePreKeyStore) GetPreKey(context.Context, uint32) (*keys.PreKey, error) {
	return nil, errors.New("not used by this domain")
}

func (f *fakePreKeyStore) RemovePreKey(context.Context, uint32) error {
	return errors.New("not used by this domain")
}

func (f *fakePreKeyStore) UploadedPreKeyCount(context.Context) (int, error) {
	return 0, errors.New("not used by this domain")
}

// fakeTransport e' o duble de Transport. Nao tem socket nem sessao Noise: as
// respostas de <iq> sao roteiras, indexadas pela ordem das chamadas.
type fakeTransport struct {
	mu sync.Mutex

	device  *store.Device
	state   State
	iqQueue []iqResult
	iqCalls []IQ
}

type iqResult struct {
	node *waBinary.Node
	err  error
}

func newFakeTransport(t *testing.T) *fakeTransport {
	t.Helper()
	identity := &keys.KeyPair{}
	identity.Pub = (*[pubLength]byte)(bytes.Repeat([]byte{0x22}, pubLength))
	identity.Priv = (*[pubLength]byte)(bytes.Repeat([]byte{0x33}, pubLength))
	return &fakeTransport{
		device: &store.Device{
			RegistrationID: 4242,
			IdentityKey:    identity,
			SignedPreKey:   testPreKey(t, 77, true),
			PreKeys:        &fakePreKeyStore{},
		},
	}
}

func (f *fakeTransport) preKeyStore() *fakePreKeyStore {
	return f.device.PreKeys.(*fakePreKeyStore)
}

// enqueueIQ empilha a proxima resposta de SendIQ.
func (f *fakeTransport) enqueueIQ(node *waBinary.Node, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iqQueue = append(f.iqQueue, iqResult{node, err})
}

func (f *fakeTransport) Store() *store.Device { return f.device }

func (f *fakeTransport) State() *State { return &f.state }

func (f *fakeTransport) Log() waLog.Logger { return waLog.Noop }

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

func (f *fakeTransport) calls() []IQ {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]IQ(nil), f.iqCalls...)
}

var _ Transport = (*fakeTransport)(nil)

// countNode monta a resposta de <iq> de contagem de prekeys.
func countNode(value string) *waBinary.Node {
	return &waBinary.Node{Tag: "iq", Content: []waBinary.Node{
		{Tag: "count", Attrs: waBinary.Attrs{"value": value}},
	}}
}

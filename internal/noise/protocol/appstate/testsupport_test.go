package appstate

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/persistence/store"
)

// memAppStateStore e' uma implementacao em memoria de store.AppStateStore e
// store.AppStateSyncKeyStore, suficiente para exercitar Processor sem banco.
// Vive so' no pacote de teste — nada em internal/noise/store/ foi alterado.
type memAppStateStore struct {
	versions map[string]uint64
	hashes   map[string][lthashLength]byte
	macs     map[string]map[string][]byte
	keys     map[string]store.AppStateSyncKey

	putVersionErr error
	putMACsErr    error
	deleteMACsErr error
	getMACErr     error
	getKeyErr     error
}

func newMemStore() *memAppStateStore {
	return &memAppStateStore{
		versions: map[string]uint64{},
		hashes:   map[string][lthashLength]byte{},
		macs:     map[string]map[string][]byte{},
		keys:     map[string]store.AppStateSyncKey{},
	}
}

func (m *memAppStateStore) device() *store.Device {
	return &store.Device{AppState: m, AppStateKeys: m}
}

func macKey(indexMAC []byte) string {
	return base64.RawStdEncoding.EncodeToString(indexMAC)
}

func (m *memAppStateStore) PutAppStateVersion(_ context.Context, name string, version uint64, hash [lthashLength]byte) error {
	if m.putVersionErr != nil {
		return m.putVersionErr
	}
	m.versions[name] = version
	m.hashes[name] = hash
	return nil
}

func (m *memAppStateStore) GetAppStateVersion(_ context.Context, name string) (uint64, [lthashLength]byte, error) {
	return m.versions[name], m.hashes[name], nil
}

func (m *memAppStateStore) DeleteAppStateVersion(_ context.Context, name string) error {
	delete(m.versions, name)
	delete(m.hashes, name)
	return nil
}

func (m *memAppStateStore) PutAppStateMutationMACs(_ context.Context, name string, _ uint64, mutations []store.AppStateMutationMAC) error {
	if m.putMACsErr != nil {
		return m.putMACsErr
	}
	if m.macs[name] == nil {
		m.macs[name] = map[string][]byte{}
	}
	for _, mut := range mutations {
		m.macs[name][macKey(mut.IndexMAC)] = mut.ValueMAC
	}
	return nil
}

func (m *memAppStateStore) DeleteAppStateMutationMACs(_ context.Context, name string, indexMACs [][]byte) error {
	if m.deleteMACsErr != nil {
		return m.deleteMACsErr
	}
	for _, indexMAC := range indexMACs {
		delete(m.macs[name], macKey(indexMAC))
	}
	return nil
}

func (m *memAppStateStore) GetAppStateMutationMAC(_ context.Context, name string, indexMAC []byte) ([]byte, error) {
	if m.getMACErr != nil {
		return nil, m.getMACErr
	}
	return m.macs[name][macKey(indexMAC)], nil
}

func (m *memAppStateStore) PutAppStateSyncKey(_ context.Context, id []byte, key store.AppStateSyncKey) error {
	m.keys[macKey(id)] = key
	return nil
}

func (m *memAppStateStore) GetAppStateSyncKey(_ context.Context, id []byte) (*store.AppStateSyncKey, error) {
	if m.getKeyErr != nil {
		return nil, m.getKeyErr
	}
	key, ok := m.keys[macKey(id)]
	if !ok {
		return nil, nil
	}
	return &key, nil
}

func (m *memAppStateStore) GetLatestAppStateSyncKeyID(context.Context) ([]byte, error) {
	return nil, errors.New("not implemented in memAppStateStore")
}

func (m *memAppStateStore) GetAllAppStateSyncKeys(context.Context) ([]*store.AppStateSyncKey, error) {
	return nil, errors.New("not implemented in memAppStateStore")
}

var _ store.AppStateStore = (*memAppStateStore)(nil)
var _ store.AppStateSyncKeyStore = (*memAppStateStore)(nil)

// testKeyID e' o key ID usado por todos os testes que precisam de uma chave.
var testKeyID = []byte{0xDE, 0xAD, 0xBE, 0xEF}

// newTestProcessor devolve um Processor com store em memoria, ja' com uma
// chave de app state registrada sob testKeyID.
func newTestProcessor(t *testing.T) (*Processor, *memAppStateStore) {
	t.Helper()
	mem := newMemStore()
	keyData := make([]byte, appStateKeyPartLength)
	for i := range keyData {
		keyData[i] = byte(i)
	}
	err := mem.PutAppStateSyncKey(context.Background(), testKeyID, store.AppStateSyncKey{Data: keyData})
	if err != nil {
		t.Fatalf("PutAppStateSyncKey: %v", err)
	}
	return NewProcessor(mem.device(), sdklog.Noop), mem
}

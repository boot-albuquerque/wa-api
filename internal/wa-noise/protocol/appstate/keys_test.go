package appstate

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/security/hkdf"
)

func TestExpandAppStateKeysSplitsHKDFOutput(t *testing.T) {
	keyData := []byte("some app state sync key material")
	expanded := hkdfutil.SHA256(keyData, nil, []byte(appStateKeyHKDFInfo), appStateKeyExpandedLength)
	if len(expanded) != appStateKeyExpandedLength {
		t.Fatalf("HKDF devolveu %d bytes, esperado %d", len(expanded), appStateKeyExpandedLength)
	}

	keys := expandAppStateKeys(keyData)
	parts := [][]byte{keys.Index, keys.ValueEncryption, keys.ValueMAC, keys.SnapshotMAC, keys.PatchMAC}
	if len(parts) != appStateKeyPartCount {
		t.Fatalf("ExpandedAppStateKeys tem %d subchaves, appStateKeyPartCount diz %d", len(parts), appStateKeyPartCount)
	}
	for i, part := range parts {
		if len(part) != appStateKeyPartLength {
			t.Errorf("subchave #%d tem %d bytes, esperado %d", i, len(part), appStateKeyPartLength)
		}
		want := expanded[i*appStateKeyPartLength : (i+1)*appStateKeyPartLength]
		if !bytes.Equal(part, want) {
			t.Errorf("subchave #%d = %X, esperado %X", i, part, want)
		}
	}
}

func TestExpandAppStateKeysIsDeterministicAndDistinct(t *testing.T) {
	a := expandAppStateKeys([]byte("key"))
	b := expandAppStateKeys([]byte("key"))
	if !bytes.Equal(a.Index, b.Index) || !bytes.Equal(a.PatchMAC, b.PatchMAC) {
		t.Fatal("expandAppStateKeys nao e' deterministica")
	}
	if bytes.Equal(a.Index, a.ValueMAC) || bytes.Equal(a.SnapshotMAC, a.PatchMAC) {
		t.Fatal("subchaves distintas nao deveriam coincidir")
	}
	c := expandAppStateKeys([]byte("outra key"))
	if bytes.Equal(a.Index, c.Index) {
		t.Fatal("chaves de entrada diferentes produziram a mesma subchave")
	}
}

func TestGetAppStateKeyCachesAndReportsMissing(t *testing.T) {
	proc, mem := newTestProcessor(t)
	ctx := context.Background()

	keys, err := proc.getAppStateKey(ctx, testKeyID)
	if err != nil {
		t.Fatalf("getAppStateKey: %v", err)
	}
	if len(keys.Index) != appStateKeyPartLength {
		t.Fatalf("Index tem %d bytes", len(keys.Index))
	}

	// Depois de cacheada, apagar do store nao deve afetar o resultado.
	delete(mem.keys, macKey(testKeyID))
	cached, err := proc.getAppStateKey(ctx, testKeyID)
	if err != nil {
		t.Fatalf("getAppStateKey (cache): %v", err)
	}
	if !bytes.Equal(cached.Index, keys.Index) {
		t.Error("cache devolveu subchave diferente")
	}

	if _, err = proc.getAppStateKey(ctx, []byte{0x01}); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("chave ausente devolveu %v, esperado ErrKeyNotFound", err)
	}
}

func TestGetAppStateKeyPropagatesStoreError(t *testing.T) {
	proc, mem := newTestProcessor(t)
	storeErr := errors.New("boom")
	mem.getKeyErr = storeErr
	if _, err := proc.getAppStateKey(context.Background(), []byte{0x99}); !errors.Is(err, storeErr) {
		t.Errorf("erro devolvido = %v, esperado %v", err, storeErr)
	}
}

func TestGetMissingKeyIDs(t *testing.T) {
	proc, _ := newTestProcessor(t)
	known := &waServerSync.KeyId{ID: testKeyID}
	missing := &waServerSync.KeyId{ID: []byte{0xAA}}

	list := &PatchList{
		Snapshot: &waServerSync.SyncdSnapshot{
			KeyID:   known,
			Records: []*waServerSync.SyncdRecord{{KeyID: missing}, {KeyID: missing}},
		},
		Patches: []*waServerSync.SyncdPatch{{KeyID: missing}, {KeyID: known}},
	}

	got := proc.GetMissingKeyIDs(context.Background(), list)
	// Deduplicado: o mesmo key ID ausente aparece 3x na lista.
	if len(got) != 1 {
		t.Fatalf("GetMissingKeyIDs devolveu %d chaves (%X), esperado 1", len(got), got)
	}
	if !bytes.Equal(got[0], missing.ID) {
		t.Errorf("chave ausente = %X, esperado %X", got[0], missing.ID)
	}
}

func TestGetMissingKeyIDsIgnoresNilKeyIDs(t *testing.T) {
	proc, _ := newTestProcessor(t)
	list := &PatchList{Patches: []*waServerSync.SyncdPatch{{}}}
	if got := proc.GetMissingKeyIDs(context.Background(), list); len(got) != 0 {
		t.Errorf("key ID nil gerou %d entradas, esperado 0", len(got))
	}
}

func TestNewProcessorWiresDependencies(t *testing.T) {
	mem := newMemStore()
	dev := mem.device()
	proc := NewProcessor(dev, nil)
	if proc.Store != dev {
		t.Error("NewProcessor nao guardou o Device recebido")
	}
	if proc.keyCache == nil {
		t.Error("NewProcessor deixou keyCache nil")
	}
	var _ *store.Device = proc.Store
}

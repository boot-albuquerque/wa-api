package appstate

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/appstate/lthash"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
)

func mutationWithBlobs(op waServerSync.SyncdMutation_SyncdOperation, indexMAC, valueMAC []byte) *waServerSync.SyncdMutation {
	return &waServerSync.SyncdMutation{
		Operation: op.Enum(),
		Record: &waServerSync.SyncdRecord{
			Index: &waServerSync.SyncdIndex{Blob: indexMAC},
			Value: &waServerSync.SyncdValue{Blob: valueMAC},
			KeyID: &waServerSync.KeyId{ID: testKeyID},
		},
	}
}

func fillBytes(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestUint64ToBytesIsBigEndian(t *testing.T) {
	got := uint64ToBytes(0x0102030405060708)
	if len(got) != versionByteLength {
		t.Fatalf("uint64ToBytes devolveu %d bytes, esperado %d", len(got), versionByteLength)
	}
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if !bytes.Equal(got, want) {
		t.Errorf("uint64ToBytes = %X, esperado %X", got, want)
	}
	if binary.BigEndian.Uint64(got) != 0x0102030405060708 {
		t.Error("round trip big-endian falhou")
	}
}

func TestConcatAndHMACConcatenatesInOrder(t *testing.T) {
	key := []byte("chave")
	joined := concatAndHMAC(sha256.New, key, []byte("ab"), []byte("cd"))
	single := concatAndHMAC(sha256.New, key, []byte("abcd"))
	if !hmac.Equal(joined, single) {
		t.Error("concatAndHMAC de partes deveria igualar o HMAC do concatenado")
	}
	swapped := concatAndHMAC(sha256.New, key, []byte("cd"), []byte("ab"))
	if hmac.Equal(joined, swapped) {
		t.Error("a ordem das partes deveria alterar o HMAC")
	}
	if len(joined) != macLength {
		t.Errorf("HMAC-SHA256 tem %d bytes, esperado %d", len(joined), macLength)
	}
}

func TestGenerateSnapshotMACDependsOnHashVersionAndName(t *testing.T) {
	key := fillBytes(macLength, 0x11)
	base := HashState{Version: 7}
	copy(base.Hash[:], fillBytes(lthashLength, 0x22))

	mac := base.generateSnapshotMAC(WAPatchRegular, key)
	if len(mac) != macLength {
		t.Fatalf("snapshot MAC tem %d bytes, esperado %d", len(mac), macLength)
	}

	otherName := base.generateSnapshotMAC(WAPatchRegularHigh, key)
	if hmac.Equal(mac, otherName) {
		t.Error("nome do patch deveria entrar no snapshot MAC")
	}

	otherVersion := base
	otherVersion.Version = 8
	if hmac.Equal(mac, otherVersion.generateSnapshotMAC(WAPatchRegular, key)) {
		t.Error("versao deveria entrar no snapshot MAC")
	}

	otherHash := base
	otherHash.Hash[0] ^= 0xFF
	if hmac.Equal(mac, otherHash.generateSnapshotMAC(WAPatchRegular, key)) {
		t.Error("hash deveria entrar no snapshot MAC")
	}
}

func TestGeneratePatchMACCoversAllValueMACs(t *testing.T) {
	key := fillBytes(macLength, 0x33)
	patch := &waServerSync.SyncdPatch{
		SnapshotMAC: fillBytes(macLength, 0x44),
		Mutations: []*waServerSync.SyncdMutation{
			mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 1), fillBytes(macLength, 2)),
			mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 3), fillBytes(macLength, 4)),
		},
	}

	mac := generatePatchMAC(patch, WAPatchRegular, key, 3)
	if len(mac) != macLength {
		t.Fatalf("patch MAC tem %d bytes", len(mac))
	}
	if hmac.Equal(mac, generatePatchMAC(patch, WAPatchRegular, key, 4)) {
		t.Error("versao deveria entrar no patch MAC")
	}
	if hmac.Equal(mac, generatePatchMAC(patch, WAPatchRegularLow, key, 3)) {
		t.Error("nome deveria entrar no patch MAC")
	}

	altered := proto.Clone(patch).(*waServerSync.SyncdPatch)
	altered.Mutations[1].Record.Value.Blob = fillBytes(macLength, 9)
	if hmac.Equal(mac, generatePatchMAC(altered, WAPatchRegular, key, 3)) {
		t.Error("value MAC de uma mutacao deveria entrar no patch MAC")
	}
}

func TestGenerateContentMACIsTruncatedAndOperationBound(t *testing.T) {
	key := fillBytes(macLength, 0x55)
	data := []byte("conteudo cifrado")

	set := generateContentMAC(waServerSync.SyncdMutation_SET, data, testKeyID, key)
	if len(set) != macLength {
		t.Fatalf("content MAC tem %d bytes, esperado %d (SHA-512 truncado)", len(set), macLength)
	}
	remove := generateContentMAC(waServerSync.SyncdMutation_REMOVE, data, testKeyID, key)
	if hmac.Equal(set, remove) {
		t.Error("a operacao deveria entrar no content MAC")
	}
	if hmac.Equal(set, generateContentMAC(waServerSync.SyncdMutation_SET, data, []byte{0x01}, key)) {
		t.Error("o key ID deveria entrar no content MAC")
	}
	if hmac.Equal(set, generateContentMAC(waServerSync.SyncdMutation_SET, []byte("outro"), testKeyID, key)) {
		t.Error("o conteudo deveria entrar no content MAC")
	}
}

func TestUpdateHashAddsSetAndSubtractsPreviousValue(t *testing.T) {
	valueMAC := fillBytes(macLength, 0x66)
	indexMAC := fillBytes(macLength, 0x77)

	var hs HashState
	warn, err := hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_SET, indexMAC, valueMAC)},
		func([]byte, int) ([]byte, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("updateHash: %v", err)
	}
	if len(warn) != 0 {
		t.Fatalf("SET sem valor anterior nao deveria avisar: %v", warn)
	}

	var want [lthashLength]byte
	lthash.WAPatchIntegrity.SubtractThenAddInPlace(want[:], nil, [][]byte{valueMAC})
	if hs.Hash != want {
		t.Error("hash apos SET diverge do LTHash calculado diretamente")
	}

	// Reaplicar o mesmo SET informando o valor anterior deve ser idempotente.
	warn, err = hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_SET, indexMAC, valueMAC)},
		func([]byte, int) ([]byte, error) { return valueMAC, nil },
	)
	if err != nil || len(warn) != 0 {
		t.Fatalf("updateHash idempotente: err=%v warn=%v", err, warn)
	}
	if hs.Hash != want {
		t.Error("reaplicar o mesmo SET deveria manter o hash")
	}
}

func TestUpdateHashRemoveWithoutPreviousValueWarns(t *testing.T) {
	var hs HashState
	warn, err := hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_REMOVE, fillBytes(macLength, 1), nil)},
		func([]byte, int) ([]byte, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("updateHash: %v", err)
	}
	if len(warn) != 1 || !errors.Is(warn[0], ErrMissingPreviousSetValueOperation) {
		t.Fatalf("warnings = %v, esperado ErrMissingPreviousSetValueOperation", warn)
	}
	if hs.Hash != ([lthashLength]byte{}) {
		t.Error("REMOVE sem valor anterior nao deveria alterar o hash")
	}
}

func TestUpdateHashRemoveSubtractsPreviousValue(t *testing.T) {
	valueMAC := fillBytes(macLength, 0x88)
	indexMAC := fillBytes(macLength, 0x99)

	var hs HashState
	if _, err := hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_SET, indexMAC, valueMAC)},
		func([]byte, int) ([]byte, error) { return nil, nil },
	); err != nil {
		t.Fatalf("SET: %v", err)
	}
	if _, err := hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_REMOVE, indexMAC, nil)},
		func([]byte, int) ([]byte, error) { return valueMAC, nil },
	); err != nil {
		t.Fatalf("REMOVE: %v", err)
	}
	if hs.Hash != ([lthashLength]byte{}) {
		t.Error("SET seguido de REMOVE do mesmo valor deveria zerar o hash")
	}
}

func TestUpdateHashPropagatesLookupError(t *testing.T) {
	lookupErr := errors.New("db caiu")
	var hs HashState
	_, err := hs.updateHash(
		[]*waServerSync.SyncdMutation{mutationWithBlobs(waServerSync.SyncdMutation_SET, nil, fillBytes(macLength, 1))},
		func([]byte, int) ([]byte, error) { return nil, lookupErr },
	)
	if !errors.Is(err, lookupErr) {
		t.Errorf("err = %v, esperado %v", err, lookupErr)
	}
}

func TestUpdateHashPassesMutationIndexAsMaxIndex(t *testing.T) {
	var seen []int
	var hs HashState
	mutations := []*waServerSync.SyncdMutation{
		mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 1), fillBytes(macLength, 1)),
		mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 2), fillBytes(macLength, 2)),
		mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 3), fillBytes(macLength, 3)),
	}
	if _, err := hs.updateHash(mutations, func(_ []byte, maxIndex int) ([]byte, error) {
		seen = append(seen, maxIndex)
		return nil, nil
	}); err != nil {
		t.Fatalf("updateHash: %v", err)
	}
	for i, got := range seen {
		if got != i {
			t.Errorf("maxIndex da mutacao #%d = %d, esperado %d", i, got, i)
		}
	}
}

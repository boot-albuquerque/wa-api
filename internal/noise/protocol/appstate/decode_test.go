package appstate

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waServerSync"
)

// encodePatchForTest codifica patchInfo e devolve o SyncdPatch resultante ja'
// com o campo Version preenchido, como o servidor entregaria.
func encodePatchForTest(t *testing.T, proc *Processor, state HashState, patchInfo PatchInfo) *waServerSync.SyncdPatch {
	t.Helper()
	raw, err := proc.EncodePatch(context.Background(), testKeyID, state, patchInfo)
	if err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}
	patch := decodeEncodedPatch(t, raw)
	patch.Version = &waServerSync.SyncdVersion{Version: proto.Uint64(state.Version + 1)}
	return patch
}

func TestDecodePatchesRoundTripsEncodePatch(t *testing.T) {
	proc, mem := newTestProcessor(t)
	ctx := context.Background()

	info := BuildSettingPushName("Ciclano")
	patch := encodePatchForTest(t, proc, HashState{Version: 0}, info)

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	mutations, state, err := proc.DecodePatches(ctx, list, HashState{}, true, true)
	if err != nil {
		t.Fatalf("DecodePatches: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("len(mutations) = %d, esperado 1", len(mutations))
	}
	got := mutations[0]
	if got.Operation != waServerSync.SyncdMutation_SET {
		t.Errorf("operacao = %v, esperado SET", got.Operation)
	}
	if len(got.Index) != 1 || got.Index[0] != IndexSettingPushName {
		t.Errorf("index = %v, esperado [%s]", got.Index, IndexSettingPushName)
	}
	if got.Action.GetPushNameSetting().GetName() != "Ciclano" {
		t.Errorf("push name decodificado = %q", got.Action.GetPushNameSetting().GetName())
	}
	if got.Version != mutationVersionSettingPushName {
		t.Errorf("Version = %d, esperado %d", got.Version, mutationVersionSettingPushName)
	}
	if got.PatchVersion != 1 || state.Version != 1 {
		t.Errorf("PatchVersion=%d state.Version=%d, esperado 1/1", got.PatchVersion, state.Version)
	}

	// Os MACs do SET foram persistidos sob o nome da colecao.
	stored, err := mem.GetAppStateMutationMAC(ctx, string(WAPatchCriticalBlock), got.IndexMAC)
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if len(stored) != macLength {
		t.Errorf("value MAC persistido tem %d bytes, esperado %d", len(stored), macLength)
	}
	if mem.versions[string(WAPatchCriticalBlock)] != 1 {
		t.Errorf("versao persistida = %d, esperado 1", mem.versions[string(WAPatchCriticalBlock)])
	}
}

func TestDecodePatchesDetectsTamperedPatchMAC(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	patch.PatchMAC[0] ^= 0xFF

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true, true)
	if !errors.Is(err, ErrMismatchingPatchMAC) {
		t.Errorf("err = %v, esperado ErrMismatchingPatchMAC", err)
	}
}

func TestDecodePatchesDetectsTamperedSnapshotMAC(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	patch.SnapshotMAC[0] ^= 0xFF

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true, true)
	if !errors.Is(err, ErrMismatchingLTHash) {
		t.Errorf("err = %v, esperado ErrMismatchingLTHash", err)
	}
}

func TestDecodePatchesSkipsValidationWhenDisabled(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	patch.PatchMAC[0] ^= 0xFF
	patch.SnapshotMAC[0] ^= 0xFF

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	mutations, _, err := proc.DecodePatches(context.Background(), list, HashState{}, false, false)
	if err != nil {
		t.Fatalf("DecodePatches com validateMACs=false: %v", err)
	}
	if len(mutations) != 1 {
		t.Errorf("len(mutations) = %d, esperado 1", len(mutations))
	}
}

func TestDecodePatchesWithEmptyListIsNoop(t *testing.T) {
	proc, mem := newTestProcessor(t)
	initial := HashState{Version: 7}
	mutations, state, err := proc.DecodePatches(context.Background(), &PatchList{Name: WAPatchRegular}, initial, true, true)
	if err != nil {
		t.Fatalf("DecodePatches: %v", err)
	}
	if len(mutations) != 0 {
		t.Errorf("len(mutations) = %d, esperado 0", len(mutations))
	}
	if state != initial {
		t.Error("lista vazia nao deveria alterar o HashState")
	}
	if len(mem.versions) != 0 {
		t.Error("lista vazia nao deveria escrever no store")
	}
}

func TestDecodePatchesAppliesMultiplePatchesInOrder(t *testing.T) {
	proc, _ := newTestProcessor(t)

	first := encodePatchForTest(t, proc, HashState{Version: 0}, BuildSettingPushName("um"))
	// A segunda codificacao parte do estado que a primeira produziu.
	stateAfterFirst := HashState{Version: 1}
	if _, err := stateAfterFirst.updateHash(first.GetMutations(), func([]byte, int) ([]byte, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("updateHash: %v", err)
	}
	second := encodePatchForTest(t, proc, stateAfterFirst, BuildLabelEdit("1", "urgente", 3, false))

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{first, second}}
	mutations, state, err := proc.DecodePatches(context.Background(), list, HashState{}, false, false)
	if err != nil {
		t.Fatalf("DecodePatches: %v", err)
	}
	if len(mutations) != 2 {
		t.Fatalf("len(mutations) = %d, esperado 2", len(mutations))
	}
	if mutations[0].Index[0] != IndexSettingPushName || mutations[1].Index[0] != IndexLabelEdit {
		t.Errorf("ordem das mutacoes trocada: %v / %v", mutations[0].Index, mutations[1].Index)
	}
	if mutations[0].PatchVersion != 1 || mutations[1].PatchVersion != 2 {
		t.Errorf("PatchVersion = %d/%d, esperado 1/2", mutations[0].PatchVersion, mutations[1].PatchVersion)
	}
	if state.Version != 2 {
		t.Errorf("state.Version = %d, esperado 2", state.Version)
	}
}

func TestDecodePatchesPropagatesStoreWriteError(t *testing.T) {
	proc, mem := newTestProcessor(t)
	writeErr := errors.New("insert falhou")
	mem.putVersionErr = writeErr

	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, false, false)
	if !errors.Is(err, writeErr) {
		t.Errorf("err = %v, esperado %v", err, writeErr)
	}
}

func TestDecodePatchesDecodesSnapshot(t *testing.T) {
	proc, mem := newTestProcessor(t)
	ctx := context.Background()

	// Um snapshot e' uma lista de SyncdRecord tratados como SET. Reaproveitamos
	// os records produzidos por EncodePatch.
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("snap"))
	records := make([]*waServerSync.SyncdRecord, 0, len(patch.GetMutations()))
	for _, mut := range patch.GetMutations() {
		records = append(records, mut.GetRecord())
	}
	snapshot := &waServerSync.SyncdSnapshot{
		Version: &waServerSync.SyncdVersion{Version: proto.Uint64(5)},
		Records: records,
		KeyID:   &waServerSync.KeyId{ID: testKeyID},
	}

	list := &PatchList{Name: WAPatchCriticalBlock, Snapshot: snapshot}
	mutations, state, err := proc.DecodePatches(ctx, list, HashState{}, false, false)
	if err != nil {
		t.Fatalf("DecodePatches: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("len(mutations) = %d, esperado 1", len(mutations))
	}
	if mutations[0].Action.GetPushNameSetting().GetName() != "snap" {
		t.Errorf("push name = %q", mutations[0].Action.GetPushNameSetting().GetName())
	}
	if state.Version != 5 || mem.versions[string(WAPatchCriticalBlock)] != 5 {
		t.Errorf("versao = %d (store %d), esperado 5", state.Version, mem.versions[string(WAPatchCriticalBlock)])
	}
}

func TestDecodePatchesDetectsTamperedSnapshot(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("snap"))
	snapshot := &waServerSync.SyncdSnapshot{
		Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)},
		Records: []*waServerSync.SyncdRecord{patch.GetMutations()[0].GetRecord()},
		KeyID:   &waServerSync.KeyId{ID: testKeyID},
		Mac:     fillBytes(macLength, 0xAB),
	}

	list := &PatchList{Name: WAPatchCriticalBlock, Snapshot: snapshot}
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true, true)
	if !errors.Is(err, ErrMismatchingLTHash) {
		t.Errorf("err = %v, esperado ErrMismatchingLTHash", err)
	}
}

// --- F223: StrictAppStateSnapshotMAC toggle ---
//
// The snapshot MAC is an aggregate over the LTHash — it verifies that the SET
// of records is complete, not that individual records are authentic. Each
// mutation carries its own content MAC and index MAC, validated separately in
// decodeMutation. When StrictAppStateSnapshotMAC is false (the default), a
// snapshot MAC mismatch is logged and decoding continues; individual mutation
// MACs still gate every record.

// snapshotWithBadMAC builds a snapshot with valid records but an invalid
// aggregate MAC. The records are individually valid (produced by EncodePatch
// with the test key), so mutation-level MACs pass.
func snapshotWithBadMAC(t *testing.T, proc *Processor) (*PatchList, int) {
	t.Helper()
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("snap"))
	snapshot := &waServerSync.SyncdSnapshot{
		Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)},
		Records: []*waServerSync.SyncdRecord{patch.GetMutations()[0].GetRecord()},
		KeyID:   &waServerSync.KeyId{ID: testKeyID},
		Mac:     fillBytes(macLength, 0xAB),
	}
	return &PatchList{Name: WAPatchCriticalBlock, Snapshot: snapshot}, len(snapshot.Records)
}

func TestNonStrictSnapshotMACContinuesWithValidMutations(t *testing.T) {
	proc, _ := newTestProcessor(t)
	list, nRecords := snapshotWithBadMAC(t, proc)

	mutations, state, err := proc.DecodePatches(context.Background(), list, HashState{}, true, false)
	if err != nil {
		t.Fatalf("DecodePatches with non-strict snapshot MAC should succeed, got: %v", err)
	}
	if len(mutations) != nRecords {
		t.Fatalf("len(mutations) = %d, expected %d", len(mutations), nRecords)
	}
	if mutations[0].Action.GetPushNameSetting().GetName() != "snap" {
		t.Errorf("push name = %q, expected 'snap'", mutations[0].Action.GetPushNameSetting().GetName())
	}
	if state.Version != 1 {
		t.Errorf("state.Version = %d, expected 1", state.Version)
	}
}

func TestNonStrictSnapshotMACStillRejectsBadMutationMAC(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("snap"))
	record := patch.GetMutations()[0].GetRecord()
	record.GetValue().Blob[0] ^= 0xFF
	snapshot := &waServerSync.SyncdSnapshot{
		Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)},
		Records: []*waServerSync.SyncdRecord{record},
		KeyID:   &waServerSync.KeyId{ID: testKeyID},
		Mac:     fillBytes(macLength, 0xAB),
	}
	list := &PatchList{Name: WAPatchCriticalBlock, Snapshot: snapshot}

	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true, false)
	if err == nil {
		t.Fatal("expected error for tampered mutation content MAC, got nil")
	}
	if !errors.Is(err, ErrMismatchingContentMAC) {
		t.Errorf("err = %v, expected ErrMismatchingContentMAC", err)
	}
}

func TestStrictSnapshotMACAborts(t *testing.T) {
	proc, _ := newTestProcessor(t)
	list, _ := snapshotWithBadMAC(t, proc)

	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true, true)
	if !errors.Is(err, ErrMismatchingLTHash) {
		t.Errorf("strict mode should abort on snapshot MAC mismatch: err = %v, expected ErrMismatchingLTHash", err)
	}
}

// TestNonStrictPatchSnapshotMACContinues exercises the validatePatch path
// (incremental patches, not full snapshot) with a snapshot MAC mismatch in
// non-strict mode. The scenario mirrors F223: the patch is internally
// consistent (its SnapshotMAC and PatchMAC agree), but the caller's LTHash
// state diverges from what the patch expects. We simulate this by feeding a
// non-zero initial hash, which makes validateSnapshotMAC fail while the
// patch's own PatchMAC remains valid.
func TestNonStrictPatchSnapshotMACContinues(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{Version: 0}, BuildSettingPushName("incr"))

	// Divergent initial state: the hash is non-zero, so the LTHash after
	// applying the patch differs from what the patch was built against. This
	// makes validateSnapshotMAC fail while the patch MAC (which hashes the
	// server's SnapshotMAC, not our computed one) stays valid.
	var divergentHash [128]byte
	divergentHash[0] = 0xFF
	divergentInitial := HashState{Version: 0, Hash: divergentHash}

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	mutations, state, err := proc.DecodePatches(context.Background(), list, divergentInitial, true, false)
	if err != nil {
		t.Fatalf("non-strict patch snapshot MAC should not abort: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("len(mutations) = %d, expected 1", len(mutations))
	}
	if mutations[0].Action.GetPushNameSetting().GetName() != "incr" {
		t.Errorf("push name = %q", mutations[0].Action.GetPushNameSetting().GetName())
	}
	if state.Version != 1 {
		t.Errorf("state.Version = %d, expected 1", state.Version)
	}

	// Strict mode with the same input must abort.
	_, _, err = proc.DecodePatches(context.Background(), list, divergentInitial, true, true)
	if !errors.Is(err, ErrMismatchingLTHash) {
		t.Errorf("strict mode should abort: err = %v, expected ErrMismatchingLTHash", err)
	}
}

package appstate

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
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
	mutations, state, err := proc.DecodePatches(ctx, list, HashState{}, true)
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
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true)
	if !errors.Is(err, ErrMismatchingPatchMAC) {
		t.Errorf("err = %v, esperado ErrMismatchingPatchMAC", err)
	}
}

func TestDecodePatchesDetectsTamperedSnapshotMAC(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	patch.SnapshotMAC[0] ^= 0xFF

	list := &PatchList{Name: WAPatchCriticalBlock, Patches: []*waServerSync.SyncdPatch{patch}}
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true)
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
	mutations, _, err := proc.DecodePatches(context.Background(), list, HashState{}, false)
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
	mutations, state, err := proc.DecodePatches(context.Background(), &PatchList{Name: WAPatchRegular}, initial, true)
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
	mutations, state, err := proc.DecodePatches(context.Background(), list, HashState{}, false)
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
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, false)
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
	mutations, state, err := proc.DecodePatches(ctx, list, HashState{}, false)
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
	_, _, err := proc.DecodePatches(context.Background(), list, HashState{}, true)
	if !errors.Is(err, ErrMismatchingLTHash) {
		t.Errorf("err = %v, esperado ErrMismatchingLTHash", err)
	}
}

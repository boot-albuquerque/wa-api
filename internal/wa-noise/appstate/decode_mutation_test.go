package appstate

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/store"
)

func TestPatchOutputAddMAC(t *testing.T) {
	var out patchOutput
	out.AddMAC(fillBytes(macLength, 1), fillBytes(macLength, 2))
	out.AddMAC(fillBytes(macLength, 3), fillBytes(macLength, 4))
	if len(out.AddedMACs) != 2 {
		t.Fatalf("len(AddedMACs) = %d, esperado 2", len(out.AddedMACs))
	}
	if !bytes.Equal(out.AddedMACs[0].IndexMAC, fillBytes(macLength, 1)) {
		t.Error("IndexMAC do primeiro AddMAC nao foi preservado")
	}
	if !bytes.Equal(out.AddedMACs[1].ValueMAC, fillBytes(macLength, 4)) {
		t.Error("ValueMAC do segundo AddMAC nao foi preservado")
	}
}

func TestPatchOutputRemoveMACCancelsEarlierAdd(t *testing.T) {
	indexA := fillBytes(macLength, 1)
	indexB := fillBytes(macLength, 3)

	var out patchOutput
	out.AddMAC(indexA, fillBytes(macLength, 2))
	out.AddMAC(indexB, fillBytes(macLength, 4))
	out.RemoveMAC(indexA)

	if len(out.RemovedMACs) != 1 || !bytes.Equal(out.RemovedMACs[0], indexA) {
		t.Fatalf("RemovedMACs = %X, esperado [%X]", out.RemovedMACs, indexA)
	}
	if len(out.AddedMACs) != 1 {
		t.Fatalf("len(AddedMACs) = %d, esperado 1 (o add cancelado sai da lista)", len(out.AddedMACs))
	}
	if !bytes.Equal(out.AddedMACs[0].IndexMAC, indexB) {
		t.Error("RemoveMAC removeu a entrada errada")
	}
}

func TestPatchOutputRemoveMACWithoutMatchingAdd(t *testing.T) {
	var out patchOutput
	out.AddMAC(fillBytes(macLength, 1), fillBytes(macLength, 2))
	out.RemoveMAC(fillBytes(macLength, 9))
	if len(out.AddedMACs) != 1 {
		t.Errorf("len(AddedMACs) = %d, esperado 1 (nada a cancelar)", len(out.AddedMACs))
	}
	if len(out.RemovedMACs) != 1 {
		t.Errorf("len(RemovedMACs) = %d, esperado 1", len(out.RemovedMACs))
	}
}

func TestIndexMACToArray(t *testing.T) {
	full := fillBytes(macLength, 0x5A)
	arr := indexMACToArray(full)
	if !bytes.Equal(arr[:], full) {
		t.Errorf("indexMACToArray = %X, esperado %X", arr, full)
	}

	var zero [macLength]byte
	for _, bad := range [][]byte{nil, {}, fillBytes(macLength-1, 1), fillBytes(macLength+1, 1)} {
		if indexMACToArray(bad) != zero {
			t.Errorf("indexMACToArray(%d bytes) deveria devolver o array zerado", len(bad))
		}
	}
}

func TestDecodeMutationRoundTrip(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("Beltrano"))

	indexMAC, valueMAC, index, syncAction, keys, err := proc.decodeMutation(
		context.Background(), patch.GetMutations()[0], 0, true,
	)
	if err != nil {
		t.Fatalf("decodeMutation: %v", err)
	}
	if len(indexMAC) != macLength || len(valueMAC) != macLength {
		t.Errorf("MACs com tamanhos %d/%d, esperado %d", len(indexMAC), len(valueMAC), macLength)
	}
	if len(index) != 1 || index[0] != IndexSettingPushName {
		t.Errorf("index = %v", index)
	}
	if syncAction.GetValue().GetPushNameSetting().GetName() != "Beltrano" {
		t.Errorf("push name = %q", syncAction.GetValue().GetPushNameSetting().GetName())
	}
	if len(keys.Index) != appStateKeyPartLength {
		t.Error("decodeMutation deveria devolver as chaves expandidas")
	}
}

func TestDecodeMutationDetectsTamperedContent(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	mut := patch.GetMutations()[0]
	mut.Record.Value.Blob[0] ^= 0xFF

	_, _, _, _, _, err := proc.decodeMutation(context.Background(), mut, 0, true)
	if !errors.Is(err, ErrMismatchingContentMAC) {
		t.Errorf("err = %v, esperado ErrMismatchingContentMAC", err)
	}
}

func TestDecodeMutationDetectsTamperedIndexMAC(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	mut := patch.GetMutations()[0]
	mut.Record.Index.Blob[0] ^= 0xFF

	_, _, _, _, _, err := proc.decodeMutation(context.Background(), mut, 0, true)
	if !errors.Is(err, ErrMismatchingIndexMAC) {
		t.Errorf("err = %v, esperado ErrMismatchingIndexMAC", err)
	}
}

func TestDecodeMutationWithoutKeyFails(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	mut := patch.GetMutations()[0]
	mut.Record.KeyID = &waServerSync.KeyId{ID: []byte{0x77}}

	_, _, _, _, _, err := proc.decodeMutation(context.Background(), mut, 0, true)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, esperado ErrKeyNotFound", err)
	}
}

func TestDecodeMutationsRecordsSetAndRemove(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))

	setMut := patch.GetMutations()[0]
	removeMut := proto.Clone(setMut).(*waServerSync.SyncdMutation)
	removeMut.Operation = waServerSync.SyncdMutation_REMOVE.Enum()

	var out patchOutput
	// validateMACs=false: o content MAC do blob foi gerado para SET, entao
	// revalida-lo como REMOVE falharia por construcao.
	err := proc.decodeMutations(
		context.Background(),
		[]*waServerSync.SyncdMutation{setMut, removeMut},
		&out, false, 3, nil,
	)
	if err != nil {
		t.Fatalf("decodeMutations: %v", err)
	}
	if len(out.Mutations) != 2 {
		t.Fatalf("len(Mutations) = %d, esperado 2", len(out.Mutations))
	}
	if out.Mutations[0].PatchVersion != 3 || out.Mutations[1].PatchVersion != 3 {
		t.Error("PatchVersion nao foi propagada para as mutacoes")
	}
	if len(out.RemovedMACs) != 1 {
		t.Errorf("len(RemovedMACs) = %d, esperado 1", len(out.RemovedMACs))
	}
	// O SET e o REMOVE tem o mesmo index MAC, entao o add e' cancelado.
	if len(out.AddedMACs) != 0 {
		t.Errorf("len(AddedMACs) = %d, esperado 0 (REMOVE cancelou o SET)", len(out.AddedMACs))
	}
}

func TestDecodeMutationsRemovesFakeIndex(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := encodePatchForTest(t, proc, HashState{}, BuildSettingPushName("a"))
	removeMut := proto.Clone(patch.GetMutations()[0]).(*waServerSync.SyncdMutation)
	removeMut.Operation = waServerSync.SyncdMutation_REMOVE.Enum()

	realIndexMAC := removeMut.GetRecord().GetIndex().GetBlob()
	altIndexMAC := fillBytes(macLength, 0xEE)
	fakes := map[[macLength]byte][]byte{indexMACToArray(realIndexMAC): altIndexMAC}

	var out patchOutput
	err := proc.decodeMutations(
		context.Background(),
		[]*waServerSync.SyncdMutation{removeMut},
		&out, false, 1, fakes,
	)
	if err != nil {
		t.Fatalf("decodeMutations: %v", err)
	}
	if len(out.RemovedMACs) != 2 {
		t.Fatalf("len(RemovedMACs) = %d, esperado 2 (real + alternativo)", len(out.RemovedMACs))
	}
	if !bytes.Equal(out.RemovedMACs[1], altIndexMAC) {
		t.Errorf("segundo MAC removido = %X, esperado %X", out.RemovedMACs[1], altIndexMAC)
	}
}

func TestStoreMACsPersistsVersionAddsAndRemovals(t *testing.T) {
	proc, mem := newTestProcessor(t)
	ctx := context.Background()
	name := WAPatchRegularLow

	keep := fillBytes(macLength, 1)
	drop := fillBytes(macLength, 2)
	if err := mem.PutAppStateMutationMACs(ctx, string(name), 1, []store.AppStateMutationMAC{
		{IndexMAC: drop, ValueMAC: fillBytes(macLength, 9)},
	}); err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}

	state := HashState{Version: 12}
	out := &patchOutput{
		AddedMACs:   []store.AppStateMutationMAC{{IndexMAC: keep, ValueMAC: fillBytes(macLength, 3)}},
		RemovedMACs: [][]byte{drop},
	}
	if err := proc.storeMACs(ctx, name, state, out); err != nil {
		t.Fatalf("storeMACs: %v", err)
	}

	if mem.versions[string(name)] != 12 {
		t.Errorf("versao persistida = %d, esperado 12", mem.versions[string(name)])
	}
	got, err := mem.GetAppStateMutationMAC(ctx, string(name), keep)
	if err != nil || !bytes.Equal(got, fillBytes(macLength, 3)) {
		t.Errorf("MAC adicionado = %X (err %v)", got, err)
	}
	if got, _ = mem.GetAppStateMutationMAC(ctx, string(name), drop); got != nil {
		t.Errorf("MAC removido ainda presente: %X", got)
	}
}

func TestStoreMACsPropagatesEachStoreError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*memAppStateStore, error)
	}{
		{"PutAppStateVersion", func(m *memAppStateStore, e error) { m.putVersionErr = e }},
		{"DeleteAppStateMutationMACs", func(m *memAppStateStore, e error) { m.deleteMACsErr = e }},
		{"PutAppStateMutationMACs", func(m *memAppStateStore, e error) { m.putMACsErr = e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proc, mem := newTestProcessor(t)
			want := errors.New("falha em " + tc.name)
			tc.apply(mem, want)
			err := proc.storeMACs(context.Background(), WAPatchRegular, HashState{}, &patchOutput{})
			if !errors.Is(err, want) {
				t.Errorf("err = %v, esperado %v", err, want)
			}
		})
	}
}

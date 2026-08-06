package appstate

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/proto/waSyncAction"
	"wa-api/internal/wa-noise/proto/waSyncdSnapshotRecovery"
)

type recoveryResponse = waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_SyncDSnapshotFatalRecoveryResponse

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip.Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip.Close: %v", err)
	}
	return buf.Bytes()
}

func sampleRecovery(t *testing.T) *waSyncdSnapshotRecovery.SyncdSnapshotRecovery {
	t.Helper()
	index, err := json.Marshal([]string{IndexSettingPushName})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return &waSyncdSnapshotRecovery.SyncdSnapshotRecovery{
		CollectionName:   proto.String(string(WAPatchCriticalBlock)),
		CollectionLthash: fillBytes(lthashLength, 0x3C),
		Version:          &waSyncdSnapshotRecovery.SyncdVersion{Version: proto.Uint64(11)},
		MutationRecords: []*waSyncdSnapshotRecovery.SyncdPlainTextRecord{{
			KeyID: testKeyID,
			Mac:   fillBytes(macLength, 0x2B),
			Value: &waSyncAction.SyncActionData{
				Index:   index,
				Version: proto.Int32(mutationVersionSettingPushName),
				Value: &waSyncAction.SyncActionValue{
					PushNameSetting: &waSyncAction.PushNameSetting{Name: proto.String("Recuperado")},
				},
			},
		}},
	}
}

func TestParseRecoveryUncompressed(t *testing.T) {
	want := sampleRecovery(t)
	raw, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	got, err := ParseRecovery(&recoveryResponse{CollectionSnapshot: raw})
	if err != nil {
		t.Fatalf("ParseRecovery: %v", err)
	}
	if got.GetCollectionName() != want.GetCollectionName() {
		t.Errorf("CollectionName = %q", got.GetCollectionName())
	}
	if len(got.GetMutationRecords()) != 1 {
		t.Errorf("len(MutationRecords) = %d, esperado 1", len(got.GetMutationRecords()))
	}
}

func TestParseRecoveryCompressed(t *testing.T) {
	raw, err := proto.Marshal(sampleRecovery(t))
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	got, err := ParseRecovery(&recoveryResponse{
		CollectionSnapshot: gzipBytes(t, raw),
		IsCompressed:       proto.Bool(true),
	})
	if err != nil {
		t.Fatalf("ParseRecovery: %v", err)
	}
	if got.GetVersion().GetVersion() != 11 {
		t.Errorf("Version = %d, esperado 11", got.GetVersion().GetVersion())
	}
}

func TestParseRecoveryRejectsBadGzip(t *testing.T) {
	_, err := ParseRecovery(&recoveryResponse{
		CollectionSnapshot: []byte("nao e' gzip"),
		IsCompressed:       proto.Bool(true),
	})
	if err == nil {
		t.Fatal("gzip invalido deveria devolver erro")
	}
}

func TestParseRecoveryRejectsBadProto(t *testing.T) {
	if _, err := ParseRecovery(&recoveryResponse{CollectionSnapshot: []byte{0xFF, 0xFF, 0xFF}}); err == nil {
		t.Fatal("protobuf invalido deveria devolver erro")
	}
}

func TestProcessRecoveryStoresStateAndMutations(t *testing.T) {
	proc, mem := newTestProcessor(t)
	ctx := context.Background()
	recovery := sampleRecovery(t)

	// Estado anterior que a recuperacao precisa substituir.
	if err := mem.PutAppStateVersion(ctx, recovery.GetCollectionName(), 3, [lthashLength]byte{}); err != nil {
		t.Fatalf("PutAppStateVersion: %v", err)
	}

	mutations, err := proc.ProcessRecovery(ctx, recovery)
	if err != nil {
		t.Fatalf("ProcessRecovery: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("len(mutations) = %d, esperado 1", len(mutations))
	}
	got := mutations[0]
	if got.Operation != waServerSync.SyncdMutation_SET {
		t.Errorf("operacao = %v, esperado SET", got.Operation)
	}
	if len(got.Index) != 1 || got.Index[0] != IndexSettingPushName {
		t.Errorf("Index = %v", got.Index)
	}
	if got.Action.GetPushNameSetting().GetName() != "Recuperado" {
		t.Errorf("push name = %q", got.Action.GetPushNameSetting().GetName())
	}
	if got.PatchVersion != 11 {
		t.Errorf("PatchVersion = %d, esperado 11", got.PatchVersion)
	}

	// O index MAC e' recalculado a partir do indice, o value MAC vem do servidor.
	keys, err := proc.getAppStateKey(ctx, testKeyID)
	if err != nil {
		t.Fatalf("getAppStateKey: %v", err)
	}
	want := concatAndHMAC(sha256.New, keys.Index, recovery.MutationRecords[0].GetValue().GetIndex())
	if !bytes.Equal(got.IndexMAC, want) {
		t.Errorf("IndexMAC = %X, esperado %X", got.IndexMAC, want)
	}
	if !bytes.Equal(got.ValueMAC, recovery.MutationRecords[0].GetMac()) {
		t.Error("ValueMAC deveria vir do registro de recuperacao")
	}

	if mem.versions[recovery.GetCollectionName()] != 11 {
		t.Errorf("versao persistida = %d, esperado 11", mem.versions[recovery.GetCollectionName()])
	}
	if mem.hashes[recovery.GetCollectionName()] != [lthashLength]byte(recovery.GetCollectionLthash()) {
		t.Error("lthash persistido diverge do recebido")
	}
	stored, err := mem.GetAppStateMutationMAC(ctx, recovery.GetCollectionName(), got.IndexMAC)
	if err != nil || !bytes.Equal(stored, got.ValueMAC) {
		t.Errorf("MAC persistido = %X (err %v)", stored, err)
	}
}

func TestProcessRecoveryRejectsWrongLTHashLength(t *testing.T) {
	proc, _ := newTestProcessor(t)
	recovery := sampleRecovery(t)
	recovery.CollectionLthash = fillBytes(lthashLength-1, 0)
	if _, err := proc.ProcessRecovery(context.Background(), recovery); err == nil {
		t.Fatal("lthash com tamanho errado deveria devolver erro")
	}
}

func TestProcessRecoveryWithoutKeyFails(t *testing.T) {
	proc, _ := newTestProcessor(t)
	recovery := sampleRecovery(t)
	recovery.MutationRecords[0].KeyID = []byte{0x01}
	if _, err := proc.ProcessRecovery(context.Background(), recovery); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, esperado ErrKeyNotFound", err)
	}
}

func TestProcessRecoveryRejectsBadIndexJSON(t *testing.T) {
	proc, _ := newTestProcessor(t)
	recovery := sampleRecovery(t)
	recovery.MutationRecords[0].Value.Index = []byte("{nao e' um array}")
	if _, err := proc.ProcessRecovery(context.Background(), recovery); err == nil {
		t.Fatal("indice JSON invalido deveria devolver erro")
	}
}

func TestProcessRecoveryReturnsMutationsEvenOnStoreError(t *testing.T) {
	proc, mem := newTestProcessor(t)
	writeErr := errors.New("update falhou")
	mem.putVersionErr = writeErr

	mutations, err := proc.ProcessRecovery(context.Background(), sampleRecovery(t))
	if !errors.Is(err, writeErr) {
		t.Errorf("err = %v, esperado %v", err, writeErr)
	}
	// Contrato do upstream: as mutacoes decodificadas voltam mesmo com erro de
	// persistencia, para o chamador poder despachar os eventos.
	if len(mutations) != 1 {
		t.Errorf("len(mutations) = %d, esperado 1 mesmo com erro", len(mutations))
	}
}

package appstate

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/security/cbc"
)

func decodeEncodedPatch(t *testing.T, raw []byte) *waServerSync.SyncdPatch {
	t.Helper()
	var patch waServerSync.SyncdPatch
	if err := proto.Unmarshal(raw, &patch); err != nil {
		t.Fatalf("proto.Unmarshal do patch codificado: %v", err)
	}
	return &patch
}

func TestEncodePatchProducesVerifiableRecord(t *testing.T) {
	proc, _ := newTestProcessor(t)
	ctx := context.Background()
	keys, err := proc.getAppStateKey(ctx, testKeyID)
	if err != nil {
		t.Fatalf("getAppStateKey: %v", err)
	}

	info := BuildSettingPushName("Fulano")
	info.Timestamp = time.Unix(1700000000, 0)

	raw, err := proc.EncodePatch(ctx, testKeyID, HashState{Version: 4}, info)
	if err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}
	patch := decodeEncodedPatch(t, raw)

	if len(patch.GetMutations()) != 1 {
		t.Fatalf("len(Mutations) = %d, esperado 1", len(patch.GetMutations()))
	}
	mut := patch.GetMutations()[0]
	if mut.GetOperation() != waServerSync.SyncdMutation_SET {
		t.Errorf("operacao = %v, esperado SET", mut.GetOperation())
	}

	// O index MAC precisa bater com o HMAC do indice JSON.
	indexBytes, err := json.Marshal(info.Mutations[0].Index)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !hmac.Equal(mut.GetRecord().GetIndex().GetBlob(), concatAndHMAC(sha256.New, keys.Index, indexBytes)) {
		t.Error("index MAC nao bate com o HMAC do indice")
	}

	// O blob e' `IV || ciphertext || valueMAC`, e o valueMAC cobre o resto.
	blob := mut.GetRecord().GetValue().GetBlob()
	content, valueMAC := blob[:len(blob)-macLength], blob[len(blob)-macLength:]
	if len(content) <= cbcIVLength {
		t.Fatalf("conteudo cifrado tem %d bytes, menor que o IV (%d)", len(content), cbcIVLength)
	}
	if !hmac.Equal(valueMAC, generateContentMAC(waServerSync.SyncdMutation_SET, content, testKeyID, keys.ValueMAC)) {
		t.Error("value MAC nao bate com o content MAC do conteudo cifrado")
	}

	plaintext, err := cbcutil.Decrypt(keys.ValueEncryption, content[:cbcIVLength], content[cbcIVLength:])
	if err != nil {
		t.Fatalf("cbcutil.Decrypt: %v", err)
	}
	var action waSyncAction.SyncActionData
	if err = proto.Unmarshal(plaintext, &action); err != nil {
		t.Fatalf("proto.Unmarshal do SyncActionData: %v", err)
	}
	if action.GetVersion() != mutationVersionSettingPushName {
		t.Errorf("Version = %d, esperado %d", action.GetVersion(), mutationVersionSettingPushName)
	}
	if action.GetValue().GetPushNameSetting().GetName() != "Fulano" {
		t.Errorf("push name = %q", action.GetValue().GetPushNameSetting().GetName())
	}
	if action.GetValue().GetTimestamp() != info.Timestamp.UnixMilli() {
		t.Errorf("timestamp = %d, esperado %d", action.GetValue().GetTimestamp(), info.Timestamp.UnixMilli())
	}
}

func TestEncodePatchBumpsVersionForMACs(t *testing.T) {
	proc, _ := newTestProcessor(t)
	ctx := context.Background()
	keys, err := proc.getAppStateKey(ctx, testKeyID)
	if err != nil {
		t.Fatalf("getAppStateKey: %v", err)
	}

	initial := HashState{Version: 10}
	raw, err := proc.EncodePatch(ctx, testKeyID, initial, BuildSettingPushName("x"))
	if err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}
	patch := decodeEncodedPatch(t, raw)

	// EncodePatch recebe HashState por valor: a versao do chamador nao muda.
	if initial.Version != 10 {
		t.Errorf("HashState do chamador mutou para %d", initial.Version)
	}

	// O snapshot MAC e o patch MAC sao gerados sobre a versao ja' incrementada.
	expected := HashState{Version: initial.Version}
	if _, err = expected.updateHash(patch.GetMutations(), func([]byte, int) ([]byte, error) { return nil, nil }); err != nil {
		t.Fatalf("updateHash: %v", err)
	}
	expected.Version++
	if !hmac.Equal(patch.GetSnapshotMAC(), expected.generateSnapshotMAC(WAPatchCriticalBlock, keys.SnapshotMAC)) {
		t.Error("snapshot MAC nao corresponde a versao incrementada")
	}
	wantMAC, err := generatePatchMAC(patch, WAPatchCriticalBlock, keys.PatchMAC, expected.Version)
	if err != nil {
		t.Fatalf("generatePatchMAC: %v", err)
	}
	if !hmac.Equal(patch.GetPatchMAC(), wantMAC) {
		t.Error("patch MAC nao corresponde a versao incrementada")
	}
}

func TestEncodePatchFillsZeroTimestamp(t *testing.T) {
	proc, _ := newTestProcessor(t)
	before := time.Now().UnixMilli()
	info := BuildSettingPushName("y")
	if !info.Timestamp.IsZero() {
		t.Fatal("builder deveria deixar Timestamp zerado")
	}
	if _, err := proc.EncodePatch(context.Background(), testKeyID, HashState{}, info); err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}
	ts := info.Mutations[0].Value.GetTimestamp()
	if ts < before {
		t.Errorf("timestamp %d anterior ao inicio do teste %d", ts, before)
	}
}

func TestEncodePatchWithoutKeyFails(t *testing.T) {
	proc, _ := newTestProcessor(t)
	_, err := proc.EncodePatch(context.Background(), []byte{0x01}, HashState{}, BuildSettingPushName("z"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, esperado ErrKeyNotFound", err)
	}
}

func TestEncodePatchPropagatesStoreLookupError(t *testing.T) {
	proc, mem := newTestProcessor(t)
	lookupErr := errors.New("select falhou")
	mem.getMACErr = lookupErr
	_, err := proc.EncodePatch(context.Background(), testKeyID, HashState{}, BuildSettingPushName("w"))
	if !errors.Is(err, lookupErr) {
		t.Errorf("err = %v, esperado %v", err, lookupErr)
	}
}

func TestEncodePatchWithNoMutations(t *testing.T) {
	proc, _ := newTestProcessor(t)
	raw, err := proc.EncodePatch(context.Background(), testKeyID, HashState{}, PatchInfo{Type: WAPatchRegular})
	if err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}
	patch := decodeEncodedPatch(t, raw)
	if len(patch.GetMutations()) != 0 {
		t.Errorf("len(Mutations) = %d, esperado 0", len(patch.GetMutations()))
	}
	if len(patch.GetSnapshotMAC()) != macLength || len(patch.GetPatchMAC()) != macLength {
		t.Error("patch vazio ainda deveria carregar snapshot MAC e patch MAC")
	}
}

// EncodePatch escrevia em MutationInfo.Value sem checar nil — SIGSEGV no meio
// da codificacao (F49). Um patch sem valor e' erro do chamador, entao sai como
// erro em vez de virar um SyncActionValue vazio em silencio.
func TestEncodePatchRejeitaMutacaoSemValor(t *testing.T) {
	proc, _ := newTestProcessor(t)
	patch := PatchInfo{
		Type: WAPatchRegular,
		Mutations: []MutationInfo{
			{Index: []string{"ok"}, Value: &waSyncAction.SyncActionValue{}},
			{Index: []string{"sem valor"}},
		},
	}
	out, err := proc.EncodePatch(context.Background(), testKeyID, HashState{Version: 1}, patch)
	if !errors.Is(err, ErrNilMutationValue) {
		t.Fatalf("err = %v, esperava ErrNilMutationValue", err)
	}
	if out != nil {
		t.Errorf("out = %x, esperava nil", out)
	}
	if err != nil && !strings.Contains(err.Error(), "#2") {
		t.Errorf("a mensagem deveria apontar a mutacao #2: %v", err)
	}
}

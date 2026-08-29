package appstatesync

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/appstate"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waSyncdSnapshotRecovery"
	"wa-api/internal/noise/protocol/types/events"
)

type recoveryResult = waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult

// mustMarshalRemovePatch serializa o patch de uma mutacao REMOVE para uso como
// conteudo de um no `<patch>`.
func mustMarshalRemovePatch(t *testing.T) []byte {
	t.Helper()
	raw, err := proto.Marshal(removeOnlyPatchList().Patches[0])
	if err != nil {
		t.Fatalf("marshal do patch: %v", err)
	}
	return raw
}

// recoveryResponse embrulha um SyncdSnapshotRecovery no formato de resposta do
// dispositivo primario, opcionalmente comprimido.
func recoveryResponse(t *testing.T, rec *waSyncdSnapshotRecovery.SyncdSnapshotRecovery, compress bool) []*recoveryResult {
	t.Helper()
	raw, err := proto.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal do recovery: %v", err)
	}
	if compress {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(raw); err != nil {
			t.Fatalf("gzip: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		raw = buf.Bytes()
	}
	return []*recoveryResult{{
		SyncdSnapshotFatalRecoveryResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_SyncDSnapshotFatalRecoveryResponse{
			CollectionSnapshot: raw,
			IsCompressed:       proto.Bool(compress),
		},
	}}
}

// emptyRecovery monta um recovery valido e vazio: lthash do tamanho certo e
// nenhuma mutacao. E' o suficiente para atravessar ProcessRecovery inteiro.
func emptyRecovery(version uint64) *waSyncdSnapshotRecovery.SyncdSnapshotRecovery {
	return &waSyncdSnapshotRecovery.SyncdSnapshotRecovery{
		CollectionName:   proto.String(string(appstate.WAPatchRegular)),
		CollectionLthash: make([]byte, 128),
		Version:          &waSyncdSnapshotRecovery.SyncdVersion{Version: proto.Uint64(version)},
	}
}

func TestHandleRecoverySemDados(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	if !HandleRecovery(context.Background(), tp, "REQ1", nil) {
		t.Error("resposta vazia deveria devolver true (nada a reprocessar)")
	}
	if !HandleRecovery(context.Background(), tp, "REQ1", []*recoveryResult{{}}) {
		t.Error("resposta sem o campo de recovery deveria devolver true")
	}
}

// TestHandleRecoveryBlobInvalido: um blob que nao e' protobuf valido so' loga.
func TestHandleRecoveryBlobInvalido(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	res := []*recoveryResult{{
		SyncdSnapshotFatalRecoveryResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_SyncDSnapshotFatalRecoveryResponse{
			CollectionSnapshot: []byte{0xff, 0xff, 0xff, 0xff},
		},
	}}
	if !HandleRecovery(context.Background(), tp, "REQ1", res) {
		t.Error("blob invalido deveria devolver true")
	}
}

func TestHandleRecoveryErroAoLerVersaoAtual(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.getErr = errors.New("db travado")
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, emptyRecovery(5), false)) {
		t.Error("erro de store deveria devolver true")
	}
}

// TestHandleRecoveryVersaoAntiga: um recovery mais velho que o estado local e'
// ignorado.
func TestHandleRecoveryVersaoAntiga(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 10
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, emptyRecovery(5), false)) {
		t.Error("versao antiga deveria devolver true")
	}
	if len(tp.dispatched) != 0 {
		t.Errorf("nada deveria ter sido despachado: %#v", tp.dispatched)
	}
}

// TestHandleRecoveryProcessamentoFalha: lthash de tamanho errado faz
// ProcessRecovery falhar; o erro so' e' logado.
func TestHandleRecoveryProcessamentoFalha(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	rec := emptyRecovery(5)
	rec.CollectionLthash = []byte{1, 2, 3}
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, rec, false)) {
		t.Error("falha de processamento deveria devolver true")
	}
	if len(tp.dispatched) != 0 {
		t.Errorf("nada deveria ter sido despachado: %#v", tp.dispatched)
	}
}

// TestHandleRecoverySucesso cobre o caminho completo, incluindo o gzip e o
// AppStateSyncComplete com Recovery=true.
func TestHandleRecoverySucesso(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, emptyRecovery(5), true)) {
		t.Fatal("esperado true")
	}
	if len(tp.dispatched) != 1 {
		t.Fatalf("esperado 1 evento, veio %#v", tp.dispatched)
	}
	evt, ok := tp.dispatched[0].(*events.AppStateSyncComplete)
	if !ok {
		t.Fatalf("despachado %T, esperado *events.AppStateSyncComplete", tp.dispatched[0])
	}
	if !evt.Recovery || evt.Version != 5 || evt.Name != appstate.WAPatchRegular {
		t.Errorf("evento inesperado: %+v", evt)
	}
}

// TestHandleRecoveryHandlerFalhaDevolveFalse: e' o unico caso em que a funcao
// devolve false — o chamador usa isso para nao dar ack na mensagem.
func TestHandleRecoveryHandlerFalhaDevolveFalse(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.handlerFails = true
	if HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, emptyRecovery(5), false)) {
		t.Error("handler falhando deveria devolver false")
	}
}

// TestHandleRecoveryMuitosResultados: mais de um resultado so' gera aviso; o
// primeiro e' processado normalmente.
func TestHandleRecoveryMuitosResultados(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	res := append(recoveryResponse(t, emptyRecovery(5), false), &recoveryResult{})
	if !HandleRecovery(context.Background(), tp, "REQ1", res) {
		t.Fatal("esperado true")
	}
	if len(tp.dispatched) != 1 {
		t.Errorf("esperado 1 evento, veio %#v", tp.dispatched)
	}
}

// TestHandleRecoveryEmitEventsOnFullSync: com a flag ligada os eventos das
// mutacoes tambem entram na lista (aqui nao ha' mutacao, entao so' o Complete).
func TestHandleRecoveryEmitEventsOnFullSync(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.emitOnFullSync = true
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, emptyRecovery(5), false)) {
		t.Fatal("esperado true")
	}
	if len(tp.dispatched) != 1 {
		t.Errorf("esperado 1 evento, veio %#v", tp.dispatched)
	}
}

// TestHandleRecoveryGzipInvalido cobre a falha de descompressao.
func TestHandleRecoveryGzipInvalido(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	res := []*recoveryResult{{
		SyncdSnapshotFatalRecoveryResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_SyncDSnapshotFatalRecoveryResponse{
			CollectionSnapshot: []byte("isto nao e' gzip"),
			IsCompressed:       proto.Bool(true),
		},
	}}
	if !HandleRecovery(context.Background(), tp, "REQ1", res) {
		t.Error("gzip invalido deveria devolver true")
	}
}

// TestHandleRecoveryColetaFalha cobre o ramo em que CollectEvents falha: para
// critical_unblock_low, o recovery passa fullSync=true e a insercao em massa de
// contatos e' o unico erro que a coleta propaga.
func TestHandleRecoveryColetaFalha(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.Contacts = &fakeContactStore{allErr: errors.New("db fora do ar")}
	rec := emptyRecovery(5)
	rec.CollectionName = proto.String(string(appstate.WAPatchCriticalUnblockLow))
	if !HandleRecovery(context.Background(), tp, "REQ1", recoveryResponse(t, rec, false)) {
		t.Error("falha de coleta deveria devolver true")
	}
	if len(tp.dispatched) != 0 {
		t.Errorf("nada deveria ter sido despachado: %#v", tp.dispatched)
	}
}

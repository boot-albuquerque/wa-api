package appstatesync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/protocol/appstate"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waServerSync"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// syncTransport monta um Transport com os sub-stores de app state e um
// Processor real (o de internal/wa-noise/appstate) — patches vazios atravessam
// a decodificacao sem erro, o que basta para exercitar o laco de Fetch.
func syncTransport() (*fakeTransport, *fakeAppStateStore) {
	tp := newFakeTransport()
	as := &fakeAppStateStore{}
	tp.store.AppState = as
	tp.store.AppStateKeys = &fakeKeyStore{}
	tp.proc = appstate.NewProcessor(tp.store, sdklog.Noop)
	return tp, as
}

// collectionNode monta a resposta `<iq><sync><collection name=...>` que o
// servidor devolve. hasMore controla o atributo que faz o laco de Fetch pedir
// mais uma pagina.
func collectionNode(name string, hasMore bool) *waBinary.Node {
	attrs := waBinary.Attrs{"name": name}
	if hasMore {
		attrs["has_more_patches"] = "true"
	}
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     syncTag,
			Content: []waBinary.Node{{Tag: collectionTag, Attrs: attrs}},
		}},
	}
}

func TestFetchSincronizacaoIncremental(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 7
	tp.iq = collectionNode(string(appstate.WAPatchRegular), false)

	evts, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Sync incremental nao emite AppStateSyncComplete.
	if len(evts) != 0 {
		t.Errorf("esperado nenhum evento, veio %#v", evts)
	}
	if len(as.deleted) != 0 {
		t.Errorf("sync incremental nao deveria apagar a versao: %v", as.deleted)
	}
	if len(tp.sentIQs) != 1 {
		t.Fatalf("esperado 1 IQ, veio %d", len(tp.sentIQs))
	}
	iq := tp.sentIQs[0]
	if iq.Namespace != namespace || iq.Type != IQSet || iq.To != types.ServerJID {
		t.Errorf("IQ inesperado: %+v", iq)
	}
	// Sem snapshot, a versao de origem vai no atributo.
	collection := iq.Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if collection.Attrs[attrVersion] != uint64(7) {
		t.Errorf("attr version = %v, esperado 7", collection.Attrs[attrVersion])
	}
	if collection.Attrs[attrReturnSnapshot] != false {
		t.Errorf("return_snapshot = %v, esperado false", collection.Attrs[attrReturnSnapshot])
	}
}

func TestFetchFullSyncApagaVersaoEEmiteComplete(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 7
	tp.iq = collectionNode(string(appstate.WAPatchRegular), false)

	evts, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, true, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(as.deleted) != 1 || as.deleted[0] != string(appstate.WAPatchRegular) {
		t.Errorf("versao nao foi apagada: %v", as.deleted)
	}
	if len(evts) != 1 {
		t.Fatalf("esperado 1 evento, veio %#v", evts)
	}
	if _, ok := evts[0].(*events.AppStateSyncComplete); !ok {
		t.Errorf("evts[0] = %T, esperado *events.AppStateSyncComplete", evts[0])
	}
	// Em full sync o primeiro IQ pede snapshot e nao manda versao.
	collection := tp.sentIQs[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if collection.Attrs[attrReturnSnapshot] != true {
		t.Errorf("return_snapshot = %v, esperado true", collection.Attrs[attrReturnSnapshot])
	}
	if _, ok := collection.Attrs[attrVersion]; ok {
		t.Error("full sync nao deveria mandar o atributo version")
	}
}

// TestFetchVersaoZeraViraFullSync: versao 0 no store forca full sync mesmo com
// fullSync=false, e por isso onlyIfNotSynced nao curto-circuita.
func TestFetchVersaoZeraViraFullSync(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iq = collectionNode(string(appstate.WAPatchRegular), false)

	evts, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, true)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("esperado o AppStateSyncComplete do full sync, veio %#v", evts)
	}
}

func TestFetchOnlyIfNotSyncedCurtoCircuita(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 3
	evts, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, true)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if evts != nil {
		t.Errorf("esperado nil, veio %#v", evts)
	}
	if len(tp.sentIQs) != 0 {
		t.Error("nao deveria ter mandado IQ nenhum")
	}
}

// TestFetchEmitEventsOnFullSyncLigado: com a flag ligada, o full sync acumula
// os eventos das mutacoes tambem (aqui so' o Complete, ja' que nao ha'
// mutacao), o que se observa pelo ponteiro de acumulacao nao ser nil.
func TestFetchEmitEventsOnFullSyncLigado(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.emitOnFullSync = true
	tp.iq = collectionNode(string(appstate.WAPatchRegular), false)
	evts, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, true, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("esperado 1 evento, veio %#v", evts)
	}
}

// TestFetchPaginaEnquantoHasMorePatches: o laco repete enquanto o servidor
// disser que ha' mais, e a segunda pagina ja' nao pede snapshot.
func TestFetchPaginaEnquantoHasMorePatches(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iqSeq = []*waBinary.Node{
		collectionNode(string(appstate.WAPatchRegular), true),
		collectionNode(string(appstate.WAPatchRegular), false),
	}
	tp.iqErrs = []error{nil, nil}

	if _, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, true, false); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(tp.sentIQs) != 2 {
		t.Fatalf("esperado 2 IQs, veio %d", len(tp.sentIQs))
	}
	second := tp.sentIQs[1].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if second.Attrs[attrReturnSnapshot] != false {
		t.Errorf("a segunda pagina nao deveria pedir snapshot: %v", second.Attrs)
	}
}

func TestFetchErroAoApagarVersao(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.delErr = errors.New("db travado")
	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, true, false)
	if err == nil || !strings.Contains(err.Error(), "failed to reset app state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestFetchErroAoLerVersao(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.getErr = errors.New("db travado")
	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err == nil || !strings.Contains(err.Error(), "failed to get app state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestFetchErroNoIQ(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iqErr = errors.New("timeout")
	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err == nil || !strings.Contains(err.Error(), "failed to fetch app state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestFetchSnapshotInesperado trava as duas guardas contra o servidor devolver
// um snapshot que nao foi pedido.
func TestFetchSnapshotInesperado(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 5
	node := collectionNode(string(appstate.WAPatchRegular), false)
	sync := node.Content.([]waBinary.Node)[0]
	collection := sync.Content.([]waBinary.Node)[0]
	collection.Content = []waBinary.Node{{Tag: "snapshot", Content: []byte("nao importa")}}
	sync.Content = []waBinary.Node{collection}
	node.Content = []waBinary.Node{sync}
	tp.iq = node

	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err == nil {
		t.Fatal("esperado erro de snapshot inesperado")
	}
}

// --- FetchPatches ---

func TestFetchPatchesColecaoAusente(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iq = &waBinary.Node{Tag: "iq"}
	_, err := FetchPatches(context.Background(), tp, appstate.WAPatchRegular, 0, false)
	if err == nil {
		t.Fatal("esperado erro de elemento ausente")
	}
	if len(tp.elementCalls) != 1 || tp.elementCalls[0] != collectionTag+"/"+fetchErrContext {
		t.Errorf("ElementMissing chamado com %v", tp.elementCalls)
	}
}

func TestFetchPatchesErroDoIQ(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iqErr = errors.New("desconectado")
	if _, err := FetchPatches(context.Background(), tp, appstate.WAPatchRegular, 0, false); err == nil {
		t.Fatal("esperado o erro do IQ")
	}
}

// --- ApplyPatches ---

// TestApplyPatchesErroDeDecodificacaoEmiteEvento: um erro que NAO seja de chave
// ausente vira um events.AppStateSyncError despachado na hora.
func TestApplyPatchesErroDeDecodificacaoEmiteEvento(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	// A leitura do MAC anterior falha, entao updateHash falha — um erro que
	// nao e' ErrKeyNotFound.
	as.macErr = errors.New("db fora do ar")
	_, err := ApplyPatches(context.Background(), tp, appstate.WAPatchRegular, appstate.HashState{},
		removeOnlyPatchList(), false, nil)
	if err == nil {
		t.Fatal("esperado erro de decodificacao")
	}
	if len(tp.dispatched) != 1 {
		t.Fatalf("esperado 1 evento despachado, veio %#v", tp.dispatched)
	}
	if _, ok := tp.dispatched[0].(*events.AppStateSyncError); !ok {
		t.Errorf("despachado %T, esperado *events.AppStateSyncError", tp.dispatched[0])
	}
}

// removeOnlyPatchList monta um PatchList com uma unica mutacao REMOVE — a forma
// mais simples de chegar em updateHash sem precisar de blobs criptografados.
func removeOnlyPatchList() *appstate.PatchList {
	return &appstate.PatchList{
		Name: appstate.WAPatchRegular,
		Patches: []*waServerSync.SyncdPatch{{
			Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)},
			Mutations: []*waServerSync.SyncdMutation{{
				Operation: waServerSync.SyncdMutation_REMOVE.Enum(),
				Record: &waServerSync.SyncdRecord{
					Index: &waServerSync.SyncdIndex{Blob: []byte("idx")},
				},
			}},
		}},
	}
}

// TestApplyPatchesChaveAusentePedeAsChaves: quando a decodificacao falha por
// ErrKeyNotFound, nenhum AppStateSyncError e' emitido — o pedido de chaves e'
// disparado em background e o sync sera' refeito quando elas chegarem.
func TestApplyPatchesChaveAusentePedeAsChaves(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	_, err := ApplyPatches(context.Background(), tp, appstate.WAPatchRegular, appstate.HashState{},
		removeOnlyPatchList(), false, nil)
	if err == nil || !errors.Is(err, appstate.ErrKeyNotFound) {
		t.Fatalf("esperado ErrKeyNotFound, veio %v", err)
	}
	// A goroutine de pedido de chaves nao emite evento nenhum; o contrato aqui
	// e' justamente NAO despachar AppStateSyncError.
	for _, evt := range tp.dispatched {
		if _, ok := evt.(*events.AppStateSyncError); ok {
			t.Error("ErrKeyNotFound nao deveria virar AppStateSyncError")
		}
	}
}

// TestApplyPatchesListaVazia: sem patches, o estado sai igual ao que entrou e
// nada e' despachado.
func TestApplyPatchesListaVazia(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	in := appstate.HashState{Version: 9}
	out, err := ApplyPatches(context.Background(), tp, appstate.WAPatchRegular, in,
		&appstate.PatchList{Name: appstate.WAPatchRegular}, false, nil)
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if out.Version != 9 {
		t.Errorf("Version = %d, esperado 9", out.Version)
	}
	if len(tp.dispatched) != 0 {
		t.Errorf("nada deveria ter sido despachado: %#v", tp.dispatched)
	}
}

// snapshotCollectionNode monta uma resposta com um `<snapshot>` valido (uma
// referencia de blob externo que resolve para um SyncdSnapshot vazio).
func snapshotCollectionNode(t *testing.T, tp *fakeTransport) *waBinary.Node {
	t.Helper()
	ref, err := proto.Marshal(&waServerSync.ExternalBlobReference{})
	if err != nil {
		t.Fatalf("marshal da referencia: %v", err)
	}
	blob, err := proto.Marshal(&waServerSync.SyncdSnapshot{
		Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)},
	})
	if err != nil {
		t.Fatalf("marshal do snapshot: %v", err)
	}
	tp.blob = blob
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag: syncTag,
			Content: []waBinary.Node{{
				Tag:   collectionTag,
				Attrs: waBinary.Attrs{"name": string(appstate.WAPatchRegular)},
				Content: []waBinary.Node{{
					Tag:     "snapshot",
					Content: ref,
				}},
			}},
		}},
	}
}

// TestFetchSnapshotSemPedir trava a guarda contra o servidor devolver um
// snapshot em uma sincronizacao incremental.
func TestFetchSnapshotSemPedir(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 5
	tp.iq = snapshotCollectionNode(t, tp)
	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err == nil || !strings.Contains(err.Error(), "without asking") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestFetchErroAoAplicar cobre a propagacao do erro de ApplyPatches de dentro
// do laco de paginas.
func TestFetchErroAoAplicar(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 5
	as.macErr = errors.New("db fora do ar")
	node := collectionNode(string(appstate.WAPatchRegular), false)
	sync := node.Content.([]waBinary.Node)[0]
	collection := sync.Content.([]waBinary.Node)[0]
	collection.Content = []waBinary.Node{{
		Tag:     "patches",
		Content: []waBinary.Node{{Tag: "patch", Content: mustMarshalRemovePatch(t)}},
	}}
	sync.Content = []waBinary.Node{collection}
	node.Content = []waBinary.Node{sync}
	tp.iq = node

	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, false, false)
	if err == nil || !strings.Contains(err.Error(), "failed to decode app state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestFetchSnapshotComEstadoNaoVazio cobre a segunda guarda de snapshot: um
// snapshot so' pode ser aplicado sobre estado zerado. Em producao o full sync
// apaga a versao antes, entao chegar aqui exige um store que aceite o DELETE
// sem zerar — e' defesa contra store inconsistente, e o duble simula isso.
func TestFetchSnapshotComEstadoNaoVazio(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.version = 5
	as.keepOnDelete = true
	tp.iq = snapshotCollectionNode(t, tp)
	_, err := Fetch(context.Background(), tp, appstate.WAPatchRegular, true, false)
	if err == nil || !strings.Contains(err.Error(), "unexpected non-empty input state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

package appstate

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waServerSync"
)

func mustMarshal(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return raw
}

// collectionNode monta o no <collection> que carrega os patches. O atributo
// `name` e' obrigatorio para ParsePatchList (ag.String), entao vem por padrao.
func collectionNode(attrs waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
	if attrs == nil {
		attrs = waBinary.Attrs{}
	}
	if _, ok := attrs[patchListAttrName]; !ok {
		attrs[patchListAttrName] = string(WAPatchRegular)
	}
	return &waBinary.Node{Tag: "collection", Attrs: attrs, Content: children}
}

func patchNode(t *testing.T, patch *waServerSync.SyncdPatch) waBinary.Node {
	t.Helper()
	return waBinary.Node{Tag: patchNodeTag, Content: mustMarshal(t, patch)}
}

func failDownload(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
	return nil, errors.New("download nao deveria ter sido chamado")
}

func TestParsePatchListReadsAttrsAndPatches(t *testing.T) {
	first := &waServerSync.SyncdPatch{Version: &waServerSync.SyncdVersion{Version: proto.Uint64(1)}}
	second := &waServerSync.SyncdPatch{Version: &waServerSync.SyncdVersion{Version: proto.Uint64(2)}}

	node := collectionNode(
		waBinary.Attrs{patchListAttrName: string(WAPatchRegularHigh), patchListAttrHasMore: "true"},
		waBinary.Node{Tag: patchesNodeTag, Content: []waBinary.Node{patchNode(t, first), patchNode(t, second)}},
	)

	list, err := ParsePatchList(context.Background(), node, failDownload)
	if err != nil {
		t.Fatalf("ParsePatchList: %v", err)
	}
	if list.Name != WAPatchRegularHigh {
		t.Errorf("Name = %q, esperado %q", list.Name, WAPatchRegularHigh)
	}
	if !list.HasMorePatches {
		t.Error("HasMorePatches deveria ser true")
	}
	if list.Snapshot != nil {
		t.Error("Snapshot deveria ser nil sem no <snapshot>")
	}
	if len(list.Patches) != 2 {
		t.Fatalf("len(Patches) = %d, esperado 2", len(list.Patches))
	}
	if list.Patches[0].GetVersion().GetVersion() != 1 || list.Patches[1].GetVersion().GetVersion() != 2 {
		t.Error("patches fora de ordem ou mal decodificados")
	}
}

func TestParsePatchListRequiresNameAttr(t *testing.T) {
	node := &waBinary.Node{Tag: "collection", Attrs: waBinary.Attrs{}}
	if _, err := ParsePatchList(context.Background(), node, failDownload); err == nil {
		t.Fatal("collection sem o atributo `name` deveria devolver erro")
	}
}

func TestParsePatchListDefaultsWithoutOptionalAttrs(t *testing.T) {
	list, err := ParsePatchList(context.Background(), collectionNode(nil), failDownload)
	if err != nil {
		t.Fatalf("ParsePatchList: %v", err)
	}
	if list.Name != WAPatchRegular || list.HasMorePatches {
		t.Errorf("defaults inesperados: name=%q hasMore=%v", list.Name, list.HasMorePatches)
	}
	if len(list.Patches) != 0 {
		t.Errorf("len(Patches) = %d, esperado 0", len(list.Patches))
	}
}

func TestParsePatchListSkipsNonPatchChildren(t *testing.T) {
	node := collectionNode(nil, waBinary.Node{
		Tag: patchesNodeTag,
		Content: []waBinary.Node{
			{Tag: "outra_coisa", Content: []byte("lixo")},
			{Tag: patchNodeTag, Content: "conteudo que nao e' []byte"},
			patchNode(t, &waServerSync.SyncdPatch{Version: &waServerSync.SyncdVersion{Version: proto.Uint64(9)}}),
		},
	})

	list, err := ParsePatchList(context.Background(), node, failDownload)
	if err != nil {
		t.Fatalf("ParsePatchList: %v", err)
	}
	if len(list.Patches) != 1 || list.Patches[0].GetVersion().GetVersion() != 9 {
		t.Fatalf("esperado apenas o patch valido, veio %d patches", len(list.Patches))
	}
}

func TestParsePatchListRejectsMalformedPatch(t *testing.T) {
	node := collectionNode(nil, waBinary.Node{
		Tag:     patchesNodeTag,
		Content: []waBinary.Node{{Tag: patchNodeTag, Content: []byte{0xFF, 0xFF, 0xFF, 0xFF}}},
	})
	if _, err := ParsePatchList(context.Background(), node, failDownload); err == nil {
		t.Fatal("patch invalido deveria devolver erro")
	}
}

func TestParsePatchListDownloadsExternalMutations(t *testing.T) {
	external := &waServerSync.SyncdMutations{Mutations: []*waServerSync.SyncdMutation{
		mutationWithBlobs(waServerSync.SyncdMutation_SET, fillBytes(macLength, 1), fillBytes(macLength, 2)),
	}}
	patch := &waServerSync.SyncdPatch{ExternalMutations: &waServerSync.ExternalBlobReference{
		DirectPath: proto.String("/blob"),
	}}
	node := collectionNode(nil, waBinary.Node{Tag: patchesNodeTag, Content: []waBinary.Node{patchNode(t, patch)}})

	var calls int
	list, err := ParsePatchList(context.Background(), node, func(_ context.Context, ref *waServerSync.ExternalBlobReference) ([]byte, error) {
		calls++
		if ref.GetDirectPath() != "/blob" {
			t.Errorf("DirectPath = %q", ref.GetDirectPath())
		}
		return mustMarshal(t, external), nil
	})
	if err != nil {
		t.Fatalf("ParsePatchList: %v", err)
	}
	if calls != 1 {
		t.Errorf("downloadExternal chamado %d vezes, esperado 1", calls)
	}
	if len(list.Patches[0].GetMutations()) != 1 {
		t.Error("mutacoes externas nao foram anexadas ao patch")
	}
}

func TestParsePatchListRejectsEmptyExternalDownload(t *testing.T) {
	patch := &waServerSync.SyncdPatch{ExternalMutations: &waServerSync.ExternalBlobReference{}}
	node := collectionNode(nil, waBinary.Node{Tag: patchesNodeTag, Content: []waBinary.Node{patchNode(t, patch)}})

	_, err := ParsePatchList(context.Background(), node, func(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
		return mustMarshal(t, &waServerSync.SyncdMutations{}), nil
	})
	if err == nil {
		t.Fatal("download sem mutacoes deveria devolver erro")
	}
}

func TestParsePatchListPropagatesDownloadError(t *testing.T) {
	downloadErr := errors.New("rede caiu")
	patch := &waServerSync.SyncdPatch{ExternalMutations: &waServerSync.ExternalBlobReference{}}
	node := collectionNode(nil, waBinary.Node{Tag: patchesNodeTag, Content: []waBinary.Node{patchNode(t, patch)}})

	_, err := ParsePatchList(context.Background(), node, func(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
		return nil, downloadErr
	})
	if !errors.Is(err, downloadErr) {
		t.Errorf("err = %v, esperado %v", err, downloadErr)
	}
}

func TestParsePatchListDownloadsSnapshot(t *testing.T) {
	snapshot := &waServerSync.SyncdSnapshot{Version: &waServerSync.SyncdVersion{Version: proto.Uint64(42)}}
	ref := mustMarshal(t, &waServerSync.ExternalBlobReference{DirectPath: proto.String("/snap")})
	node := collectionNode(nil, waBinary.Node{Tag: snapshotNodeTag, Content: ref})

	list, err := ParsePatchList(context.Background(), node, func(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
		return mustMarshal(t, snapshot), nil
	})
	if err != nil {
		t.Fatalf("ParsePatchList: %v", err)
	}
	if list.Snapshot.GetVersion().GetVersion() != 42 {
		t.Errorf("versao do snapshot = %d, esperado 42", list.Snapshot.GetVersion().GetVersion())
	}
}

func TestParsePatchListPropagatesSnapshotDownloadError(t *testing.T) {
	downloadErr := errors.New("blob sumiu")
	ref := mustMarshal(t, &waServerSync.ExternalBlobReference{})
	node := collectionNode(nil, waBinary.Node{Tag: snapshotNodeTag, Content: ref})

	_, err := ParsePatchList(context.Background(), node, func(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
		return nil, downloadErr
	})
	if !errors.Is(err, downloadErr) {
		t.Errorf("err = %v, esperado %v", err, downloadErr)
	}
}

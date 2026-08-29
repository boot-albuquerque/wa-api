package user

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// Relocado de user_blocklist_test.go (Fase E lote 7), adaptado aos dubles.

func blocklistNode(dhash string, children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag:     "list",
		Attrs:   waBinary.Attrs{"dhash": dhash},
		Content: children,
	}
}

func TestParseBlocklistReadsDHashAndJIDs(t *testing.T) {
	node := blocklistNode("2:abc",
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPN2JID}},
	)
	got := ParseBlocklist(newFakeTransport(), &node)
	if got.DHash != "2:abc" {
		t.Errorf("got dhash %q, want %q", got.DHash, "2:abc")
	}
	if len(got.JIDs) != 2 || got.JIDs[0] != userTestPNJID || got.JIDs[1] != userTestPN2JID {
		t.Errorf("got JIDs %v", got.JIDs)
	}
}

// A tag do filho nao e' conferida — o que filtra e' o `jid` ser lido com
// sucesso. Travado para que o criterio real fique explicito.
func TestParseBlocklistSkipsChildrenWithoutValidJID(t *testing.T) {
	node := blocklistNode("",
		waBinary.Node{Tag: "item"}, // sem jid
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": "not-a-jid"}}, // jid de tipo errado
		waBinary.Node{Tag: "qualquer-tag", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)
	got := ParseBlocklist(newFakeTransport(), &node)
	if len(got.JIDs) != 1 || got.JIDs[0] != userTestPNJID {
		t.Errorf("got %v, want only %s", got.JIDs, userTestPNJID)
	}
}

func TestParseBlocklistEmptyNode(t *testing.T) {
	node := waBinary.Node{Tag: "list"}
	got := ParseBlocklist(newFakeTransport(), &node)
	if got == nil {
		t.Fatal("expected a non-nil blocklist")
	}
	if got.DHash != "" || len(got.JIDs) != 0 {
		t.Errorf("got %+v, want zero-valued blocklist", got)
	}
}

// O AttrGetter e' recriado por filho, entao um filho invalido nao contamina os
// seguintes. Sem isso, um unico item ruim mataria o resto da lista.
func TestParseBlocklistInvalidChildDoesNotPoisonLaterOnes(t *testing.T) {
	node := blocklistNode("",
		waBinary.Node{Tag: "item"},
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)
	got := ParseBlocklist(newFakeTransport(), &node)
	if len(got.JIDs) != 1 {
		t.Fatalf("got %v, want 1 JID", got.JIDs)
	}
}

// --- GetBlocklist ---

func blocklistResponse(children ...waBinary.Node) *waBinary.Node {
	list := blocklistNode("2:abc", children...)
	return &waBinary.Node{Tag: "iq", Content: []waBinary.Node{list}}
}

func TestGetBlocklistBuildsTheIQAndParses(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{blocklistResponse(
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)}

	got, err := GetBlocklist(t.Context(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DHash != "2:abc" || len(got.JIDs) != 1 {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != blocklistIQNamespace || iq.Type != IQGet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	if iq.Content != nil {
		t.Errorf("o <iq> de leitura nao tem conteudo: %v", iq.Content)
	}
}

func TestGetBlocklistPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetBlocklist(t.Context(), f); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestGetBlocklistMissingListIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetBlocklist(t.Context(), f)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "list" {
		t.Errorf("got %v", err)
	}
}

// --- UpdateBlocklist ---

func TestUpdateBlocklistBuildsTheIQAndParses(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{blocklistResponse(
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)}

	got, err := UpdateBlocklist(t.Context(), f, userTestLIDJID, userTestPNJID, events.BlocklistChangeActionBlock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.JIDs) != 1 {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != blocklistIQNamespace || iq.Type != IQSet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	item := iq.Content.([]waBinary.Node)[0]
	if item.Tag != "item" || item.Attrs["jid"] != userTestLIDJID ||
		item.Attrs["action"] != string(events.BlocklistChangeActionBlock) {
		t.Errorf("<item> = %+v", item)
	}
}

// TestUpdateBlocklistBlockCarriesPNJID trava a causa da F264/LIB-02: o
// WhatsApp migrou a escrita da blocklist para LID e exige, no block, um
// `pn_jid` adicional com o número de telefone (whatsmeow 8d023aa973,
// Baileys 8ca9316a10). Sem este atributo o servidor recusa com
// `400 bad-request` — medido em campo antes desta correção.
func TestUpdateBlocklistBlockCarriesPNJID(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{blocklistResponse()}

	_, err := UpdateBlocklist(t.Context(), f, userTestLIDJID, userTestPNJID, events.BlocklistChangeActionBlock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item := f.sent[0].Content.([]waBinary.Node)[0]
	if item.Attrs["jid"] != userTestLIDJID {
		t.Errorf("jid = %v, want LID %v", item.Attrs["jid"], userTestLIDJID)
	}
	if item.Attrs["pn_jid"] != userTestPNJID {
		t.Errorf("pn_jid = %v, want %v", item.Attrs["pn_jid"], userTestPNJID)
	}
}

// TestUpdateBlocklistUnblockOmitsPNJID trava a outra metade da regra: o
// stanza de unblock NÃO leva `pn_jid`, mesmo quando o chamador tem um PN
// para oferecer — enviá-lo lá seria divergir da forma que as três
// referências (whatsmeow, Baileys, wwebjs) concordam.
func TestUpdateBlocklistUnblockOmitsPNJID(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{blocklistResponse()}

	_, err := UpdateBlocklist(t.Context(), f, userTestLIDJID, userTestPNJID, events.BlocklistChangeActionUnblock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item := f.sent[0].Content.([]waBinary.Node)[0]
	if _, has := item.Attrs["pn_jid"]; has {
		t.Errorf("<item> = %+v, unblock nao deveria levar pn_jid", item)
	}
}

// TestUpdateBlocklistBlockWithZeroPNJIDOmitsAttribute trava o caso sem
// mapeamento em cache (LID sem PN conhecido): a biblioteca nao inventa um
// pn_jid vazio, so' omite o atributo — pior que a forma completa, nunca
// pior que o comportamento anterior a esta correcao.
func TestUpdateBlocklistBlockWithZeroPNJIDOmitsAttribute(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{blocklistResponse()}

	_, err := UpdateBlocklist(t.Context(), f, userTestLIDJID, types.JID{}, events.BlocklistChangeActionBlock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item := f.sent[0].Content.([]waBinary.Node)[0]
	if _, has := item.Attrs["pn_jid"]; has {
		t.Errorf("<item> = %+v, pn_jid zero deveria ser omitido", item)
	}
}

func TestUpdateBlocklistPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	_, err := UpdateBlocklist(t.Context(), f, userTestPNJID, types.JID{}, events.BlocklistChangeActionUnblock)
	if !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestUpdateBlocklistMissingListIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := UpdateBlocklist(t.Context(), f, userTestPNJID, types.JID{}, events.BlocklistChangeActionBlock)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.In != "response to blocklist update" {
		t.Errorf("got %v", err)
	}
}

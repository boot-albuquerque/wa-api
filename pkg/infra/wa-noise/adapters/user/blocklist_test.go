package user

import (
	"context"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// TestUserAdapter_GetBlocklist_NoSession.
func TestUserAdapter_GetBlocklist_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.GetBlocklist(context.Background(), "u1")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetBlocklist code = %q", testkit.AppErrCode(err))
	}
}

// TestUserAdapter_GetBlocklist_Nil devolve Blocklist vazio.
func TestUserAdapter_GetBlocklist_Nil(t *testing.T) {
	fake := &testkit.Fake{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) { return nil, nil }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetBlocklist(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetBlocklist = %v", err)
	}
	if len(got.JIDs) != 0 {
		t.Errorf("GetBlocklist = %+v, want empty", got)
	}
}

// TestUserAdapter_GetBlocklist_OK mapeia JIDs.
func TestUserAdapter_GetBlocklist_OK(t *testing.T) {
	fake := &testkit.Fake{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) {
		return &types.Blocklist{
			JIDs:  []types.JID{types.NewJID("5511", types.DefaultUserServer)},
			DHash: "abc",
		}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetBlocklist(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetBlocklist = %v", err)
	}
	if got.JIDs[0] != "5511@s.whatsapp.net" {
		t.Errorf("GetBlocklist[0] = %q", got.JIDs[0])
	}
	if got.DHash != "abc" {
		t.Errorf("GetBlocklist.DHash = %q", got.DHash)
	}
}

// TestUserAdapter_UpdateBlocklist_NoSession.
func TestUserAdapter_UpdateBlocklist_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "x@s.whatsapp.net", true)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("UpdateBlocklist code = %q", testkit.AppErrCode(err))
	}
}

// TestUserAdapter_UpdateBlocklist_BlockOK.
func TestUserAdapter_UpdateBlocklist_BlockOK(t *testing.T) {
	var seenAction events.BlocklistChangeAction
	fake := &testkit.Fake{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenAction = action
		return &types.Blocklist{JIDs: []types.JID{jid}, DHash: "h"}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.UpdateBlocklist(context.Background(), "u1", "5511@s.whatsapp.net", true)
	if err != nil {
		t.Fatalf("UpdateBlocklist = %v", err)
	}
	if seenAction != events.BlocklistChangeActionBlock {
		t.Errorf("action = %v, want block", seenAction)
	}
	if got.DHash != "h" {
		t.Errorf("UpdateBlocklist.DHash = %q", got.DHash)
	}
}

// TestUserAdapter_UpdateBlocklist_BlockPassesPNJID trava a causa da
// F264/LIB-02 na camada do adaptador: um pedido de block com telefone
// (o caso comum de quem chama a API) tem de entregar esse MESMO telefone
// como pnJID ao cliente — é o valor que vira `pn_jid` no stanza. Sem
// isto, mesmo com a biblioteca já corrigida, o adaptador nunca oferece o
// PN e o atributo sai sempre omitido.
func TestUserAdapter_UpdateBlocklist_BlockPassesPNJID(t *testing.T) {
	var seenPNJID types.JID
	fake := &testkit.Fake{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenPNJID = pnJID
		return &types.Blocklist{}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "5511@s.whatsapp.net", true)
	if err != nil {
		t.Fatalf("UpdateBlocklist = %v", err)
	}
	if seenPNJID.Server != types.DefaultUserServer || seenPNJID.User != "5511" {
		t.Errorf("pnJID = %v, queria o PN pedido (5511@s.whatsapp.net)", seenPNJID)
	}
}

// TestUserAdapter_UpdateBlocklist_UnblockDoesNotResolvePNJID trava que
// unblock não gasta um info query à toa: pnJID chega sempre zero ao
// cliente nesse caminho, porque a biblioteca nunca o usa fora de block.
func TestUserAdapter_UpdateBlocklist_UnblockDoesNotResolvePNJID(t *testing.T) {
	var seenPNJID types.JID
	fake := &testkit.Fake{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenPNJID = pnJID
		return &types.Blocklist{}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "5511@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("UpdateBlocklist unblock = %v", err)
	}
	if !seenPNJID.IsEmpty() {
		t.Errorf("pnJID = %v, queria zero (unblock não usa pn_jid)", seenPNJID)
	}
}

// TestUserAdapter_UpdateBlocklist_UnblockOK.
func TestUserAdapter_UpdateBlocklist_UnblockOK(t *testing.T) {
	var seenAction events.BlocklistChangeAction
	fake := &testkit.Fake{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenAction = action
		return &types.Blocklist{}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "5511@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("UpdateBlocklist unblock = %v", err)
	}
	if seenAction != events.BlocklistChangeActionUnblock {
		t.Errorf("action = %v, want unblock", seenAction)
	}
}

// TestToDomainBlocklist_Nil devolve Blocklist vazio.
func TestToDomainBlocklist_Nil(t *testing.T) {
	got := toDomainBlocklist(nil)
	if len(got.JIDs) != 0 {
		t.Errorf("toDomainBlocklist(nil).JIDs = %v, want empty", got.JIDs)
	}
}

// TestNormalizeBlocklistJID_LegacyUserServer converte legacy → default.
func TestNormalizeBlocklistJID_LegacyUserServer(t *testing.T) {
	jid := types.NewJID("5511", types.LegacyUserServer)
	got := normalizeBlocklistJID(jid)
	if got.Server != types.DefaultUserServer {
		t.Errorf("normalizeBlocklistJID.LegacyUserServer = %v, want DefaultUserServer", got.Server)
	}
}

// TestNormalizeBlocklistJID_DefaultPreserved.
func TestNormalizeBlocklistJID_DefaultPreserved(t *testing.T) {
	jid := types.NewJID("5511", types.DefaultUserServer)
	got := normalizeBlocklistJID(jid)
	if got.Server != types.DefaultUserServer {
		t.Errorf("normalizeBlocklistJID = %v, want DefaultUserServer", got.Server)
	}
}

// TestResolveBlocklistPNJID_DefaultUserServer_SemLIDEmCacheDevolveOPN: sem
// mapeamento PN→LID em cache, cai para o PN tal como veio — pior que a
// forma correta, mas nunca pior que o comportamento anterior a esta
// correção (que nem tentava resolver).
func TestResolveBlocklistPNJID_DefaultUserServer_SemLIDEmCacheDevolveOPN(t *testing.T) {
	jid := types.NewJID("5511", types.DefaultUserServer)
	got, err := resolveBlocklistPNJID(context.Background(), &testkit.Fake{}, jid)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID = %v", err)
	}
	if got.Server != types.DefaultUserServer || got.User != "5511" {
		t.Errorf("resolveBlocklistPNJID = %v, queria o PN inalterado", got)
	}
}

// TestResolveBlocklistPNJID_DefaultUserServer_ComLIDEmCacheResolveParaLID
// é o caso mais comum na prática: quem chama a API manda telefone, não
// LID. Sem esta resolução, `POST /users/unblock {"phone": "..."}` envia o
// `<item>` em `@s.whatsapp.net` — a forma que a F278/LIB-02 documentou
// como recusada pelo WhatsApp — mesmo depois da correção do sentido
// LID→PN, porque um PN nunca passava pelo ramo que essa correção mudou.
func TestResolveBlocklistPNJID_DefaultUserServer_ComLIDEmCacheResolveParaLID(t *testing.T) {
	pn := types.NewJID("5511", types.DefaultUserServer)
	lid := types.NewJID("999888", types.HiddenUserServer)
	fake := &testkit.Fake{StoreFn: func() *store.Device {
		return storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{lid: pn}}, nil)
	}}
	got, err := resolveBlocklistPNJID(context.Background(), fake, pn)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID = %v", err)
	}
	if got.Server != types.HiddenUserServer || got.User != "999888" {
		t.Errorf("resolveBlocklistPNJID = %v, queria o LID em cache %v", got, lid)
	}
}

// TestResolveBlocklistPNJID_HiddenUserServer_DevolveOMesmoLID trava a CAUSA
// da F278/LIB-02, não o sintoma: um LID que chega aqui já é a forma que o
// protocolo exige para escrever a blocklist, e não deve ser traduzido para
// PN. Antes desta correção, esta função chamava getCachedPNForLID e
// devolvia o PN — a forma pré-migração, que o WhatsApp recusa com `400
// bad-request` em UpdateBlocklist.
func TestResolveBlocklistPNJID_HiddenUserServer_DevolveOMesmoLID(t *testing.T) {
	jid := types.NewJID("123456", types.HiddenUserServer)
	// &testkit.Fake{} não implementa StoreFn: se resolveBlocklistPNJID ainda
	// chamasse getCachedPNForLID, o Store() nil faria este teste falhar por
	// erro, não silenciosamente — o que também prova que a chamada não
	// acontece mais.
	got, err := resolveBlocklistPNJID(context.Background(), &testkit.Fake{}, jid)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID(LID) = %v, queria nil (sem resolver PN)", err)
	}
	if got.Server != types.HiddenUserServer {
		t.Errorf("resolveBlocklistPNJID(LID).Server = %v, queria %v (permanecer @lid)", got.Server, types.HiddenUserServer)
	}
	if got.User != "123456" {
		t.Errorf("resolveBlocklistPNJID(LID).User = %q, queria %q (o mesmo LID)", got.User, "123456")
	}
}

// TestResolveBlocklistPNJID_UnsupportedServer devolve erro.
func TestResolveBlocklistPNJID_UnsupportedServer(t *testing.T) {
	jid := types.NewJID("5511", types.GroupServer)
	_, err := resolveBlocklistPNJID(context.Background(), &testkit.Fake{}, jid)
	if err == nil {
		t.Fatal("resolveBlocklistPNJID com servidor não suportado = nil")
	}
}

// TestGetCachedPNForLID_NilStore devolve erro.
func TestGetCachedPNForLID_NilStore(t *testing.T) {
	fake := &testkit.Fake{StoreFn: func() *store.Device { return nil }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com nil store = nil")
	}
}

// TestGetCachedPNForLID_NilLIDs devolve erro.
func TestGetCachedPNForLID_NilLIDs(t *testing.T) {
	fake := &testkit.Fake{StoreFn: func() *store.Device { return &store.Device{LIDs: nil, Contacts: nil} }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com LIDs nil = nil")
	}
}

// TestGetCachedPNForLID_NotMapped devolve erro.
func TestGetCachedPNForLID_NotMapped(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{}}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID sem mapeamento = nil")
	}
}

// TestGetCachedPNForLID_OK devolve PN mapeado.
func TestGetCachedPNForLID_OK(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{
		types.NewJID("lid", types.HiddenUserServer): types.NewJID("pn", types.DefaultUserServer),
	}}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	got, err := getCachedPNForLID(context.Background(), fake, types.NewJID("lid", types.HiddenUserServer))
	if err != nil {
		t.Fatalf("getCachedPNForLID = %v", err)
	}
	if got.User != "pn" {
		t.Errorf("getCachedPNForLID.User = %q", got.User)
	}
}

package whatsmeow

import (
	"context"
	"testing"

	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
	"wa-api/internal/waclient/types/events"
)

// TestUserAdapter_GetBlocklist_NoSession.
func TestUserAdapter_GetBlocklist_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetBlocklist(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetBlocklist code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetBlocklist_Nil devolve Blocklist vazio.
func TestUserAdapter_GetBlocklist_Nil(t *testing.T) {
	fake := &fakeWAClient{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) { return nil, nil }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	fake := &fakeWAClient{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) {
		return &types.Blocklist{
			JIDs:  []types.JID{types.NewJID("5511", types.DefaultUserServer)},
			DHash: "abc",
		}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	a := NewUserAdapter(getterWith(nil))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "x@s.whatsapp.net", true)
	if appErrCode(err) != "no_session" {
		t.Errorf("UpdateBlocklist code = %q", appErrCode(err))
	}
}

// TestUserAdapter_UpdateBlocklist_BlockOK.
func TestUserAdapter_UpdateBlocklist_BlockOK(t *testing.T) {
	var seenAction events.BlocklistChangeAction
	fake := &fakeWAClient{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenAction = action
		return &types.Blocklist{JIDs: []types.JID{jid}, DHash: "h"}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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

// TestUserAdapter_UpdateBlocklist_UnblockOK.
func TestUserAdapter_UpdateBlocklist_UnblockOK(t *testing.T) {
	var seenAction events.BlocklistChangeAction
	fake := &fakeWAClient{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		seenAction = action
		return &types.Blocklist{}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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

// TestResolveBlocklistPNJID_DefaultUserServer devolve como está.
func TestResolveBlocklistPNJID_DefaultUserServer(t *testing.T) {
	jid := types.NewJID("5511", types.DefaultUserServer)
	got, err := resolveBlocklistPNJID(context.Background(), &fakeWAClient{}, jid)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID = %v", err)
	}
	if got.User != "5511" {
		t.Errorf("resolveBlocklistPNJID.User = %q", got.User)
	}
}

// TestResolveBlocklistPNJID_UnsupportedServer devolve erro.
func TestResolveBlocklistPNJID_UnsupportedServer(t *testing.T) {
	jid := types.NewJID("5511", types.GroupServer)
	_, err := resolveBlocklistPNJID(context.Background(), &fakeWAClient{}, jid)
	if err == nil {
		t.Fatal("resolveBlocklistPNJID com servidor não suportado = nil")
	}
}

// TestGetCachedPNForLID_NilStore devolve erro.
func TestGetCachedPNForLID_NilStore(t *testing.T) {
	fake := &fakeWAClient{StoreFn: func() *store.Device { return nil }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com nil store = nil")
	}
}

// TestGetCachedPNForLID_NilLIDs devolve erro.
func TestGetCachedPNForLID_NilLIDs(t *testing.T) {
	fake := &fakeWAClient{StoreFn: func() *store.Device { return &store.Device{LIDs: nil, Contacts: nil} }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com LIDs nil = nil")
	}
}

// TestGetCachedPNForLID_NotMapped devolve erro.
func TestGetCachedPNForLID_NotMapped(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{}}, nil)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
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
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	got, err := getCachedPNForLID(context.Background(), fake, types.NewJID("lid", types.HiddenUserServer))
	if err != nil {
		t.Fatalf("getCachedPNForLID = %v", err)
	}
	if got.User != "pn" {
		t.Errorf("getCachedPNForLID.User = %q", got.User)
	}
}

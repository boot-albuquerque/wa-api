package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestNewUserAdapter(t *testing.T) {
	if NewUserAdapter(getterWith(nil)) == nil {
		t.Fatal("NewUserAdapter returned nil")
	}
}

// fakeLIDStore é um LIDStore mínimo com mapa pré-carregado.
type fakeLIDStore struct {
	mapping map[types.JID]types.JID
	errOnGet error
}

func (f *fakeLIDStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return nil
}
func (f *fakeLIDStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	if f.mapping == nil {
		f.mapping = map[types.JID]types.JID{}
	}
	f.mapping[lid] = jid
	return nil
}
func (f *fakeLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	if f.errOnGet != nil {
		return types.JID{}, f.errOnGet
	}
	if pn, ok := f.mapping[lid]; ok {
		return pn, nil
	}
	return types.JID{}, nil
}
func (f *fakeLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if f.errOnGet != nil {
		return types.JID{}, f.errOnGet
	}
	for lid, mapped := range f.mapping {
		if mapped == pn {
			return lid, nil
		}
	}
	return types.JID{}, nil
}
func (f *fakeLIDStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	out := map[types.JID]types.JID{}
	for _, pn := range pns {
		for lid, mapped := range f.mapping {
			if mapped == pn {
				out[lid] = pn
			}
		}
	}
	return out, nil
}

// fakeContactStore devolve um mapa pré-carregado de contatos.
type fakeContactStore struct {
	contacts map[types.JID]types.ContactInfo
	errOnGet error
}

func (f *fakeContactStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return true, "", nil
}
func (f *fakeContactStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return true, "", nil
}
func (f *fakeContactStore) PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error {
	return nil
}
func (f *fakeContactStore) PutAllContactNames(ctx context.Context, contacts []store.ContactEntry) error {
	return nil
}
func (f *fakeContactStore) PutManyRedactedPhones(ctx context.Context, entries []store.RedactedPhoneEntry) error {
	return nil
}
func (f *fakeContactStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	if c, ok := f.contacts[user]; ok {
		return c, nil
	}
	return types.ContactInfo{}, nil
}
func (f *fakeContactStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	if f.errOnGet != nil {
		return nil, f.errOnGet
	}
	return f.contacts, nil
}

// storeWith monta um *store.Device só com os campos que vamos usar.
func storeWith(lids store.LIDStore, contacts store.ContactStore) *store.Device {
	return &store.Device{LIDs: lids, Contacts: contacts}
}

// TestUserAdapter_IsOnWhatsApp_NoSession.
func TestUserAdapter_IsOnWhatsApp_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.IsOnWhatsApp(context.Background(), "u1", []string{"5511"})
	if appErrCode(err) != "no_session" {
		t.Errorf("IsOnWhatsApp code = %q", appErrCode(err))
	}
}

// TestUserAdapter_IsOnWhatsApp_OK mapeia resposta do SDK para domain.
func TestUserAdapter_IsOnWhatsApp_OK(t *testing.T) {
	fake := &fakeWAClient{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return []types.IsOnWhatsAppResponse{{
			Query: "5511",
			IsIn:  true,
			JID:   types.NewJID("5511", types.DefaultUserServer),
		}}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.IsOnWhatsApp(context.Background(), "u1", []string{"5511"})
	if err != nil {
		t.Fatalf("IsOnWhatsApp = %v", err)
	}
	if len(got) != 1 || !got[0].IsIn {
		t.Errorf("IsOnWhatsApp = %+v", got)
	}
	if got[0].JID != "5511@s.whatsapp.net" {
		t.Errorf("IsOnWhatsApp[0].JID = %q", got[0].JID)
	}
}

// TestUserAdapter_IsOnWhatsApp_PropagatesError.
func TestUserAdapter_IsOnWhatsApp_PropagatesError(t *testing.T) {
	sdkErr := errors.New("sdk fail")
	fake := &fakeWAClient{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.IsOnWhatsApp(context.Background(), "u1", nil)
	if err == nil {
		t.Fatal("IsOnWhatsApp não propagou erro")
	}
}

// TestUserAdapter_GetUserInfo_NoSession.
func TestUserAdapter_GetUserInfo_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetUserInfo(context.Background(), "u1", nil)
	if appErrCode(err) != "no_session" {
		t.Errorf("GetUserInfo code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetUserInfo_OK.
func TestUserAdapter_GetUserInfo_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{GetUserInfoFn: func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
		called = true
		return map[types.JID]types.UserInfo{}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"x@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("GetUserInfo = %v", err)
	}
	if !called {
		t.Fatal("GetUserInfo não invocou o SDK")
	}
}

// TestUserAdapter_GetAllContacts_NoSession.
func TestUserAdapter_GetAllContacts_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, _, err := a.GetAllContacts(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetAllContacts code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetAllContacts_OK.
func TestUserAdapter_GetAllContacts_OK(t *testing.T) {
	cs := &fakeContactStore{contacts: map[types.JID]types.ContactInfo{
		types.NewJID("5511", types.DefaultUserServer): {Found: true, PushName: "Alice"},
	}}
	fake := &fakeWAClient{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, count, err := a.GetAllContacts(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetAllContacts = %v", err)
	}
	if count != 1 {
		t.Errorf("GetAllContacts count = %d, want 1", count)
	}
	m, ok := got.(map[types.JID]types.ContactInfo)
	if !ok || len(m) != 1 {
		t.Errorf("GetAllContacts returned %+v", got)
	}
}

// TestUserAdapter_GetProfilePicture_NoSession.
func TestUserAdapter_GetProfilePicture_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetProfilePicture(context.Background(), "u1", "x@y.com", false)
	if appErrCode(err) != "no_session" {
		t.Errorf("GetProfilePicture code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetProfilePicture_NilPictureInfo devolve nil, nil.
func TestUserAdapter_GetProfilePicture_NilPictureInfo(t *testing.T) {
	fake := &fakeWAClient{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("GetProfilePicture = %v", err)
	}
	if got != nil {
		t.Errorf("GetProfilePicture = %v, want nil", got)
	}
}

// TestUserAdapter_GetProfilePicture_OK mapeia ID e URL.
func TestUserAdapter_GetProfilePicture_OK(t *testing.T) {
	fake := &fakeWAClient{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return &types.ProfilePictureInfo{ID: "abc", URL: "https://example/pic"}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("GetProfilePicture = %v", err)
	}
	if got.ID != "abc" || got.URL != "https://example/pic" {
		t.Errorf("GetProfilePicture = %+v", got)
	}
}

// TestUserAdapter_GetProfilePicture_PropagatesError.
func TestUserAdapter_GetProfilePicture_PropagatesError(t *testing.T) {
	sdkErr := errors.New("not found")
	fake := &fakeWAClient{GetProfilePictureInfoFn: func(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetProfilePicture(context.Background(), "u1", "x@s.whatsapp.net", false)
	if err == nil {
		t.Fatal("GetProfilePicture não propagou erro")
	}
}

// TestUserAdapter_GetLIDForPN_NoSession.
func TestUserAdapter_GetLIDForPN_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetLIDForPN code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID: o adapter
// não protege contra Store()==nil (caminho de produção nunca atingido
// porque client() garante não-nil antes); aqui validamos diretamente
// que getCachedPNForLID devolve erro em vez de panic.
func TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID(t *testing.T) {
	fake := &fakeWAClient{StoreFn: func() *store.Device { return nil }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com nil store = nil")
	}
}

// TestUserAdapter_GetLIDForPN_OK.
func TestUserAdapter_GetLIDForPN_OK(t *testing.T) {
	// mapping: lid → pn (a chave é o LID, o valor é o PN)
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{
		types.NewJID("lid-x", types.HiddenUserServer): types.NewJID("1234", types.DefaultUserServer),
	}}, nil)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetLIDForPN(context.Background(), "u1", "1234@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetLIDForPN = %v", err)
	}
	if got != "lid-x@lid" {
		t.Errorf("GetLIDForPN = %q", got)
	}
}

// TestUserAdapter_GetLIDForPN_NotMapped devolve "", nil.
func TestUserAdapter_GetLIDForPN_NotMapped(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{}}, nil)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetLIDForPN = %v", err)
	}
	if got != "" {
		t.Errorf("GetLIDForPN = %q, want empty", got)
	}
}

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

// TestUserAdapter_GetPrivacySettings_NoSession.
func TestUserAdapter_GetPrivacySettings_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetPrivacySettings(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetPrivacySettings code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetPrivacySettings_OK.
func TestUserAdapter_GetPrivacySettings_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{TryFetchPrivacySettingsFn: func(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) {
		called = true
		return &types.PrivacySettings{}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.GetPrivacySettings(context.Background(), "u1"); err != nil {
		t.Fatalf("GetPrivacySettings = %v", err)
	}
	if !called {
		t.Fatal("GetPrivacySettings não invocou o SDK")
	}
}

// TestUserAdapter_SetPrivacySetting_NoSession.
func TestUserAdapter_SetPrivacySetting_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.SetPrivacySetting(context.Background(), "u1", "last_seen", "everyone")
	if appErrCode(err) != "no_session" {
		t.Errorf("SetPrivacySetting code = %q", appErrCode(err))
	}
}

// TestUserAdapter_SetPrivacySetting_OK.
func TestUserAdapter_SetPrivacySetting_OK(t *testing.T) {
	called := false
	var seen types.PrivacySettingType
	fake := &fakeWAClient{SetPrivacySettingFn: func(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
		called = true
		seen = name
		return types.PrivacySettings{}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.SetPrivacySetting(context.Background(), "u1", "last_seen", "everyone"); err != nil {
		t.Fatalf("SetPrivacySetting = %v", err)
	}
	if !called {
		t.Fatal("SetPrivacySetting não invocou o SDK")
	}
	if seen != "last_seen" {
		t.Errorf("SetPrivacySetting name = %q", seen)
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
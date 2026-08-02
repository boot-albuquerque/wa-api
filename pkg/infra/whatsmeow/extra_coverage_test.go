package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// TestProfileDataAccess_NewProfileDataAccess cria adapter com client.
func TestProfileDataAccess_NewProfileDataAccess(t *testing.T) {
	da := NewProfileDataAccess(&whatsmeow.Client{})
	if da == nil {
		t.Fatal("NewProfileDataAccess returned nil")
	}
}

// TestProfileDataAccess_PushName_WithStore devolve Store.PushName.
func TestProfileDataAccess_PushName_WithStore(t *testing.T) {
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{PushName: "Alice"}}}
	if got := da.PushName(); got != "Alice" {
		t.Errorf("PushName = %q, want Alice", got)
	}
}

// TestProfileDataAccess_OwnJID_OK com Store.ID preenchido.
func TestProfileDataAccess_OwnJID_OK(t *testing.T) {
	myJID := types.NewJID("5511", types.DefaultUserServer)
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{ID: &myJID}}}
	got, ok := da.OwnJID()
	if !ok {
		t.Error("OwnJID returned !ok")
	}
	if got != "5511@s.whatsapp.net" {
		t.Errorf("OwnJID = %q, want 5511@s.whatsapp.net", got)
	}
}

// TestRealWAClient_Store devolve Client.Store.
func TestRealWAClient_Store(t *testing.T) {
	wac := &whatsmeow.Client{Store: &store.Device{}}
	rc := realWAClient{Client: wac}
	if got := rc.Store(); got != wac.Store {
		t.Errorf("realWAClient.Store = %v, want %v", got, wac.Store)
	}
}

// TestResolveBlocklistPNJID_HiddenUserServer_NoLIDs devolve erro.
func TestResolveBlocklistPNJID_HiddenUserServer_NoLIDs(t *testing.T) {
	jid := types.NewJID("5511", types.HiddenUserServer)
	_, err := resolveBlocklistPNJID(context.Background(), &fakeWAClient{}, jid)
	if err == nil {
		t.Fatal("resolveBlocklistPNJID HiddenUserServer sem store = nil")
	}
}

// TestResolveBlocklistPNJID_HiddenUserServer_NotFound devolve erro.
func TestResolveBlocklistPNJID_HiddenUserServer_NotFound(t *testing.T) {
	jid := types.NewJID("5511", types.HiddenUserServer)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{}}, nil) }}
	_, err := resolveBlocklistPNJID(context.Background(), fake, jid)
	if err == nil {
		t.Fatal("resolveBlocklistPNJID HiddenUserServer sem mapping = nil")
	}
}

// TestResolveBlocklistPNJID_HiddenUserServer_OK devolve PN mapeado.
func TestResolveBlocklistPNJID_HiddenUserServer_OK(t *testing.T) {
	jid := types.NewJID("5511", types.HiddenUserServer)
	fake := &fakeWAClient{StoreFn: func() *store.Device {
		return storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{
			jid: types.NewJID("1234", types.DefaultUserServer),
		}}, nil)
	}}
	got, err := resolveBlocklistPNJID(context.Background(), fake, jid)
	if err != nil {
		t.Fatalf("resolveBlocklistPNJID = %v", err)
	}
	if got.User != "1234" {
		t.Errorf("got.User = %q", got.User)
	}
}

// TestMiscAdapter_ProfileAccess_OK devolve ProfileDataAccess.
func TestMiscAdapter_ProfileAccess_OK(t *testing.T) {
	fake := &fakeWAClient{StoreFn: func() *store.Device { return &store.Device{} }}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	pa, err := a.ProfileAccess(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ProfileAccess = %v", err)
	}
	if pa == nil {
		t.Fatal("ProfileAccess returned nil")
	}
}

// TestMiscAdapter_RequestUnavailableMessage_PropagatesError.
func TestMiscAdapter_RequestUnavailableMessage_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{
		BuildUnavailableMessageFn: func(chat, sender types.JID, id string) *waE2E.Message { return nil },
		SendMessageFn: func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			return whatsmeow.SendResponse{}, sdkErr
		},
	}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.RequestUnavailableMessage(context.Background(), "u1", "x@y.com", "z@y.com", "m1")
	if err == nil {
		t.Fatal("RequestUnavailableMessage não propagou erro")
	}
}

// TestMiscAdapter_ListSubscribed_OK_WithItems.
func TestMiscAdapter_ListSubscribed_OK_WithItems(t *testing.T) {
	meta1 := &types.NewsletterMetadata{ID: types.NewJID("a", types.DefaultUserServer)}
	meta2 := &types.NewsletterMetadata{ID: types.NewJID("b", types.DefaultUserServer)}
	fake := &fakeWAClient{GetSubscribedNewslettersFn: func(ctx context.Context) ([]*types.NewsletterMetadata, error) {
		return []*types.NewsletterMetadata{meta1, meta2}, nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.ListSubscribed(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListSubscribed = %v", err)
	}
	if len(got.([]types.NewsletterMetadata)) != 2 {
		t.Errorf("ListSubscribed = %+v", got)
	}
}

// TestMiscAdapter_RejectCall_PropagatesError.
func TestMiscAdapter_RejectCall_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{RejectCallFn: func(ctx context.Context, callFrom types.JID, callID string) error { return sdkErr }}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.RejectCall(context.Background(), "u1", "x@y.com", "c1")
	if err == nil {
		t.Fatal("RejectCall não propagou erro")
	}
}

// TestGetCachedPNForLID_PropagatesError devolve erro.
func TestGetCachedPNForLID_PropagatesError(t *testing.T) {
	sdkErr := errors.New("mapping fail")
	dev := storeWith(&fakeLIDStore{errOnGet: sdkErr}, nil)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID não propagou erro")
	}
}

// TestUserAdapter_GetAllContacts_PropagatesError.
func TestUserAdapter_GetAllContacts_PropagatesError(t *testing.T) {
	cs := &fakeContactStore{errOnGet: errors.New("store fail")}
	fake := &fakeWAClient{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, _, err := a.GetAllContacts(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetAllContacts não propagou erro")
	}
}

// TestUserAdapter_GetBlocklist_PropagatesError.
func TestUserAdapter_GetBlocklist_PropagatesError(t *testing.T) {
	sdkErr := errors.New("blocklist fail")
	fake := &fakeWAClient{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) { return nil, sdkErr }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetBlocklist(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetBlocklist não propagou erro")
	}
}

// TestUserAdapter_GetPrivacySettings_PropagatesError.
func TestUserAdapter_GetPrivacySettings_PropagatesError(t *testing.T) {
	sdkErr := errors.New("privacy fail")
	fake := &fakeWAClient{TryFetchPrivacySettingsFn: func(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) { return nil, sdkErr }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetPrivacySettings(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetPrivacySettings não propagou erro")
	}
}

// TestUserAdapter_SetPrivacySetting_PropagatesError.
func TestUserAdapter_SetPrivacySetting_PropagatesError(t *testing.T) {
	sdkErr := errors.New("privacy fail")
	fake := &fakeWAClient{SetPrivacySettingFn: func(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
		return types.PrivacySettings{}, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.SetPrivacySetting(context.Background(), "u1", "last_seen", "everyone")
	if err == nil {
		t.Fatal("SetPrivacySetting não propagou erro")
	}
}

// TestUserAdapter_GetUserInfo_PropagatesError.
func TestUserAdapter_GetUserInfo_PropagatesError(t *testing.T) {
	sdkErr := errors.New("userinfo fail")
	fake := &fakeWAClient{GetUserInfoFn: func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"x@s.whatsapp.net"})
	if err == nil {
		t.Fatal("GetUserInfo não propagou erro")
	}
}

// TestUserAdapter_GetLIDForPN_PropagatesError.
func TestUserAdapter_GetLIDForPN_PropagatesError(t *testing.T) {
	sdkErr := errors.New("lid fail")
	dev := storeWith(&fakeLIDStore{errOnGet: sdkErr}, nil)
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if err == nil {
		t.Fatal("GetLIDForPN não propagou erro")
	}
}

// TestUserAdapter_UpdateBlocklist_PropagatesError.
func TestUserAdapter_UpdateBlocklist_PropagatesError(t *testing.T) {
	sdkErr := errors.New("block fail")
	fake := &fakeWAClient{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "x@s.whatsapp.net", true)
	if err == nil {
		t.Fatal("UpdateBlocklist não propagou erro")
	}
}

// imports usados indiretamente pelos fakes
var _ = errors.New
var _ domain.JID = ""
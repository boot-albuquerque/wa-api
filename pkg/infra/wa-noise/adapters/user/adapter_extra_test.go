package user

import (
	"context"
	"errors"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// TestGetCachedPNForLID_PropagatesError devolve erro.
func TestGetCachedPNForLID_PropagatesError(t *testing.T) {
	sdkErr := errors.New("mapping fail")
	dev := storeWith(&fakeLIDStore{errOnGet: sdkErr}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID não propagou erro")
	}
}

// TestUserAdapter_GetAllContacts_PropagatesError.
func TestUserAdapter_GetAllContacts_PropagatesError(t *testing.T) {
	cs := &testkit.ContactStore{ErrOnGet: errors.New("store fail")}
	fake := &testkit.Fake{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, _, err := a.GetAllContacts(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetAllContacts não propagou erro")
	}
}

// TestUserAdapter_GetBlocklist_PropagatesError.
func TestUserAdapter_GetBlocklist_PropagatesError(t *testing.T) {
	sdkErr := errors.New("blocklist fail")
	fake := &testkit.Fake{GetBlocklistFn: func(ctx context.Context) (*types.Blocklist, error) { return nil, sdkErr }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetBlocklist(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetBlocklist não propagou erro")
	}
}

// TestUserAdapter_GetPrivacySettings_PropagatesError.
func TestUserAdapter_GetPrivacySettings_PropagatesError(t *testing.T) {
	sdkErr := errors.New("privacy fail")
	fake := &testkit.Fake{TryFetchPrivacySettingsFn: func(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) { return nil, sdkErr }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetPrivacySettings(context.Background(), "u1")
	if err == nil {
		t.Fatal("GetPrivacySettings não propagou erro")
	}
}

// TestUserAdapter_SetPrivacySetting_PropagatesError.
func TestUserAdapter_SetPrivacySetting_PropagatesError(t *testing.T) {
	sdkErr := errors.New("privacy fail")
	fake := &testkit.Fake{SetPrivacySettingFn: func(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
		return types.PrivacySettings{}, sdkErr
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SetPrivacySetting(context.Background(), "u1", "last_seen", "everyone")
	if err == nil {
		t.Fatal("SetPrivacySetting não propagou erro")
	}
}

// TestUserAdapter_GetUserInfo_PropagatesError.
func TestUserAdapter_GetUserInfo_PropagatesError(t *testing.T) {
	sdkErr := errors.New("userinfo fail")
	fake := &testkit.Fake{GetUserInfoFn: func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"x@s.whatsapp.net"})
	if err == nil {
		t.Fatal("GetUserInfo não propagou erro")
	}
}

// TestUserAdapter_GetLIDForPN_PropagatesError.
func TestUserAdapter_GetLIDForPN_PropagatesError(t *testing.T) {
	sdkErr := errors.New("lid fail")
	dev := storeWith(&fakeLIDStore{errOnGet: sdkErr}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if err == nil {
		t.Fatal("GetLIDForPN não propagou erro")
	}
}

// TestUserAdapter_UpdateBlocklist_PropagatesError.
func TestUserAdapter_UpdateBlocklist_PropagatesError(t *testing.T) {
	sdkErr := errors.New("block fail")
	fake := &testkit.Fake{UpdateBlocklistFn: func(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.UpdateBlocklist(context.Background(), "u1", "x@s.whatsapp.net", true)
	if err == nil {
		t.Fatal("UpdateBlocklist não propagou erro")
	}
}

// imports usados indiretamente pelos fakes
var _ = errors.New

var _ domain.JID = ""

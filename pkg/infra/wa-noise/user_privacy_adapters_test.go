package whatsmeow

import (
	"context"
	"testing"

	"wa-api/internal/wa-noise/types"
)

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

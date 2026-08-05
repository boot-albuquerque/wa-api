package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestNewMiscAdapter(t *testing.T) {
	if NewMiscAdapter(getterWith(nil)) == nil {
		t.Fatal("NewMiscAdapter returned nil")
	}
}

// TestMiscAdapter_ArchiveChat_NoSession.
func TestMiscAdapter_ArchiveChat_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	err := a.ArchiveChat(context.Background(), "u1", "x@y.com", true)
	if appErrCode(err) != "no_session" {
		t.Errorf("ArchiveChat code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_ArchiveChat_OK.
func TestMiscAdapter_ArchiveChat_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SendAppStateFn: func(ctx context.Context, patch appstate.PatchInfo) error {
		called = true
		return nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.ArchiveChat(context.Background(), "u1", "x@y.com", true); err != nil {
		t.Fatalf("ArchiveChat = %v", err)
	}
	if !called {
		t.Fatal("ArchiveChat não invocou o SDK")
	}
}

// TestMiscAdapter_ArchiveChat_PropagatesError.
func TestMiscAdapter_ArchiveChat_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{SendAppStateFn: func(ctx context.Context, patch appstate.PatchInfo) error { return sdkErr }}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.ArchiveChat(context.Background(), "u1", "x@y.com", false)
	if err == nil {
		t.Fatal("ArchiveChat não propagou erro")
	}
}

// TestMiscAdapter_RejectCall_NoSession.
func TestMiscAdapter_RejectCall_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	err := a.RejectCall(context.Background(), "u1", "x@y.com", "call-1")
	if appErrCode(err) != "no_session" {
		t.Errorf("RejectCall code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_RejectCall_OK.
func TestMiscAdapter_RejectCall_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{RejectCallFn: func(ctx context.Context, callFrom types.JID, callID string) error {
		called = true
		if callID != "call-1" {
			t.Errorf("RejectCall callID = %q, want call-1", callID)
		}
		return nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.RejectCall(context.Background(), "u1", "x@y.com", "call-1"); err != nil {
		t.Fatalf("RejectCall = %v", err)
	}
	if !called {
		t.Fatal("RejectCall não invocou o SDK")
	}
}

// TestMiscAdapter_RequestUnavailableMessage_NoSession.
func TestMiscAdapter_RequestUnavailableMessage_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	_, err := a.RequestUnavailableMessage(context.Background(), "u1", "x@y.com", "z@y.com", "m1")
	if appErrCode(err) != "no_session" {
		t.Errorf("RequestUnavailableMessage code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_RequestUnavailableMessage_OK.
func TestMiscAdapter_RequestUnavailableMessage_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{
		BuildUnavailableMessageFn: func(chat, sender types.JID, id string) *waE2E.Message { return nil },
		SendMessageFn: func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
			called = true
			return whatsmeow.SendResponse{ID: "x", Timestamp: time.Now()}, nil
		},
	}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	res, err := a.RequestUnavailableMessage(context.Background(), "u1", "x@y.com", "z@y.com", "m1")
	if err != nil {
		t.Fatalf("RequestUnavailableMessage = %v", err)
	}
	if res.RequestID != "x" {
		t.Errorf("RequestID = %q, want x", res.RequestID)
	}
	if !called {
		t.Fatal("RequestUnavailableMessage não invocou SendMessage")
	}
}

// TestMiscAdapter_ProfileAccess_NoSession.
func TestMiscAdapter_ProfileAccess_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	_, err := a.ProfileAccess(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("ProfileAccess code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_ListSubscribed_NoSession.
func TestMiscAdapter_ListSubscribed_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	_, err := a.ListSubscribed(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("ListSubscribed code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_ListSubscribed_OK com newsletters nil.
func TestMiscAdapter_ListSubscribed_OK(t *testing.T) {
	fake := &fakeWAClient{GetSubscribedNewslettersFn: func(ctx context.Context) ([]*types.NewsletterMetadata, error) {
		return nil, nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.ListSubscribed(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListSubscribed = %v", err)
	}
	if len(got.([]types.NewsletterMetadata)) != 0 {
		t.Errorf("ListSubscribed = %+v, want empty", got)
	}
}

// TestMiscAdapter_ListSubscribed_FilterNil descarta entradas nil.
func TestMiscAdapter_ListSubscribed_FilterNil(t *testing.T) {
	meta := &types.NewsletterMetadata{ID: types.NewJID("n1", types.DefaultUserServer)}
	fake := &fakeWAClient{GetSubscribedNewslettersFn: func(ctx context.Context) ([]*types.NewsletterMetadata, error) {
		return []*types.NewsletterMetadata{nil, meta}, nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.ListSubscribed(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListSubscribed = %v", err)
	}
	list := got.([]types.NewsletterMetadata)
	if len(list) != 1 {
		t.Errorf("ListSubscribed length = %d, want 1", len(list))
	}
	if list[0].ID.User != "n1" {
		t.Errorf("ListSubscribed[0].ID.User = %q, want n1", list[0].ID.User)
	}
}

// TestMiscAdapter_SyncContactRoster_NoSession.
func TestMiscAdapter_SyncContactRoster_NoSession(t *testing.T) {
	a := NewMiscAdapter(getterWith(nil))
	err := a.SyncContactRoster(context.Background(), "u1", "incremental")
	if appErrCode(err) != "no_session" {
		t.Errorf("SyncContactRoster code = %q", appErrCode(err))
	}
}

// TestMiscAdapter_SyncContactRoster_ModeMapping cobre os tres modos aceitos
// e o par (fullSync, onlyIfNotSynced) que cada um deve produzir na chamada ao
// SDK — a diferenca entre eles e' exatamente o que esta capacidade existe
// para controlar.
func TestMiscAdapter_SyncContactRoster_ModeMapping(t *testing.T) {
	cases := []struct {
		mode                string
		wantFullSync        bool
		wantOnlyIfNotSynced bool
	}{
		{"if_unsynced", false, true},
		{"incremental", false, false},
		{"full", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			var gotName appstate.WAPatchName
			var gotFullSync, gotOnlyIfNotSynced bool
			called := false
			fake := &fakeWAClient{FetchAppStateFn: func(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
				called = true
				gotName = name
				gotFullSync = fullSync
				gotOnlyIfNotSynced = onlyIfNotSynced
				return nil
			}}
			a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
			if err := a.SyncContactRoster(context.Background(), "u1", tc.mode); err != nil {
				t.Fatalf("SyncContactRoster(%q) = %v", tc.mode, err)
			}
			if !called {
				t.Fatal("SyncContactRoster não invocou o SDK")
			}
			if gotName != appstate.WAPatchCriticalUnblockLow {
				t.Errorf("name = %q, want %q", gotName, appstate.WAPatchCriticalUnblockLow)
			}
			if gotFullSync != tc.wantFullSync {
				t.Errorf("fullSync = %v, want %v", gotFullSync, tc.wantFullSync)
			}
			if gotOnlyIfNotSynced != tc.wantOnlyIfNotSynced {
				t.Errorf("onlyIfNotSynced = %v, want %v", gotOnlyIfNotSynced, tc.wantOnlyIfNotSynced)
			}
		})
	}
}

// TestMiscAdapter_SyncContactRoster_InvalidMode devolve um erro tipado de
// validacao — nunca alcanca o SDK.
func TestMiscAdapter_SyncContactRoster_InvalidMode(t *testing.T) {
	called := false
	fake := &fakeWAClient{FetchAppStateFn: func(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
		called = true
		return nil
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.SyncContactRoster(context.Background(), "u1", "bogus")
	if appErrCode(err) != "invalid_sync_mode" {
		t.Errorf("SyncContactRoster code = %q, want invalid_sync_mode", appErrCode(err))
	}
	if called {
		t.Error("SyncContactRoster invocou o SDK com um modo invalido")
	}
}

// TestMiscAdapter_SyncContactRoster_PropagatesError.
func TestMiscAdapter_SyncContactRoster_PropagatesError(t *testing.T) {
	sdkErr := errors.New("fetch app state: boom")
	fake := &fakeWAClient{FetchAppStateFn: func(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
		return sdkErr
	}}
	a := NewMiscAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.SyncContactRoster(context.Background(), "u1", "incremental")
	if !errors.Is(err, sdkErr) {
		t.Fatalf("SyncContactRoster não propagou o erro do SDK: %v", err)
	}
}

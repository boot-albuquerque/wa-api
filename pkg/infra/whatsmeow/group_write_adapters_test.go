package whatsmeow

import (
	"context"
	"testing"
	"time"

	whatsmeow "wa-api/internal/waclient"
	"wa-api/internal/waclient/types"
)

// TestGroupAdapter_CreateGroup_OK.
func TestGroupAdapter_CreateGroup_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{CreateGroupFn: func(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error) {
		called = true
		if req.Name != "MyGroup" {
			t.Errorf("CreateGroup name = %q, want MyGroup", req.Name)
		}
		return &types.GroupInfo{}, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.CreateGroup(context.Background(), "u1", "MyGroup", nil); err != nil {
		t.Fatalf("CreateGroup = %v", err)
	}
	if !called {
		t.Fatal("CreateGroup não invocou o SDK")
	}
}

// TestGroupAdapter_CreateGroup_NoSession.
func TestGroupAdapter_CreateGroup_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.CreateGroup(context.Background(), "u1", "X", nil)
	if appErrCode(err) != "no_session" {
		t.Errorf("CreateGroup code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_JoinGroup_OK.
func TestGroupAdapter_JoinGroup_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{JoinGroupWithLinkFn: func(ctx context.Context, code string) (types.JID, error) {
		called = true
		return types.NewJID("joined", types.DefaultUserServer), nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.JoinGroup(context.Background(), "u1", "code"); err != nil {
		t.Fatalf("JoinGroup = %v", err)
	}
	if !called {
		t.Fatal("JoinGroup não invocou o SDK")
	}
}

// TestGroupAdapter_JoinGroup_NoSession.
func TestGroupAdapter_JoinGroup_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.JoinGroup(context.Background(), "u1", "code")
	if appErrCode(err) != "no_session" {
		t.Errorf("JoinGroup code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_LeaveGroup_NoSession.
func TestGroupAdapter_LeaveGroup_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.LeaveGroup(context.Background(), "u1", "g@g.us")
	if appErrCode(err) != "no_session" {
		t.Errorf("LeaveGroup code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_LeaveGroup_OK.
func TestGroupAdapter_LeaveGroup_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{LeaveGroupFn: func(ctx context.Context, jid types.JID) error {
		called = true
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.LeaveGroup(context.Background(), "u1", "g@g.us"); err != nil {
		t.Fatalf("LeaveGroup = %v", err)
	}
	if !called {
		t.Fatal("LeaveGroup não invocou o SDK")
	}
}

// TestGroupAdapter_SetGroupName_OK.
func TestGroupAdapter_SetGroupName_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetGroupNameFn: func(ctx context.Context, jid types.JID, name string) error {
		called = true
		if name != "NewName" {
			t.Errorf("SetGroupName name = %q", name)
		}
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupName(context.Background(), "u1", "g@g.us", "NewName"); err != nil {
		t.Fatalf("SetGroupName = %v", err)
	}
	if !called {
		t.Fatal("SetGroupName não invocou o SDK")
	}
}

// TestGroupAdapter_SetGroupName_NoSession.
func TestGroupAdapter_SetGroupName_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetGroupName(context.Background(), "u1", "g@g.us", "X")
	if appErrCode(err) != "no_session" {
		t.Errorf("SetGroupName code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetGroupTopic_OK.
func TestGroupAdapter_SetGroupTopic_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetGroupTopicFn: func(ctx context.Context, jid types.JID, prev, new, topic string) error {
		called = true
		if topic != "my new topic" {
			t.Errorf("SetGroupTopic topic = %q", topic)
		}
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupTopic(context.Background(), "u1", "g@g.us", "my new topic"); err != nil {
		t.Fatalf("SetGroupTopic = %v", err)
	}
	if !called {
		t.Fatal("SetGroupTopic não invocou o SDK")
	}
}

// TestGroupAdapter_SetGroupTopic_NoSession.
func TestGroupAdapter_SetGroupTopic_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetGroupTopic(context.Background(), "u1", "g@g.us", "topic")
	if appErrCode(err) != "no_session" {
		t.Errorf("SetGroupTopic code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetGroupPhoto_OK.
func TestGroupAdapter_SetGroupPhoto_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetGroupPhotoFn: func(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
		called = true
		return "photo-id-123", nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupPhoto(context.Background(), "u1", "g@g.us", []byte{0xFF}); err != nil {
		t.Fatalf("SetGroupPhoto = %v", err)
	}
	if !called {
		t.Fatal("SetGroupPhoto não invocou o SDK")
	}
}

// TestGroupAdapter_SetGroupPhoto_NilPhoto remove foto.
func TestGroupAdapter_SetGroupPhoto_NilPhoto(t *testing.T) {
	fake := &fakeWAClient{SetGroupPhotoFn: func(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
		if avatar != nil {
			t.Errorf("SetGroupPhoto avatar = %v, want nil", avatar)
		}
		return "", nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupPhoto(context.Background(), "u1", "g@g.us", nil); err != nil {
		t.Fatalf("SetGroupPhoto nil = %v", err)
	}
}

// TestGroupAdapter_SetGroupPhoto_NoSession.
func TestGroupAdapter_SetGroupPhoto_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetGroupPhoto(context.Background(), "u1", "g@g.us", nil)
	if appErrCode(err) != "no_session" {
		t.Errorf("SetGroupPhoto code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetGroupAnnounce_OK.
func TestGroupAdapter_SetGroupAnnounce_OK(t *testing.T) {
	called := false
	var seen bool
	fake := &fakeWAClient{SetGroupAnnounceFn: func(ctx context.Context, jid types.JID, announce bool) error {
		called = true
		seen = announce
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupAnnounce(context.Background(), "u1", "g@g.us", true); err != nil {
		t.Fatalf("SetGroupAnnounce = %v", err)
	}
	if !called {
		t.Fatal("SetGroupAnnounce não invocou o SDK")
	}
	if !seen {
		t.Error("SetGroupAnnounce não propagou o flag")
	}
}

// TestGroupAdapter_SetGroupAnnounce_NoSession.
func TestGroupAdapter_SetGroupAnnounce_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetGroupAnnounce(context.Background(), "u1", "g@g.us", false)
	if appErrCode(err) != "no_session" {
		t.Errorf("SetGroupAnnounce code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetGroupLocked_OK.
func TestGroupAdapter_SetGroupLocked_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetGroupLockedFn: func(ctx context.Context, jid types.JID, locked bool) error {
		called = true
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetGroupLocked(context.Background(), "u1", "g@g.us", true); err != nil {
		t.Fatalf("SetGroupLocked = %v", err)
	}
	if !called {
		t.Fatal("SetGroupLocked não invocou o SDK")
	}
}

// TestGroupAdapter_SetGroupLocked_NoSession.
func TestGroupAdapter_SetGroupLocked_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetGroupLocked(context.Background(), "u1", "g@g.us", false)
	if appErrCode(err) != "no_session" {
		t.Errorf("SetGroupLocked code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetDisappearingTimer_OK.
func TestGroupAdapter_SetDisappearingTimer_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetDisappearingTimerFn: func(ctx context.Context, chat types.JID, timer time.Duration, ts time.Time) error {
		called = true
		if timer != 24*time.Hour {
			t.Errorf("SetDisappearingTimer timer = %v", timer)
		}
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetDisappearingTimer(context.Background(), "u1", "g@g.us", 24*time.Hour, time.Now()); err != nil {
		t.Fatalf("SetDisappearingTimer = %v", err)
	}
	if !called {
		t.Fatal("SetDisappearingTimer não invocou o SDK")
	}
}

// TestGroupAdapter_SetDisappearingTimer_NoSession.
func TestGroupAdapter_SetDisappearingTimer_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetDisappearingTimer(context.Background(), "u1", "g@g.us", time.Hour, time.Now())
	if appErrCode(err) != "no_session" {
		t.Errorf("SetDisappearingTimer code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_SetJoinApprovalMode_OK.
func TestGroupAdapter_SetJoinApprovalMode_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{SetGroupJoinApprovalModeFn: func(ctx context.Context, jid types.JID, mode bool) error {
		called = true
		return nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SetJoinApprovalMode(context.Background(), "u1", "g@g.us", true); err != nil {
		t.Fatalf("SetJoinApprovalMode = %v", err)
	}
	if !called {
		t.Fatal("SetJoinApprovalMode não invocou o SDK")
	}
}

// TestGroupAdapter_SetJoinApprovalMode_NoSession.
func TestGroupAdapter_SetJoinApprovalMode_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.SetJoinApprovalMode(context.Background(), "u1", "g@g.us", false)
	if appErrCode(err) != "no_session" {
		t.Errorf("SetJoinApprovalMode code = %q", appErrCode(err))
	}
}

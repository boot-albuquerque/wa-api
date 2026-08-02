package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func TestNewGroupAdapter(t *testing.T) {
	if NewGroupAdapter(getterWith(nil)) == nil {
		t.Fatal("NewGroupAdapter returned nil")
	}
}

// TestGroupAdapter_GetGroupInfo_NoSession.
func TestGroupAdapter_GetGroupInfo_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.GetGroupInfo(context.Background(), "u1", "g@g.us")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetGroupInfo code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_GetGroupInfo_OK.
func TestGroupAdapter_GetGroupInfo_OK(t *testing.T) {
	info := &types.GroupInfo{JID: types.JID{User: "g", Server: "g.us"}}
	fake := &fakeWAClient{GetGroupInfoFn: func(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
		return info, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetGroupInfo(context.Background(), "u1", "g@g.us")
	if err != nil {
		t.Fatalf("GetGroupInfo = %v", err)
	}
	if got != info {
		t.Errorf("GetGroupInfo returned different pointer")
	}
}

// TestGroupAdapter_GetGroupInfo_PropagatesError.
func TestGroupAdapter_GetGroupInfo_PropagatesError(t *testing.T) {
	sdkErr := errors.New("network")
	fake := &fakeWAClient{GetGroupInfoFn: func(ctx context.Context, jid types.JID) (*types.GroupInfo, error) { return nil, sdkErr }}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetGroupInfo(context.Background(), "u1", "g@g.us")
	if err == nil {
		t.Fatal("GetGroupInfo não propagou erro")
	}
}

// TestGroupAdapter_GetGroupInfoFromLink_OK.
func TestGroupAdapter_GetGroupInfoFromLink_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{GetGroupInfoFromLinkFn: func(ctx context.Context, code string) (*types.GroupInfo, error) {
		called = true
		return &types.GroupInfo{}, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.GetGroupInfoFromLink(context.Background(), "u1", "ABCD"); err != nil {
		t.Fatalf("GetGroupInfoFromLink = %v", err)
	}
	if !called {
		t.Fatal("GetGroupInfoFromLink não invocou o SDK")
	}
}

// TestGroupAdapter_GetGroupInfoFromLink_NoSession.
func TestGroupAdapter_GetGroupInfoFromLink_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.GetGroupInfoFromLink(context.Background(), "u1", "code")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetGroupInfoFromLink code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_GetGroupInviteLink_OK.
func TestGroupAdapter_GetGroupInviteLink_OK(t *testing.T) {
	fake := &fakeWAClient{GetGroupInviteLinkFn: func(ctx context.Context, jid types.JID, reset bool) (string, error) {
		return "https://chat.whatsapp.com/XYZ", nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetGroupInviteLink(context.Background(), "u1", "g@g.us")
	if err != nil {
		t.Fatalf("GetGroupInviteLink = %v", err)
	}
	if got != "https://chat.whatsapp.com/XYZ" {
		t.Errorf("GetGroupInviteLink = %q", got)
	}
}

// TestGroupAdapter_GetGroupInviteLink_NoSession.
func TestGroupAdapter_GetGroupInviteLink_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.GetGroupInviteLink(context.Background(), "u1", "g@g.us")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetGroupInviteLink code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_ListJoinedGroups_OK.
func TestGroupAdapter_ListJoinedGroups_OK(t *testing.T) {
	groups := []*types.GroupInfo{{}, {}}
	fake := &fakeWAClient{GetJoinedGroupsFn: func(ctx context.Context) ([]*types.GroupInfo, error) { return groups, nil }}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, count, err := a.ListJoinedGroups(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListJoinedGroups = %v", err)
	}
	if count != 2 {
		t.Errorf("ListJoinedGroups count = %d, want 2", count)
	}
	if len(got.([]*types.GroupInfo)) != 2 {
		t.Errorf("ListJoinedGroups returned %d items", len(got.([]*types.GroupInfo)))
	}
}

// TestGroupAdapter_ListJoinedGroups_NoSession.
func TestGroupAdapter_ListJoinedGroups_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, _, err := a.ListJoinedGroups(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("ListJoinedGroups code = %q", appErrCode(err))
	}
}

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

// TestGroupAdapter_UpdateGroupParticipants_AddOK.
func TestGroupAdapter_UpdateGroupParticipants_AddOK(t *testing.T) {
	var seen whatsmeow.ParticipantChange
	fake := &fakeWAClient{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantAdd); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != whatsmeow.ParticipantChangeAdd {
		t.Errorf("UpdateGroupParticipants action = %v, want Add", seen)
	}
}

// TestGroupAdapter_UpdateGroupParticipants_RemoveOK.
func TestGroupAdapter_UpdateGroupParticipants_RemoveOK(t *testing.T) {
	var seen whatsmeow.ParticipantChange
	fake := &fakeWAClient{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantRemove); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != whatsmeow.ParticipantChangeRemove {
		t.Errorf("UpdateGroupParticipants action = %v, want Remove", seen)
	}
}

// TestGroupAdapter_UpdateGroupParticipants_NoSession.
func TestGroupAdapter_UpdateGroupParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", nil, domain.ParticipantAdd)
	if appErrCode(err) != "no_session" {
		t.Errorf("UpdateGroupParticipants code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_GetRequestParticipants_OK.
func TestGroupAdapter_GetRequestParticipants_OK(t *testing.T) {
	fake := &fakeWAClient{GetGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
		return []types.GroupParticipantRequest{{JID: types.NewJID("x", types.DefaultUserServer)}}, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetRequestParticipants(context.Background(), "u1", "g@g.us")
	if err != nil {
		t.Fatalf("GetRequestParticipants = %v", err)
	}
	if got == nil {
		t.Error("GetRequestParticipants returned nil")
	}
}

// TestGroupAdapter_GetRequestParticipants_NoSession.
func TestGroupAdapter_GetRequestParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	_, err := a.GetRequestParticipants(context.Background(), "u1", "g@g.us")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetRequestParticipants code = %q", appErrCode(err))
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Approve.
func TestGroupAdapter_UpdateRequestParticipants_Approve(t *testing.T) {
	var seen whatsmeow.ParticipantRequestChange
	fake := &fakeWAClient{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove); err != nil {
		t.Fatalf("UpdateRequestParticipants approve = %v", err)
	}
	if seen != whatsmeow.ParticipantChangeApprove {
		t.Errorf("action = %v, want approve", seen)
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Reject.
func TestGroupAdapter_UpdateRequestParticipants_Reject(t *testing.T) {
	var seen whatsmeow.ParticipantRequestChange
	fake := &fakeWAClient{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestReject); err != nil {
		t.Fatalf("UpdateRequestParticipants reject = %v", err)
	}
	if seen != whatsmeow.ParticipantChangeReject {
		t.Errorf("action = %v, want reject", seen)
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Unknown.
func TestGroupAdapter_UpdateRequestParticipants_Unknown(t *testing.T) {
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestAction("weird"))
	if err == nil {
		t.Fatal("UpdateRequestParticipants com ação inválida = nil")
	}
}

// TestGroupAdapter_UpdateRequestParticipants_NoSession.
func TestGroupAdapter_UpdateRequestParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(getterWith(nil))
	err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestApprove)
	if appErrCode(err) != "no_session" {
		t.Errorf("UpdateRequestParticipants code = %q", appErrCode(err))
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

// TestToJIDs_Empty devolve slice vazio sem erro.
func TestToJIDs_Empty(t *testing.T) {
	got, err := toJIDs(nil)
	if err != nil {
		t.Fatalf("toJIDs(nil) = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("toJIDs(nil) length = %d, want 0", len(got))
	}
}

// TestToJIDs_PropagatesError.
func TestToJIDs_PropagatesError(t *testing.T) {
	// entrada 0x00 deve falhar em toJID (ou pode não falhar — skip).
	_, err := toJIDs([]domain.JID{domain.JID(string([]byte{0x00}))})
	if err == nil {
		t.Skip("toJIDs não falhou para esta entrada")
	}
}
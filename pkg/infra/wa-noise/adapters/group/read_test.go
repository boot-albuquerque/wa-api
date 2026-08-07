package group

import (
	"context"
	"errors"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/internal/wa-noise/protocol/types"
)

// TestGroupAdapter_GetGroupInfo_NoSession.
func TestGroupAdapter_GetGroupInfo_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetGroupInfo(context.Background(), "u1", "g@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetGroupInfo code = %q", testkit.AppErrCode(err))
	}
}

// TestGroupAdapter_GetGroupInfo_OK.
func TestGroupAdapter_GetGroupInfo_OK(t *testing.T) {
	info := &types.GroupInfo{JID: types.JID{User: "g", Server: "g.us"}}
	fake := &testkit.Fake{GetGroupInfoFn: func(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
		return info, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
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
	fake := &testkit.Fake{GetGroupInfoFn: func(ctx context.Context, jid types.JID) (*types.GroupInfo, error) { return nil, sdkErr }}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetGroupInfo(context.Background(), "u1", "g@g.us")
	if err == nil {
		t.Fatal("GetGroupInfo não propagou erro")
	}
}

// TestGroupAdapter_GetGroupInfoFromLink_OK.
func TestGroupAdapter_GetGroupInfoFromLink_OK(t *testing.T) {
	called := false
	fake := &testkit.Fake{GetGroupInfoFromLinkFn: func(ctx context.Context, code string) (*types.GroupInfo, error) {
		called = true
		return &types.GroupInfo{}, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if _, err := a.GetGroupInfoFromLink(context.Background(), "u1", "ABCD"); err != nil {
		t.Fatalf("GetGroupInfoFromLink = %v", err)
	}
	if !called {
		t.Fatal("GetGroupInfoFromLink não invocou o SDK")
	}
}

// TestGroupAdapter_GetGroupInfoFromLink_NoSession.
func TestGroupAdapter_GetGroupInfoFromLink_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetGroupInfoFromLink(context.Background(), "u1", "code")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetGroupInfoFromLink code = %q", testkit.AppErrCode(err))
	}
}

// TestGroupAdapter_GetGroupInviteLink_OK.
func TestGroupAdapter_GetGroupInviteLink_OK(t *testing.T) {
	fake := &testkit.Fake{GetGroupInviteLinkFn: func(ctx context.Context, jid types.JID, reset bool) (string, error) {
		return "https://chat.whatsapp.com/XYZ", nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
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
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetGroupInviteLink(context.Background(), "u1", "g@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetGroupInviteLink code = %q", testkit.AppErrCode(err))
	}
}

// TestGroupAdapter_ListJoinedGroups_OK.
func TestGroupAdapter_ListJoinedGroups_OK(t *testing.T) {
	groups := []*types.GroupInfo{{}, {}}
	fake := &testkit.Fake{GetJoinedGroupsFn: func(ctx context.Context) ([]*types.GroupInfo, error) { return groups, nil }}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
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
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, _, err := a.ListJoinedGroups(context.Background(), "u1")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("ListJoinedGroups code = %q", testkit.AppErrCode(err))
	}
}

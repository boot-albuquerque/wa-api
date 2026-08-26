package group

import (
	"context"
	"errors"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/internal/wa-noise/protocol/types"
)

// --- GetSubGroups ---

func TestGroupAdapter_GetSubGroups_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetSubGroups(context.Background(), "u1", "community@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetSubGroups code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestGroupAdapter_GetSubGroups_OK(t *testing.T) {
	targets := []*types.GroupLinkTarget{
		{JID: types.JID{User: "sub1", Server: "g.us"}},
	}
	fake := &testkit.Fake{GetSubGroupsFn: func(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
		if community.User != "community" {
			t.Errorf("community.User = %q, want community", community.User)
		}
		return targets, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetSubGroups(context.Background(), "u1", "community@g.us")
	if err != nil {
		t.Fatalf("GetSubGroups = %v", err)
	}
	res, ok := got.([]*types.GroupLinkTarget)
	if !ok {
		t.Fatalf("GetSubGroups type = %T", got)
	}
	if len(res) != 1 || res[0].JID.User != "sub1" {
		t.Errorf("GetSubGroups returned unexpected result")
	}
}

func TestGroupAdapter_GetSubGroups_PropagatesError(t *testing.T) {
	sdkErr := errors.New("network")
	fake := &testkit.Fake{GetSubGroupsFn: func(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
		return nil, sdkErr
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetSubGroups(context.Background(), "u1", "community@g.us")
	if err == nil {
		t.Fatal("GetSubGroups should propagate error")
	}
}

// --- GetLinkedGroupsParticipants ---

func TestGroupAdapter_GetLinkedGroupsParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetLinkedGroupsParticipants(context.Background(), "u1", "community@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetLinkedGroupsParticipants code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestGroupAdapter_GetLinkedGroupsParticipants_OK(t *testing.T) {
	jids := []types.JID{{User: "user1", Server: "s.whatsapp.net"}}
	fake := &testkit.Fake{GetLinkedGroupsParticipantsFn: func(ctx context.Context, community types.JID) ([]types.JID, error) {
		return jids, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetLinkedGroupsParticipants(context.Background(), "u1", "community@g.us")
	if err != nil {
		t.Fatalf("GetLinkedGroupsParticipants = %v", err)
	}
	res, ok := got.([]types.JID)
	if !ok {
		t.Fatalf("GetLinkedGroupsParticipants type = %T", got)
	}
	if len(res) != 1 {
		t.Errorf("GetLinkedGroupsParticipants returned %d items, want 1", len(res))
	}
}

// --- LinkGroup ---

func TestGroupAdapter_LinkGroup_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	err := a.LinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("LinkGroup code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestGroupAdapter_LinkGroup_OK(t *testing.T) {
	called := false
	fake := &testkit.Fake{LinkGroupFn: func(ctx context.Context, parent, child types.JID) error {
		called = true
		if parent.User != "parent" {
			t.Errorf("parent.User = %q", parent.User)
		}
		if child.User != "child" {
			t.Errorf("child.User = %q", child.User)
		}
		return nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.LinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us"); err != nil {
		t.Fatalf("LinkGroup = %v", err)
	}
	if !called {
		t.Fatal("LinkGroup did not invoke the SDK")
	}
}

func TestGroupAdapter_LinkGroup_PropagatesError(t *testing.T) {
	sdkErr := errors.New("forbidden")
	fake := &testkit.Fake{LinkGroupFn: func(ctx context.Context, parent, child types.JID) error {
		return sdkErr
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.LinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us")
	if err == nil {
		t.Fatal("LinkGroup should propagate error")
	}
}

// --- UnlinkGroup ---

func TestGroupAdapter_UnlinkGroup_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	err := a.UnlinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("UnlinkGroup code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestGroupAdapter_UnlinkGroup_OK(t *testing.T) {
	called := false
	fake := &testkit.Fake{UnlinkGroupFn: func(ctx context.Context, parent, child types.JID) error {
		called = true
		return nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.UnlinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us"); err != nil {
		t.Fatalf("UnlinkGroup = %v", err)
	}
	if !called {
		t.Fatal("UnlinkGroup did not invoke the SDK")
	}
}

func TestGroupAdapter_UnlinkGroup_PropagatesError(t *testing.T) {
	sdkErr := errors.New("network")
	fake := &testkit.Fake{UnlinkGroupFn: func(ctx context.Context, parent, child types.JID) error {
		return sdkErr
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.UnlinkGroup(context.Background(), "u1", "parent@g.us", "child@g.us")
	if err == nil {
		t.Fatal("UnlinkGroup should propagate error")
	}
}

// --- InvalidJID ---

func TestGroupAdapter_GetSubGroups_InvalidJID(t *testing.T) {
	fake := &testkit.Fake{}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetSubGroups(context.Background(), "u1", "not-a-jid")
	if err == nil {
		t.Fatal("GetSubGroups should reject invalid JID")
	}
}

func TestGroupAdapter_LinkGroup_InvalidParentJID(t *testing.T) {
	fake := &testkit.Fake{}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.LinkGroup(context.Background(), "u1", "not-a-jid", "child@g.us")
	if err == nil {
		t.Fatal("LinkGroup should reject invalid parent JID")
	}
}

func TestGroupAdapter_LinkGroup_InvalidChildJID(t *testing.T) {
	fake := &testkit.Fake{}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.LinkGroup(context.Background(), "u1", "parent@g.us", "not-a-jid")
	if err == nil {
		t.Fatal("LinkGroup should reject invalid child JID")
	}
}

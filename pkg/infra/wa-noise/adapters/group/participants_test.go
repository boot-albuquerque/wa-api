package group

import (
	"context"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
)

// TestGroupAdapter_UpdateGroupParticipants_AddOK.
func TestGroupAdapter_UpdateGroupParticipants_AddOK(t *testing.T) {
	var seen wanoise.ParticipantChange
	fake := &testkit.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantAdd); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != wanoise.ParticipantChangeAdd {
		t.Errorf("UpdateGroupParticipants action = %v, want Add", seen)
	}
}

// TestGroupAdapter_UpdateGroupParticipants_RemoveOK.
func TestGroupAdapter_UpdateGroupParticipants_RemoveOK(t *testing.T) {
	var seen wanoise.ParticipantChange
	fake := &testkit.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantRemove); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != wanoise.ParticipantChangeRemove {
		t.Errorf("UpdateGroupParticipants action = %v, want Remove", seen)
	}
}

// TestGroupAdapter_UpdateGroupParticipants_NoSession.
func TestGroupAdapter_UpdateGroupParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", nil, domain.ParticipantAdd)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("UpdateGroupParticipants code = %q", testkit.AppErrCode(err))
	}
}

// TestGroupAdapter_GetRequestParticipants_OK.
func TestGroupAdapter_GetRequestParticipants_OK(t *testing.T) {
	fake := &testkit.Fake{GetGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
		return []types.GroupParticipantRequest{{JID: types.NewJID("x", types.DefaultUserServer)}}, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
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
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.GetRequestParticipants(context.Background(), "u1", "g@g.us")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetRequestParticipants code = %q", testkit.AppErrCode(err))
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Approve.
func TestGroupAdapter_UpdateRequestParticipants_Approve(t *testing.T) {
	var seen wanoise.ParticipantRequestChange
	fake := &testkit.Fake{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove); err != nil {
		t.Fatalf("UpdateRequestParticipants approve = %v", err)
	}
	if seen != wanoise.ParticipantChangeApprove {
		t.Errorf("action = %v, want approve", seen)
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Reject.
func TestGroupAdapter_UpdateRequestParticipants_Reject(t *testing.T) {
	var seen wanoise.ParticipantRequestChange
	fake := &testkit.Fake{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestReject); err != nil {
		t.Fatalf("UpdateRequestParticipants reject = %v", err)
	}
	if seen != wanoise.ParticipantChangeReject {
		t.Errorf("action = %v, want reject", seen)
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Unknown.
func TestGroupAdapter_UpdateRequestParticipants_Unknown(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestAction("weird"))
	if err == nil {
		t.Fatal("UpdateRequestParticipants com ação inválida = nil")
	}
}

// TestGroupAdapter_UpdateRequestParticipants_NoSession.
func TestGroupAdapter_UpdateRequestParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestApprove)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("UpdateRequestParticipants code = %q", testkit.AppErrCode(err))
	}
}

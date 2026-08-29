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

// TestGroupAdapter_UpdateGroupParticipants_PromoteOK and
// TestGroupAdapter_UpdateGroupParticipants_DemoteOK are the test of the F263
// defect: the adapter used to map every action that was not literally
// domain.ParticipantAdd to wa.ParticipantChangeRemove — so a promote/demote
// request that had already cleared the use case's switch (F263, part one)
// would STILL have gone out on the wire as a removal. This is what closes the
// gap the use-case-level test cannot see, since the mapping bug lived here.
func TestGroupAdapter_UpdateGroupParticipants_PromoteOK(t *testing.T) {
	var seen wanoise.ParticipantChange
	fake := &testkit.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantPromote); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != wanoise.ParticipantChangePromote {
		t.Errorf("UpdateGroupParticipants action = %v, want Promote (F263: this used to silently come out as Remove)", seen)
	}
}

func TestGroupAdapter_UpdateGroupParticipants_DemoteOK(t *testing.T) {
	var seen wanoise.ParticipantChange
	fake := &testkit.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
		seen = action
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if _, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantDemote); err != nil {
		t.Fatalf("UpdateGroupParticipants = %v", err)
	}
	if seen != wanoise.ParticipantChangeDemote {
		t.Errorf("UpdateGroupParticipants action = %v, want Demote (F263: this used to silently come out as Remove)", seen)
	}
}

// TestGroupAdapter_UpdateGroupParticipants_UnknownAction is the negative
// space of the switch: a value that is neither of the four must be refused
// here too, and not fall through to Remove — the exact silent-default bug
// this switch replaces.
func TestGroupAdapter_UpdateGroupParticipants_UnknownAction(t *testing.T) {
	fake := &testkit.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
		t.Fatalf("port reached with unknown action %v; the adapter must refuse before calling it", action)
		return nil, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"u2@s.whatsapp.net"}, domain.ParticipantAction("weird"))
	if err == nil {
		t.Fatal("UpdateGroupParticipants com ação desconhecida = nil")
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

// TestGroupAdapter_UpdateRequestParticipants_Approve trava a F280: o
// resultado por solicitante que o protocolo devolve (JID + Error) sai no
// ParticipantsUpdate, não é descartado.
func TestGroupAdapter_UpdateRequestParticipants_Approve(t *testing.T) {
	var seen wanoise.ParticipantRequestChange
	fake := &testkit.Fake{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error) {
		seen = action
		return []types.GroupParticipant{
			{JID: types.NewJID("aprovado", types.DefaultUserServer)},
			{JID: types.NewJID("recusado", types.DefaultUserServer), Error: 409},
		}, nil
	}}
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove)
	if err != nil {
		t.Fatalf("UpdateRequestParticipants approve = %v", err)
	}
	if seen != wanoise.ParticipantChangeApprove {
		t.Errorf("action = %v, want approve", seen)
	}
	if !got.Confirmed {
		t.Error("Confirmed = false, queria true: o transporte leu o resultado na mesma chamada")
	}
	if len(got.Participants) != 2 {
		t.Fatalf("Participants = %d, queria 2 (um sucesso, um parcial)", len(got.Participants))
	}
	if got.Participants[0].Error != 0 {
		t.Errorf("Participants[0].Error = %d, queria 0 (aprovado)", got.Participants[0].Error)
	}
	if got.Participants[1].Error != 409 {
		t.Errorf("Participants[1].Error = %d, queria 409 (sucesso parcial, não silenciado)", got.Participants[1].Error)
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
	if _, err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestReject); err != nil {
		t.Fatalf("UpdateRequestParticipants reject = %v", err)
	}
	if seen != wanoise.ParticipantChangeReject {
		t.Errorf("action = %v, want reject", seen)
	}
}

// TestGroupAdapter_UpdateRequestParticipants_Unknown.
func TestGroupAdapter_UpdateRequestParticipants_Unknown(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	_, err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestAction("weird"))
	if err == nil {
		t.Fatal("UpdateRequestParticipants com ação inválida = nil")
	}
}

// TestGroupAdapter_UpdateRequestParticipants_NoSession.
func TestGroupAdapter_UpdateRequestParticipants_NoSession(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(nil))
	_, err := a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", nil, domain.RequestApprove)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("UpdateRequestParticipants code = %q", testkit.AppErrCode(err))
	}
}

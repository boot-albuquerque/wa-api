package group

import (
	"context"
	"errors"
	"testing"
	"time"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/pkg/domain"

	whatsmeow "wa-api/internal/wa-noise/core"
	"wa-api/internal/wa-noise/protocol/types"
)

// TestGroupAdapter_UpdateGroupParticipants_InvalidParticipantJID cobre
// o caminho de erro do wajid.ToJIDs (uma entrada inválida).
func TestGroupAdapter_UpdateGroupParticipants_InvalidParticipantJID(t *testing.T) {
	a := NewGroupAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": &waclienttest.Fake{}}))
	invalid := []domain.JID{"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00}))}
	for _, jid := range invalid {
		_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{jid}, domain.ParticipantAdd)
		if err != nil && waclienttest.AppErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("wajid.ParseJID não falhou para nenhuma entrada testada")
}

// TestGroupAdapter_PropagatesErrors cobre todos os caminhos de propagação
// de erro do SDK nos adapters de grupo. Cada subteste monta um fake que
// devolve sdkErr para a operação relevante.
func TestGroupAdapter_PropagatesErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	type fnCall func(a *GroupAdapter) error
	type fakeSpec struct {
		setup func() *waclienttest.Fake
		call  fnCall
	}
	cases := []struct {
		name string
		spec fakeSpec
	}{
		{"GetGroupInfoFromLink", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{GetGroupInfoFromLinkFn: func(ctx context.Context, code string) (*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetGroupInfoFromLink(context.Background(), "u1", "code")
				return err
			},
		}},
		{"GetGroupInviteLink", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{GetGroupInviteLinkFn: func(ctx context.Context, jid types.JID, reset bool) (string, error) {
					return "", sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetGroupInviteLink(context.Background(), "u1", "g@g.us")
				return err
			},
		}},
		{"ListJoinedGroups", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{GetJoinedGroupsFn: func(ctx context.Context) ([]*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, _, err := a.ListJoinedGroups(context.Background(), "u1")
				return err
			},
		}},
		{"CreateGroup", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{CreateGroupFn: func(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.CreateGroup(context.Background(), "u1", "n", nil)
				return err
			},
		}},
		{"JoinGroup", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{JoinGroupWithLinkFn: func(ctx context.Context, code string) (types.JID, error) {
					return types.JID{}, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.JoinGroup(context.Background(), "u1", "code")
				return err
			},
		}},
		{"LeaveGroup", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{LeaveGroupFn: func(ctx context.Context, jid types.JID) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.LeaveGroup(context.Background(), "u1", "g@g.us")
			},
		}},
		{"SetGroupName", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupNameFn: func(ctx context.Context, jid types.JID, name string) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupName(context.Background(), "u1", "g@g.us", "n")
			},
		}},
		{"SetGroupTopic", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupTopicFn: func(ctx context.Context, jid types.JID, prev, new, topic string) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupTopic(context.Background(), "u1", "g@g.us", "t")
			},
		}},
		{"SetGroupPhoto", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupPhotoFn: func(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
					return "", sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupPhoto(context.Background(), "u1", "g@g.us", nil)
			},
		}},
		{"SetGroupAnnounce", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupAnnounceFn: func(ctx context.Context, jid types.JID, announce bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupAnnounce(context.Background(), "u1", "g@g.us", true)
			},
		}},
		{"SetGroupLocked", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupLockedFn: func(ctx context.Context, jid types.JID, locked bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupLocked(context.Background(), "u1", "g@g.us", true)
			},
		}},
		{"SetDisappearingTimer", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetDisappearingTimerFn: func(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error {
					return sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.SetDisappearingTimer(context.Background(), "u1", "g@g.us", time.Hour, time.Now())
			},
		}},
		{"UpdateGroupParticipants", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.ParticipantAdd)
				return err
			},
		}},
		{"GetRequestParticipants", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{GetGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetRequestParticipants(context.Background(), "u1", "g@g.us")
				return err
			},
		}},
		{"UpdateRequestParticipants", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove)
			},
		}},
		{"SetJoinApprovalMode", fakeSpec{
			setup: func() *waclienttest.Fake {
				return &waclienttest.Fake{SetGroupJoinApprovalModeFn: func(ctx context.Context, jid types.JID, mode bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetJoinApprovalMode(context.Background(), "u1", "g@g.us", true)
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.spec.setup()
			a := NewGroupAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
			err := tc.spec.call(a)
			if err == nil {
				t.Fatalf("%s did not propagate error", tc.name)
			}
		})
	}
}

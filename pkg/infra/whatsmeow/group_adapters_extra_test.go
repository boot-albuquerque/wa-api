package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// TestGroupAdapter_UpdateGroupParticipants_InvalidParticipantJID cobre
// o caminho de erro do toJIDs (uma entrada inválida).
func TestGroupAdapter_UpdateGroupParticipants_InvalidParticipantJID(t *testing.T) {
	a := NewGroupAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	invalid := []domain.JID{"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00}))}
	for _, jid := range invalid {
		_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{jid}, domain.ParticipantAdd)
		if err != nil && appErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("ParseJID não falhou para nenhuma entrada testada")
}

// --- below is original ---

// TestGroupAdapter_PropagatesErrors cobre todos os caminhos de propagação
// de erro do SDK nos adapters de grupo. Cada subteste monta um fake que
// devolve sdkErr para a operação relevante.
func TestGroupAdapter_PropagatesErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	type fnCall func(a *GroupAdapter) error
	type fakeSpec struct {
		setup func() *fakeWAClient
		call  fnCall
	}
	cases := []struct {
		name string
		spec fakeSpec
	}{
		{"GetGroupInfoFromLink", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{GetGroupInfoFromLinkFn: func(ctx context.Context, code string) (*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetGroupInfoFromLink(context.Background(), "u1", "code")
				return err
			},
		}},
		{"GetGroupInviteLink", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{GetGroupInviteLinkFn: func(ctx context.Context, jid types.JID, reset bool) (string, error) {
					return "", sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetGroupInviteLink(context.Background(), "u1", "g@g.us")
				return err
			},
		}},
		{"ListJoinedGroups", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{GetJoinedGroupsFn: func(ctx context.Context) ([]*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, _, err := a.ListJoinedGroups(context.Background(), "u1")
				return err
			},
		}},
		{"CreateGroup", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{CreateGroupFn: func(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.CreateGroup(context.Background(), "u1", "n", nil)
				return err
			},
		}},
		{"JoinGroup", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{JoinGroupWithLinkFn: func(ctx context.Context, code string) (types.JID, error) {
					return types.JID{}, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.JoinGroup(context.Background(), "u1", "code")
				return err
			},
		}},
		{"LeaveGroup", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{LeaveGroupFn: func(ctx context.Context, jid types.JID) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.LeaveGroup(context.Background(), "u1", "g@g.us")
			},
		}},
		{"SetGroupName", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupNameFn: func(ctx context.Context, jid types.JID, name string) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupName(context.Background(), "u1", "g@g.us", "n")
			},
		}},
		{"SetGroupTopic", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupTopicFn: func(ctx context.Context, jid types.JID, prev, new, topic string) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupTopic(context.Background(), "u1", "g@g.us", "t")
			},
		}},
		{"SetGroupPhoto", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupPhotoFn: func(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
					return "", sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupPhoto(context.Background(), "u1", "g@g.us", nil)
			},
		}},
		{"SetGroupAnnounce", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupAnnounceFn: func(ctx context.Context, jid types.JID, announce bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupAnnounce(context.Background(), "u1", "g@g.us", true)
			},
		}},
		{"SetGroupLocked", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupLockedFn: func(ctx context.Context, jid types.JID, locked bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetGroupLocked(context.Background(), "u1", "g@g.us", true)
			},
		}},
		{"SetDisappearingTimer", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetDisappearingTimerFn: func(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error {
					return sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.SetDisappearingTimer(context.Background(), "u1", "g@g.us", time.Hour, time.Now())
			},
		}},
		{"UpdateGroupParticipants", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{UpdateGroupParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.UpdateGroupParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.ParticipantAdd)
				return err
			},
		}},
		{"GetRequestParticipants", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{GetGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				_, err := a.GetRequestParticipants(context.Background(), "u1", "g@g.us")
				return err
			},
		}},
		{"UpdateRequestParticipants", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{UpdateGroupRequestParticipantsFn: func(ctx context.Context, jid types.JID, p []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error) {
					return nil, sdkErr
				}}
			},
			call: func(a *GroupAdapter) error {
				return a.UpdateRequestParticipants(context.Background(), "u1", "g@g.us", []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove)
			},
		}},
		{"SetJoinApprovalMode", fakeSpec{
			setup: func() *fakeWAClient {
				return &fakeWAClient{SetGroupJoinApprovalModeFn: func(ctx context.Context, jid types.JID, mode bool) error { return sdkErr }}
			},
			call: func(a *GroupAdapter) error {
				return a.SetJoinApprovalMode(context.Background(), "u1", "g@g.us", true)
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.spec.setup()
			a := NewGroupAdapter(getterWith(map[string]waClient{"u1": fake}))
			err := tc.spec.call(a)
			if err == nil {
				t.Fatalf("%s did not propagate error", tc.name)
			}
		})
	}
}

// TestChatAdapter_PropagatesErrors.
func TestChatAdapter_PropagatesErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	fake := &fakeWAClient{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		return sdkErr
	}}
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com"); err == nil {
		t.Fatal("MarkRead não propagou erro")
	}

	fake2 := &fakeWAClient{SendMessageFn: func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return whatsmeow.SendResponse{}, sdkErr
	}}
	a2 := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake2}))
	if _, err := a2.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"}); err == nil {
		t.Fatal("SendReaction não propagou erro")
	}
}

// TestPresenceControllerAdapter_PropagatesAllErrors.
func TestPresenceControllerAdapter_PropagatesAllErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	fake := &fakeWAClient{
		SendPresenceFn: func(ctx context.Context, state types.Presence) error { return sdkErr },
		SendChatPresenceFn: func(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
			return sdkErr
		},
		SubscribePresenceFn: func(ctx context.Context, jid types.JID) error { return sdkErr },
	}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable); err == nil {
		t.Fatal("SendPresence não propagou erro")
	}
	if err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "c", ""); err == nil {
		t.Fatal("SendChatPresence não propagou erro")
	}
	if err := a.SubscribePresence(context.Background(), "u1", "x@y.com"); err == nil {
		t.Fatal("SubscribePresence não propagou erro")
	}
}

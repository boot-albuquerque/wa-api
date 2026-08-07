package waclienttest

import (
	"context"
	"time"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
)

func (f *Fake) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	if f.GetGroupInfoFn != nil {
		return f.GetGroupInfoFn(ctx, jid)
	}
	return nil, nil
}

func (f *Fake) GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error) {
	if f.GetGroupInfoFromLinkFn != nil {
		return f.GetGroupInfoFromLinkFn(ctx, code)
	}
	return nil, nil
}

func (f *Fake) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	if f.GetGroupInviteLinkFn != nil {
		return f.GetGroupInviteLinkFn(ctx, jid, reset)
	}
	return "", nil
}

func (f *Fake) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	if f.GetJoinedGroupsFn != nil {
		return f.GetJoinedGroupsFn(ctx)
	}
	return nil, nil
}

func (f *Fake) CreateGroup(ctx context.Context, req wanoise.ReqCreateGroup) (*types.GroupInfo, error) {
	if f.CreateGroupFn != nil {
		return f.CreateGroupFn(ctx, req)
	}
	return nil, nil
}

func (f *Fake) JoinGroupWithLink(ctx context.Context, code string) (types.JID, error) {
	if f.JoinGroupWithLinkFn != nil {
		return f.JoinGroupWithLinkFn(ctx, code)
	}
	return types.JID{}, nil
}

func (f *Fake) LeaveGroup(ctx context.Context, jid types.JID) error {
	if f.LeaveGroupFn != nil {
		return f.LeaveGroupFn(ctx, jid)
	}
	return nil
}

func (f *Fake) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	if f.SetGroupNameFn != nil {
		return f.SetGroupNameFn(ctx, jid, name)
	}
	return nil
}

func (f *Fake) SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error {
	if f.SetGroupTopicFn != nil {
		return f.SetGroupTopicFn(ctx, jid, previousID, newID, topic)
	}
	return nil
}

func (f *Fake) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	if f.SetGroupPhotoFn != nil {
		return f.SetGroupPhotoFn(ctx, jid, avatar)
	}
	return "", nil
}

func (f *Fake) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	if f.SetGroupAnnounceFn != nil {
		return f.SetGroupAnnounceFn(ctx, jid, announce)
	}
	return nil
}

func (f *Fake) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	if f.SetGroupLockedFn != nil {
		return f.SetGroupLockedFn(ctx, jid, locked)
	}
	return nil
}

func (f *Fake) SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error {
	if f.SetDisappearingTimerFn != nil {
		return f.SetDisappearingTimerFn(ctx, chat, timer, settingTS)
	}
	return nil
}

func (f *Fake) UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
	if f.UpdateGroupParticipantsFn != nil {
		return f.UpdateGroupParticipantsFn(ctx, jid, participantChanges, action)
	}
	return nil, nil
}

func (f *Fake) GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
	if f.GetGroupRequestParticipantsFn != nil {
		return f.GetGroupRequestParticipantsFn(ctx, jid)
	}
	return nil, nil
}

func (f *Fake) UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error) {
	if f.UpdateGroupRequestParticipantsFn != nil {
		return f.UpdateGroupRequestParticipantsFn(ctx, jid, participantChanges, action)
	}
	return nil, nil
}

func (f *Fake) SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error {
	if f.SetGroupJoinApprovalModeFn != nil {
		return f.SetGroupJoinApprovalModeFn(ctx, jid, mode)
	}
	return nil
}

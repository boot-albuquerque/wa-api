package contractsfake

import (
	"context"
	"time"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- GroupDirectory ----------------------------------------------------

// GroupDirectoryGetGroupInfoCall é uma chamada a GetGroupInfo.
type GroupDirectoryGetGroupInfoCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
}

// GroupDirectoryGetGroupInfoFromLinkCall é uma chamada a
// GetGroupInfoFromLink.
type GroupDirectoryGetGroupInfoFromLinkCall struct {
	Ctx   context.Context
	TxtID string
	Code  string
}

// GroupDirectoryGetGroupInviteLinkCall é uma chamada a GetGroupInviteLink.
type GroupDirectoryGetGroupInviteLinkCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
}

// GroupDirectoryListJoinedGroupsCall é uma chamada a ListJoinedGroups.
type GroupDirectoryListJoinedGroupsCall struct {
	Ctx   context.Context
	TxtID string
}

// GroupDirectory é o fake de port.GroupDirectory.
type GroupDirectory struct {
	SessionGuard

	GetGroupInfoFunc  func(ctx context.Context, txtID string, group domain.JID) (any, error)
	GetGroupInfoCalls []GroupDirectoryGetGroupInfoCall

	GetGroupInfoFromLinkFunc  func(ctx context.Context, txtID, code string) (any, error)
	GetGroupInfoFromLinkCalls []GroupDirectoryGetGroupInfoFromLinkCall

	GetGroupInviteLinkFunc  func(ctx context.Context, txtID string, group domain.JID) (string, error)
	GetGroupInviteLinkCalls []GroupDirectoryGetGroupInviteLinkCall

	ListJoinedGroupsFunc  func(ctx context.Context, txtID string) (any, int, error)
	ListJoinedGroupsCalls []GroupDirectoryListJoinedGroupsCall
}

var _ port.GroupDirectory = (*GroupDirectory)(nil)

// GetGroupInfo implementa port.GroupDirectory.
func (f *GroupDirectory) GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (any, error) {
	f.GetGroupInfoCalls = append(f.GetGroupInfoCalls, GroupDirectoryGetGroupInfoCall{Ctx: ctx, TxtID: txtID, Group: group})
	if f.GetGroupInfoFunc != nil {
		return f.GetGroupInfoFunc(ctx, txtID, group)
	}
	return nil, nil
}

// GetGroupInfoFromLink implementa port.GroupDirectory.
func (f *GroupDirectory) GetGroupInfoFromLink(ctx context.Context, txtID, code string) (any, error) {
	f.GetGroupInfoFromLinkCalls = append(f.GetGroupInfoFromLinkCalls, GroupDirectoryGetGroupInfoFromLinkCall{Ctx: ctx, TxtID: txtID, Code: code})
	if f.GetGroupInfoFromLinkFunc != nil {
		return f.GetGroupInfoFromLinkFunc(ctx, txtID, code)
	}
	return nil, nil
}

// GetGroupInviteLink implementa port.GroupDirectory.
func (f *GroupDirectory) GetGroupInviteLink(ctx context.Context, txtID string, group domain.JID) (string, error) {
	f.GetGroupInviteLinkCalls = append(f.GetGroupInviteLinkCalls, GroupDirectoryGetGroupInviteLinkCall{Ctx: ctx, TxtID: txtID, Group: group})
	if f.GetGroupInviteLinkFunc != nil {
		return f.GetGroupInviteLinkFunc(ctx, txtID, group)
	}
	return "", nil
}

// ListJoinedGroups implementa port.GroupDirectory.
func (f *GroupDirectory) ListJoinedGroups(ctx context.Context, txtID string) (any, int, error) {
	f.ListJoinedGroupsCalls = append(f.ListJoinedGroupsCalls, GroupDirectoryListJoinedGroupsCall{Ctx: ctx, TxtID: txtID})
	if f.ListJoinedGroupsFunc != nil {
		return f.ListJoinedGroupsFunc(ctx, txtID)
	}
	return nil, 0, nil
}

// --- GroupLifecycle ----------------------------------------------------

// GroupLifecycleCreateGroupCall é uma chamada a CreateGroup.
type GroupLifecycleCreateGroupCall struct {
	Ctx          context.Context
	TxtID        string
	Name         string
	Participants []domain.JID
}

// GroupLifecycleJoinGroupCall é uma chamada a JoinGroup.
type GroupLifecycleJoinGroupCall struct {
	Ctx   context.Context
	TxtID string
	Code  string
}

// GroupLifecycleLeaveGroupCall é uma chamada a LeaveGroup.
type GroupLifecycleLeaveGroupCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
}

// GroupLifecycle é o fake de port.GroupLifecycle.
type GroupLifecycle struct {
	SessionGuard

	CreateGroupFunc  func(ctx context.Context, txtID, name string, participants []domain.JID) (any, error)
	CreateGroupCalls []GroupLifecycleCreateGroupCall

	JoinGroupFunc  func(ctx context.Context, txtID, code string) (any, error)
	JoinGroupCalls []GroupLifecycleJoinGroupCall

	LeaveGroupFunc  func(ctx context.Context, txtID string, group domain.JID) error
	LeaveGroupCalls []GroupLifecycleLeaveGroupCall
}

var _ port.GroupLifecycle = (*GroupLifecycle)(nil)

// CreateGroup implementa port.GroupLifecycle.
func (f *GroupLifecycle) CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID) (any, error) {
	f.CreateGroupCalls = append(f.CreateGroupCalls, GroupLifecycleCreateGroupCall{Ctx: ctx, TxtID: txtID, Name: name, Participants: participants})
	if f.CreateGroupFunc != nil {
		return f.CreateGroupFunc(ctx, txtID, name, participants)
	}
	return nil, nil
}

// JoinGroup implementa port.GroupLifecycle.
func (f *GroupLifecycle) JoinGroup(ctx context.Context, txtID, code string) (any, error) {
	f.JoinGroupCalls = append(f.JoinGroupCalls, GroupLifecycleJoinGroupCall{Ctx: ctx, TxtID: txtID, Code: code})
	if f.JoinGroupFunc != nil {
		return f.JoinGroupFunc(ctx, txtID, code)
	}
	return nil, nil
}

// LeaveGroup implementa port.GroupLifecycle.
func (f *GroupLifecycle) LeaveGroup(ctx context.Context, txtID string, group domain.JID) error {
	f.LeaveGroupCalls = append(f.LeaveGroupCalls, GroupLifecycleLeaveGroupCall{Ctx: ctx, TxtID: txtID, Group: group})
	if f.LeaveGroupFunc != nil {
		return f.LeaveGroupFunc(ctx, txtID, group)
	}
	return nil
}

// --- GroupSettings -----------------------------------------------------

// GroupSettingsSetGroupNameCall é uma chamada a SetGroupName.
type GroupSettingsSetGroupNameCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
	Name  string
}

// GroupSettingsSetGroupTopicCall é uma chamada a SetGroupTopic.
type GroupSettingsSetGroupTopicCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
	Topic string
}

// GroupSettingsSetGroupPhotoCall é uma chamada a SetGroupPhoto. Photo nil
// significa remoção — é o contrato da porta, não um argumento faltando.
type GroupSettingsSetGroupPhotoCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
	Photo []byte
}

// GroupSettingsSetGroupAnnounceCall é uma chamada a SetGroupAnnounce.
type GroupSettingsSetGroupAnnounceCall struct {
	Ctx      context.Context
	TxtID    string
	Group    domain.JID
	Announce bool
}

// GroupSettingsSetGroupLockedCall é uma chamada a SetGroupLocked.
type GroupSettingsSetGroupLockedCall struct {
	Ctx    context.Context
	TxtID  string
	Group  domain.JID
	Locked bool
}

// GroupSettingsSetDisappearingTimerCall é uma chamada a
// SetDisappearingTimer.
type GroupSettingsSetDisappearingTimerCall struct {
	Ctx      context.Context
	TxtID    string
	Group    domain.JID
	Duration time.Duration
	At       time.Time
}

// GroupSettingsUpdateGroupParticipantsCall é uma chamada a
// UpdateGroupParticipants.
type GroupSettingsUpdateGroupParticipantsCall struct {
	Ctx          context.Context
	TxtID        string
	Group        domain.JID
	Participants []domain.JID
	Action       domain.ParticipantAction
}

// GroupSettings é o fake de port.GroupSettings.
type GroupSettings struct {
	SessionGuard

	SetGroupNameFunc  func(ctx context.Context, txtID string, group domain.JID, name string) error
	SetGroupNameCalls []GroupSettingsSetGroupNameCall

	SetGroupTopicFunc  func(ctx context.Context, txtID string, group domain.JID, topic string) error
	SetGroupTopicCalls []GroupSettingsSetGroupTopicCall

	SetGroupPhotoFunc  func(ctx context.Context, txtID string, group domain.JID, photo []byte) error
	SetGroupPhotoCalls []GroupSettingsSetGroupPhotoCall

	SetGroupAnnounceFunc  func(ctx context.Context, txtID string, group domain.JID, announce bool) error
	SetGroupAnnounceCalls []GroupSettingsSetGroupAnnounceCall

	SetGroupLockedFunc  func(ctx context.Context, txtID string, group domain.JID, locked bool) error
	SetGroupLockedCalls []GroupSettingsSetGroupLockedCall

	SetDisappearingTimerFunc  func(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error
	SetDisappearingTimerCalls []GroupSettingsSetDisappearingTimerCall

	UpdateGroupParticipantsFunc  func(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (any, error)
	UpdateGroupParticipantsCalls []GroupSettingsUpdateGroupParticipantsCall
}

var _ port.GroupSettings = (*GroupSettings)(nil)

// SetGroupName implementa port.GroupSettings.
func (f *GroupSettings) SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error {
	f.SetGroupNameCalls = append(f.SetGroupNameCalls, GroupSettingsSetGroupNameCall{Ctx: ctx, TxtID: txtID, Group: group, Name: name})
	if f.SetGroupNameFunc != nil {
		return f.SetGroupNameFunc(ctx, txtID, group, name)
	}
	return nil
}

// SetGroupTopic implementa port.GroupSettings.
func (f *GroupSettings) SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error {
	f.SetGroupTopicCalls = append(f.SetGroupTopicCalls, GroupSettingsSetGroupTopicCall{Ctx: ctx, TxtID: txtID, Group: group, Topic: topic})
	if f.SetGroupTopicFunc != nil {
		return f.SetGroupTopicFunc(ctx, txtID, group, topic)
	}
	return nil
}

// SetGroupPhoto implementa port.GroupSettings.
func (f *GroupSettings) SetGroupPhoto(ctx context.Context, txtID string, group domain.JID, photo []byte) error {
	f.SetGroupPhotoCalls = append(f.SetGroupPhotoCalls, GroupSettingsSetGroupPhotoCall{Ctx: ctx, TxtID: txtID, Group: group, Photo: photo})
	if f.SetGroupPhotoFunc != nil {
		return f.SetGroupPhotoFunc(ctx, txtID, group, photo)
	}
	return nil
}

// SetGroupAnnounce implementa port.GroupSettings.
func (f *GroupSettings) SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error {
	f.SetGroupAnnounceCalls = append(f.SetGroupAnnounceCalls, GroupSettingsSetGroupAnnounceCall{Ctx: ctx, TxtID: txtID, Group: group, Announce: announce})
	if f.SetGroupAnnounceFunc != nil {
		return f.SetGroupAnnounceFunc(ctx, txtID, group, announce)
	}
	return nil
}

// SetGroupLocked implementa port.GroupSettings.
func (f *GroupSettings) SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error {
	f.SetGroupLockedCalls = append(f.SetGroupLockedCalls, GroupSettingsSetGroupLockedCall{Ctx: ctx, TxtID: txtID, Group: group, Locked: locked})
	if f.SetGroupLockedFunc != nil {
		return f.SetGroupLockedFunc(ctx, txtID, group, locked)
	}
	return nil
}

// SetDisappearingTimer implementa port.GroupSettings.
func (f *GroupSettings) SetDisappearingTimer(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error {
	f.SetDisappearingTimerCalls = append(f.SetDisappearingTimerCalls, GroupSettingsSetDisappearingTimerCall{Ctx: ctx, TxtID: txtID, Group: group, Duration: d, At: at})
	if f.SetDisappearingTimerFunc != nil {
		return f.SetDisappearingTimerFunc(ctx, txtID, group, d, at)
	}
	return nil
}

// UpdateGroupParticipants implementa port.GroupSettings.
func (f *GroupSettings) UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (any, error) {
	f.UpdateGroupParticipantsCalls = append(f.UpdateGroupParticipantsCalls, GroupSettingsUpdateGroupParticipantsCall{Ctx: ctx, TxtID: txtID, Group: group, Participants: participants, Action: action})
	if f.UpdateGroupParticipantsFunc != nil {
		return f.UpdateGroupParticipantsFunc(ctx, txtID, group, participants, action)
	}
	return nil, nil
}

// --- GroupRequests -----------------------------------------------------

// GroupRequestsGetRequestParticipantsCall é uma chamada a
// GetRequestParticipants.
type GroupRequestsGetRequestParticipantsCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
}

// GroupRequestsUpdateRequestParticipantsCall é uma chamada a
// UpdateRequestParticipants.
type GroupRequestsUpdateRequestParticipantsCall struct {
	Ctx          context.Context
	TxtID        string
	Group        domain.JID
	Participants []domain.JID
	Action       domain.RequestAction
}

// GroupRequestsSetJoinApprovalModeCall é uma chamada a SetJoinApprovalMode.
type GroupRequestsSetJoinApprovalModeCall struct {
	Ctx   context.Context
	TxtID string
	Group domain.JID
	Mode  bool
}

// GroupRequests é o fake de port.GroupRequests.
type GroupRequests struct {
	SessionGuard

	GetRequestParticipantsFunc  func(ctx context.Context, txtID string, group domain.JID) (any, error)
	GetRequestParticipantsCalls []GroupRequestsGetRequestParticipantsCall

	UpdateRequestParticipantsFunc  func(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) error
	UpdateRequestParticipantsCalls []GroupRequestsUpdateRequestParticipantsCall

	SetJoinApprovalModeFunc  func(ctx context.Context, txtID string, group domain.JID, mode bool) error
	SetJoinApprovalModeCalls []GroupRequestsSetJoinApprovalModeCall
}

var _ port.GroupRequests = (*GroupRequests)(nil)

// GetRequestParticipants implementa port.GroupRequests.
func (f *GroupRequests) GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) (any, error) {
	f.GetRequestParticipantsCalls = append(f.GetRequestParticipantsCalls, GroupRequestsGetRequestParticipantsCall{Ctx: ctx, TxtID: txtID, Group: group})
	if f.GetRequestParticipantsFunc != nil {
		return f.GetRequestParticipantsFunc(ctx, txtID, group)
	}
	return nil, nil
}

// UpdateRequestParticipants implementa port.GroupRequests.
func (f *GroupRequests) UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) error {
	f.UpdateRequestParticipantsCalls = append(f.UpdateRequestParticipantsCalls, GroupRequestsUpdateRequestParticipantsCall{Ctx: ctx, TxtID: txtID, Group: group, Participants: participants, Action: action})
	if f.UpdateRequestParticipantsFunc != nil {
		return f.UpdateRequestParticipantsFunc(ctx, txtID, group, participants, action)
	}
	return nil
}

// SetJoinApprovalMode implementa port.GroupRequests.
func (f *GroupRequests) SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error {
	f.SetJoinApprovalModeCalls = append(f.SetJoinApprovalModeCalls, GroupRequestsSetJoinApprovalModeCall{Ctx: ctx, TxtID: txtID, Group: group, Mode: mode})
	if f.SetJoinApprovalModeFunc != nil {
		return f.SetJoinApprovalModeFunc(ctx, txtID, group, mode)
	}
	return nil
}

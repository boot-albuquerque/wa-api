package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- CommunityDirectory --------------------------------------------------

// CommunityDirectoryGetSubGroupsCall records a call to GetSubGroups.
type CommunityDirectoryGetSubGroupsCall struct {
	Ctx       context.Context
	TxtID     string
	Community domain.JID
}

// CommunityDirectoryGetLinkedGroupsParticipantsCall records a call to
// GetLinkedGroupsParticipants.
type CommunityDirectoryGetLinkedGroupsParticipantsCall struct {
	Ctx       context.Context
	TxtID     string
	Community domain.JID
}

// CommunityDirectory is the fake for port.CommunityDirectory.
type CommunityDirectory struct {
	SessionGuard

	GetSubGroupsFunc  func(ctx context.Context, txtID string, community domain.JID) ([]domain.CommunitySubGroup, error)
	GetSubGroupsCalls []CommunityDirectoryGetSubGroupsCall

	GetLinkedGroupsParticipantsFunc  func(ctx context.Context, txtID string, community domain.JID) ([]domain.JID, error)
	GetLinkedGroupsParticipantsCalls []CommunityDirectoryGetLinkedGroupsParticipantsCall
}

var _ port.CommunityDirectory = (*CommunityDirectory)(nil)

// GetSubGroups implements port.CommunityDirectory.
func (f *CommunityDirectory) GetSubGroups(ctx context.Context, txtID string, community domain.JID) ([]domain.CommunitySubGroup, error) {
	f.GetSubGroupsCalls = append(f.GetSubGroupsCalls, CommunityDirectoryGetSubGroupsCall{Ctx: ctx, TxtID: txtID, Community: community})
	if f.GetSubGroupsFunc != nil {
		return f.GetSubGroupsFunc(ctx, txtID, community)
	}
	// Empty and NON-nil, because that is what the real adapter returns for a
	// community with no children. A double that answered nil would be more
	// permissive than production and would bless a nil check nobody needs
	// (ARMADILHAS #1).
	return []domain.CommunitySubGroup{}, nil
}

// GetLinkedGroupsParticipants implements port.CommunityDirectory.
func (f *CommunityDirectory) GetLinkedGroupsParticipants(ctx context.Context, txtID string, community domain.JID) ([]domain.JID, error) {
	f.GetLinkedGroupsParticipantsCalls = append(f.GetLinkedGroupsParticipantsCalls, CommunityDirectoryGetLinkedGroupsParticipantsCall{Ctx: ctx, TxtID: txtID, Community: community})
	if f.GetLinkedGroupsParticipantsFunc != nil {
		return f.GetLinkedGroupsParticipantsFunc(ctx, txtID, community)
	}
	return []domain.JID{}, nil
}

// --- CommunityLifecycle --------------------------------------------------

// CommunityLifecycleLinkGroupCall records a call to LinkGroup.
type CommunityLifecycleLinkGroupCall struct {
	Ctx    context.Context
	TxtID  string
	Parent domain.JID
	Child  domain.JID
}

// CommunityLifecycleUnlinkGroupCall records a call to UnlinkGroup.
type CommunityLifecycleUnlinkGroupCall struct {
	Ctx    context.Context
	TxtID  string
	Parent domain.JID
	Child  domain.JID
}

// CommunityLifecycle is the fake for port.CommunityLifecycle.
type CommunityLifecycle struct {
	SessionGuard

	LinkGroupFunc  func(ctx context.Context, txtID string, parent, child domain.JID) error
	LinkGroupCalls []CommunityLifecycleLinkGroupCall

	UnlinkGroupFunc  func(ctx context.Context, txtID string, parent, child domain.JID) error
	UnlinkGroupCalls []CommunityLifecycleUnlinkGroupCall
}

var _ port.CommunityLifecycle = (*CommunityLifecycle)(nil)

// LinkGroup implements port.CommunityLifecycle.
func (f *CommunityLifecycle) LinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error {
	f.LinkGroupCalls = append(f.LinkGroupCalls, CommunityLifecycleLinkGroupCall{Ctx: ctx, TxtID: txtID, Parent: parent, Child: child})
	if f.LinkGroupFunc != nil {
		return f.LinkGroupFunc(ctx, txtID, parent, child)
	}
	return nil
}

// UnlinkGroup implements port.CommunityLifecycle.
func (f *CommunityLifecycle) UnlinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error {
	f.UnlinkGroupCalls = append(f.UnlinkGroupCalls, CommunityLifecycleUnlinkGroupCall{Ctx: ctx, TxtID: txtID, Parent: parent, Child: child})
	if f.UnlinkGroupFunc != nil {
		return f.UnlinkGroupFunc(ctx, txtID, parent, child)
	}
	return nil
}

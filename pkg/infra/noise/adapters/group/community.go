package group

import (
	"context"

	"wa-api/pkg/domain"
	"wa-api/pkg/infra/noise/errmap"
	wajid "wa-api/pkg/infra/noise/mapping/jid"

	clientpkg "wa-api/pkg/infra/noise/client"
)

// GetSubGroups returns the child groups of a community.
func (a *GroupAdapter) GetSubGroups(ctx context.Context, txtID string, community domain.JID) ([]domain.CommunitySubGroup, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(community)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, clientpkg.RequestTimeout)
	defer cancel()
	res, err := client.GetSubGroups(ctxWithTimeout, jid)
	if err := errmap.ClassifyIQ(err); err != nil {
		return nil, err
	}
	out := make([]domain.CommunitySubGroup, 0, len(res))
	for _, g := range res {
		if g == nil {
			continue
		}
		out = append(out, domain.CommunitySubGroup{
			JID:               jidOrEmpty(g.JID),
			Name:              g.Name,
			IsDefaultSubGroup: g.IsDefaultSubGroup,
		})
	}
	return out, nil
}

// GetLinkedGroupsParticipants returns participants across all linked groups.
func (a *GroupAdapter) GetLinkedGroupsParticipants(ctx context.Context, txtID string, community domain.JID) ([]domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(community)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, clientpkg.RequestTimeout)
	defer cancel()
	res, err := client.GetLinkedGroupsParticipants(ctxWithTimeout, jid)
	if err := errmap.ClassifyIQ(err); err != nil {
		return nil, err
	}
	out := make([]domain.JID, 0, len(res))
	for _, p := range res {
		out = append(out, jidOrEmpty(p))
	}
	return out, nil
}

// LinkGroup adds an existing group as a child of a community.
func (a *GroupAdapter) LinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	parentJID, err := wajid.ToJID(parent)
	if err != nil {
		return err
	}
	childJID, err := wajid.ToJID(child)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, clientpkg.RequestTimeout)
	defer cancel()
	return errmap.ClassifyIQ(client.LinkGroup(ctxWithTimeout, parentJID, childJID))
}

// UnlinkGroup removes a child group from a community.
func (a *GroupAdapter) UnlinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	parentJID, err := wajid.ToJID(parent)
	if err != nil {
		return err
	}
	childJID, err := wajid.ToJID(child)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, clientpkg.RequestTimeout)
	defer cancel()
	return errmap.ClassifyIQ(client.UnlinkGroup(ctxWithTimeout, parentJID, childJID))
}

package group

import (
	"context"

	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-noise/errmap"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	waclient "wa-api/pkg/infra/wa-noise/client"
)

// GetSubGroups returns the child groups of a community.
func (a *GroupAdapter) GetSubGroups(ctx context.Context, txtID string, community domain.JID) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(community)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	res, err := client.GetSubGroups(ctxWithTimeout, jid)
	return res, errmap.ClassifyIQ(err)
}

// GetLinkedGroupsParticipants returns participants across all linked groups.
func (a *GroupAdapter) GetLinkedGroupsParticipants(ctx context.Context, txtID string, community domain.JID) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(community)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	res, err := client.GetLinkedGroupsParticipants(ctxWithTimeout, jid)
	return res, errmap.ClassifyIQ(err)
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
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
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
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return errmap.ClassifyIQ(client.UnlinkGroup(ctxWithTimeout, parentJID, childJID))
}

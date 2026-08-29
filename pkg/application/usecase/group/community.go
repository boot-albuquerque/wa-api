package group

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const (
	opGetSubGroups    = "GetCommunitySubGroups"
	opGetParticipants = "GetCommunityParticipants"
	opLinkGroup       = "LinkGroupToCommunity"
	opUnlinkGroup     = "UnlinkGroupFromCommunity"
)

// CommunityReadUseCase handles read operations on communities.
type CommunityReadUseCase struct {
	dir    appport.CommunityDirectory
	jids   appport.JIDResolver
	logger appport.Logger
}

func NewCommunityReadUseCase(
	dir appport.CommunityDirectory,
	jids appport.JIDResolver,
	l appport.Logger,
) *CommunityReadUseCase {
	return &CommunityReadUseCase{dir: dir, jids: jids, logger: l}
}

func (uc *CommunityReadUseCase) parseCommunityJID(ctx context.Context, s string) (domain.JID, error) {
	jid, err := uc.jids.ResolveJID(ctx, s)
	if err != nil {
		uc.logger.Warn(ctx, "could not parse community JID", "jid", s, "error", err)
		return "", apperr.New("invalid_community_jid", apperr.CategoryValidation,
			fmt.Sprintf("could not parse community JID %q", s), false, err)
	}
	return jid, nil
}

// GetSubGroups returns the subgroups of a community.
func (uc *CommunityReadUseCase) GetSubGroups(ctx context.Context, txtID string, req domain.GetCommunitySubGroupsRequest) (*domain.GetCommunitySubGroupsResult, error) {
	if req.CommunityJID == "" {
		uc.logger.Warn(ctx, "missing communityJID", "txtID", txtID)
		return nil, apperr.New("missing_community_jid", apperr.CategoryValidation,
			"missing communityJID parameter", false, nil)
	}
	community, err := uc.parseCommunityJID(ctx, req.CommunityJID)
	if err != nil {
		return nil, err
	}
	if err := uc.dir.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}
	res, err := uc.dir.GetSubGroups(ctx, txtID, community)
	if err != nil {
		uc.logger.Error(ctx, "failed to get community sub groups", "txtID", txtID, "communityJID", req.CommunityJID, "error", err)
		return nil, fmt.Errorf("failed to get community sub groups: %w", err)
	}
	uc.logger.Info(ctx, opGetSubGroups, "txtID", txtID, "communityJID", req.CommunityJID)
	return &domain.GetCommunitySubGroupsResult{SubGroups: res}, nil
}

// GetParticipants returns participants across all linked groups.
func (uc *CommunityReadUseCase) GetParticipants(ctx context.Context, txtID string, req domain.GetCommunityParticipantsRequest) (*domain.GetCommunityParticipantsResult, error) {
	if req.CommunityJID == "" {
		uc.logger.Warn(ctx, "missing communityJID", "txtID", txtID)
		return nil, apperr.New("missing_community_jid", apperr.CategoryValidation,
			"missing communityJID parameter", false, nil)
	}
	community, err := uc.parseCommunityJID(ctx, req.CommunityJID)
	if err != nil {
		return nil, err
	}
	if err := uc.dir.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}
	res, err := uc.dir.GetLinkedGroupsParticipants(ctx, txtID, community)
	if err != nil {
		uc.logger.Error(ctx, "failed to get community participants", "txtID", txtID, "communityJID", req.CommunityJID, "error", err)
		return nil, fmt.Errorf("failed to get community participants: %w", err)
	}
	uc.logger.Info(ctx, opGetParticipants, "txtID", txtID, "communityJID", req.CommunityJID)
	return &domain.GetCommunityParticipantsResult{Participants: res}, nil
}

// CommunityWriteUseCase handles link/unlink operations.
type CommunityWriteUseCase struct {
	lifecycle appport.CommunityLifecycle
	jids      appport.JIDResolver
	logger    appport.Logger
}

func NewCommunityWriteUseCase(
	lc appport.CommunityLifecycle,
	jids appport.JIDResolver,
	l appport.Logger,
) *CommunityWriteUseCase {
	return &CommunityWriteUseCase{lifecycle: lc, jids: jids, logger: l}
}

func (uc *CommunityWriteUseCase) parseJID(ctx context.Context, s, label string) (domain.JID, error) {
	jid, err := uc.jids.ResolveJID(ctx, s)
	if err != nil {
		uc.logger.Warn(ctx, "could not parse JID", "label", label, "jid", s, "error", err)
		return "", apperr.New("invalid_jid", apperr.CategoryValidation,
			fmt.Sprintf("could not parse %s JID %q", label, s), false, err)
	}
	return jid, nil
}

// LinkGroup links an existing group to a community.
func (uc *CommunityWriteUseCase) LinkGroup(ctx context.Context, txtID string, req domain.CommunityLinkRequest) error {
	if req.CommunityJID == "" {
		return apperr.New("missing_community_jid", apperr.CategoryValidation,
			"missing communityJID parameter", false, nil)
	}
	if req.GroupJID == "" {
		return apperr.New("missing_group_jid", apperr.CategoryValidation,
			"missing groupJID parameter", false, nil)
	}
	parent, err := uc.parseJID(ctx, req.CommunityJID, "community")
	if err != nil {
		return err
	}
	child, err := uc.parseJID(ctx, req.GroupJID, "group")
	if err != nil {
		return err
	}
	if err := uc.lifecycle.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return err
	}
	if err := uc.lifecycle.LinkGroup(ctx, txtID, parent, child); err != nil {
		uc.logger.Error(ctx, "failed to link group to community",
			"txtID", txtID, "communityJID", req.CommunityJID, "groupJID", req.GroupJID, "error", err)
		return fmt.Errorf("failed to link group to community: %w", err)
	}
	uc.logger.Info(ctx, opLinkGroup, "txtID", txtID, "communityJID", req.CommunityJID, "groupJID", req.GroupJID)
	return nil
}

// UnlinkGroup unlinks a group from a community.
func (uc *CommunityWriteUseCase) UnlinkGroup(ctx context.Context, txtID string, req domain.CommunityUnlinkRequest) error {
	if req.CommunityJID == "" {
		return apperr.New("missing_community_jid", apperr.CategoryValidation,
			"missing communityJID parameter", false, nil)
	}
	if req.GroupJID == "" {
		return apperr.New("missing_group_jid", apperr.CategoryValidation,
			"missing groupJID parameter", false, nil)
	}
	parent, err := uc.parseJID(ctx, req.CommunityJID, "community")
	if err != nil {
		return err
	}
	child, err := uc.parseJID(ctx, req.GroupJID, "group")
	if err != nil {
		return err
	}
	if err := uc.lifecycle.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return err
	}
	if err := uc.lifecycle.UnlinkGroup(ctx, txtID, parent, child); err != nil {
		uc.logger.Error(ctx, "failed to unlink group from community",
			"txtID", txtID, "communityJID", req.CommunityJID, "groupJID", req.GroupJID, "error", err)
		return fmt.Errorf("failed to unlink group from community: %w", err)
	}
	uc.logger.Info(ctx, opUnlinkGroup, "txtID", txtID, "communityJID", req.CommunityJID, "groupJID", req.GroupJID)
	return nil
}

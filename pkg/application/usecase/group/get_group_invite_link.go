package group

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetGroupInviteLinkUseCase encapsula a validação para obter link de convite
type GetGroupInviteLinkUseCase struct {
	groups appport.GroupDirectory
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewGetGroupInviteLinkUseCase cria uma nova instância do usecase
func NewGetGroupInviteLinkUseCase(gd appport.GroupDirectory, jr appport.JIDResolver, l appport.Logger) *GetGroupInviteLinkUseCase {
	return &GetGroupInviteLinkUseCase{
		groups: gd,
		jids:   jr,
		logger: l,
	}
}

// Execute valida os campos e obtém o link de convite do grupo
func (uc *GetGroupInviteLinkUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInviteLinkRequest) (*domain.GetGroupInviteLinkResult, error) {
	// Validar GroupJID
	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	// Parse GroupJID
	group, err := uc.jids.ResolveJID(ctx, req.GroupJID)
	if err != nil {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	// Garantir que há sessão
	if err := uc.groups.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	// Obter link de convite
	link, err := uc.groups.GetGroupInviteLink(ctx, txtID, group)
	if err != nil {
		uc.logger.Error(ctx, "failed to get group invite link", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, fmt.Errorf("failed to get group invite link: %v", err)
	}

	result := &domain.GetGroupInviteLinkResult{
		InviteLink: link,
	}

	uc.logger.Info(ctx, "group invite link retrieved", "txtID", txtID, "groupJID", req.GroupJID)
	return result, nil
}

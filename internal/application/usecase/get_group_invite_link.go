package usecase

import (
	"context"
	"fmt"

	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
	"disparazap/internal/infra/whatsmeow"
)

// GetGroupInviteLinkUseCase encapsula a validação para obter link de convite
type GetGroupInviteLinkUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetGroupInviteLinkUseCase cria uma nova instância do usecase
func NewGetGroupInviteLinkUseCase(cp appport.ClientProvider, l appport.Logger) *GetGroupInviteLinkUseCase {
	return &GetGroupInviteLinkUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos e obtém o link de convite do grupo
func (uc *GetGroupInviteLinkUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInviteLinkRequest) (*domain.GetGroupInviteLinkResult, error) {
	// Validar GroupJID
	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	// Parse GroupJID
	group, ok := whatsmeow.ParseJID(req.GroupJID)
	if !ok {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	// Obter cliente whatsmeow
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	// Obter link de convite
	link, err := client.GetGroupInviteLink(ctx, group, false)
	if err != nil {
		uc.logger.Error("failed to get group invite link", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, fmt.Errorf("failed to get group invite link: %v", err)
	}

	result := &domain.GetGroupInviteLinkResult{
		InviteLink: link,
	}

	uc.logger.Info("group invite link retrieved", "txtID", txtID, "groupJID", req.GroupJID)
	return result, nil
}

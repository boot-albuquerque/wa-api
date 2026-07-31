package group

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// GetGroupInviteInfoUseCase encapsula a validação para obter informações de convite
type GetGroupInviteInfoUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetGroupInviteInfoUseCase cria uma nova instância do usecase
func NewGetGroupInviteInfoUseCase(cp appport.ClientProvider, l appport.Logger) *GetGroupInviteInfoUseCase {
	return &GetGroupInviteInfoUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos e obtém as informações do convite
func (uc *GetGroupInviteInfoUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInviteInfoRequest) (*domain.GetGroupInviteInfoResult, error) {
	// Validar Code
	if req.Code == "" {
		return nil, fmt.Errorf("missing Code in payload")
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

	// Obter informações do convite
	inviteInfo, err := client.GetGroupInfoFromLink(ctx, req.Code)
	if err != nil {
		uc.logger.Error("failed to get group invite info", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("failed to get group invite info: %v", err)
	}

	result := &domain.GetGroupInviteInfoResult{
		InviteInfo: inviteInfo,
	}

	uc.logger.Info("group invite info retrieved", "txtID", txtID)
	return result, nil
}

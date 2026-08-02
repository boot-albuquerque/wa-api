package group

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetGroupInviteInfoUseCase encapsula a validação para obter informações de convite
type GetGroupInviteInfoUseCase struct {
	groups appport.GroupDirectory
	logger appport.Logger
}

// NewGetGroupInviteInfoUseCase cria uma nova instância do usecase
func NewGetGroupInviteInfoUseCase(gd appport.GroupDirectory, l appport.Logger) *GetGroupInviteInfoUseCase {
	return &GetGroupInviteInfoUseCase{
		groups: gd,
		logger: l,
	}
}

// Execute valida os campos e obtém as informações do convite
func (uc *GetGroupInviteInfoUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInviteInfoRequest) (*domain.GetGroupInviteInfoResult, error) {
	// Validar Code
	if req.Code == "" {
		return nil, fmt.Errorf("missing Code in payload")
	}

	// Garantir que há sessão
	if err := uc.groups.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	// Obter informações do convite
	inviteInfo, err := uc.groups.GetGroupInfoFromLink(ctx, txtID, req.Code)
	if err != nil {
		uc.logger.Error(ctx, "failed to get group invite info", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("failed to get group invite info: %v", err)
	}

	result := &domain.GetGroupInviteInfoResult{
		InviteInfo: inviteInfo,
	}

	uc.logger.Info(ctx, "group invite info retrieved", "txtID", txtID)
	return result, nil
}

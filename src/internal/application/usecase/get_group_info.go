package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
	"wa-api/internal/infra/whatsmeow"
)

// GetGroupInfoUseCase encapsula a validação para obter informações de grupo
type GetGroupInfoUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetGroupInfoUseCase cria uma nova instância do usecase
func NewGetGroupInfoUseCase(cp appport.ClientProvider, l appport.Logger) *GetGroupInfoUseCase {
	return &GetGroupInfoUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos e obtém as informações do grupo
func (uc *GetGroupInfoUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInfoRequest) (*domain.GetGroupInfoResult, error) {
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

	// Obter informações do grupo
	groupInfo, err := client.GetGroupInfo(ctx, group)
	if err != nil {
		uc.logger.Error("failed to get group info", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, fmt.Errorf("Failed to get group info: %v", err)
	}

	result := &domain.GetGroupInfoResult{
		GroupInfo: groupInfo,
	}

	uc.logger.Info("group info retrieved", "txtID", txtID, "groupJID", req.GroupJID)
	return result, nil
}

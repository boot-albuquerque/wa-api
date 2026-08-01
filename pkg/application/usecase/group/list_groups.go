package group

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ListGroupsUseCase encapsula a validação para listar grupos
type ListGroupsUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewListGroupsUseCase cria uma nova instância do usecase
func NewListGroupsUseCase(cp appport.ClientProvider, l appport.Logger) *ListGroupsUseCase {
	return &ListGroupsUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível e obtém a lista de grupos
func (uc *ListGroupsUseCase) Execute(ctx context.Context, txtID string, req domain.ListGroupsRequest) (*domain.ListGroupsResult, error) {
	// Obter cliente whatsmeow
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	// Obter grupos
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		uc.logger.Error(ctx, "failed to get groups", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("failed to get group list: %v", err)
	}

	result := &domain.ListGroupsResult{
		Groups: groups,
	}

	uc.logger.Info(ctx, "groups listed successfully", "txtID", txtID, "count", len(groups))
	return result, nil
}

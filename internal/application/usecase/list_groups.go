package usecase

import (
	"context"
	"fmt"

	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// ListGroupsUseCase encapsula a validação para listar grupos
type ListGroupsUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewListGroupsUseCase cria uma nova instância do usecase
func NewListGroupsUseCase(cp port.ClientProvider, l port.Logger) *ListGroupsUseCase {
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
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	// Obter grupos
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		uc.logger.Error("failed to get groups", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("failed to get group list: %v", err)
	}

	result := &domain.ListGroupsResult{
		Groups: groups,
	}

	uc.logger.Info("groups listed successfully", "txtID", txtID, "count", len(groups))
	return result, nil
}

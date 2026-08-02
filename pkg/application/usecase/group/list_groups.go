package group

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ListGroupsUseCase encapsula a validação para listar grupos
type ListGroupsUseCase struct {
	groups appport.GroupDirectory
	logger appport.Logger
}

// NewListGroupsUseCase cria uma nova instância do usecase
func NewListGroupsUseCase(gd appport.GroupDirectory, l appport.Logger) *ListGroupsUseCase {
	return &ListGroupsUseCase{
		groups: gd,
		logger: l,
	}
}

// Execute valida se o cliente está disponível e obtém a lista de grupos
func (uc *ListGroupsUseCase) Execute(ctx context.Context, txtID string, req domain.ListGroupsRequest) (*domain.ListGroupsResult, error) {
	// Garantir que há sessão
	if err := uc.groups.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	// Obter grupos
	groups, count, err := uc.groups.ListJoinedGroups(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get groups", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("failed to get group list: %v", err)
	}

	result := &domain.ListGroupsResult{
		Groups: groups,
	}

	uc.logger.Info(ctx, "groups listed successfully", "txtID", txtID, "count", count)
	return result, nil
}

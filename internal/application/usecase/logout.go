package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
)

// LogoutUseCase encapsula a validação de logout.
type LogoutUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewLogoutUseCase cria uma nova instância do usecase.
func NewLogoutUseCase(cp port.ClientProvider, l port.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível e logado.
func (uc *LogoutUseCase) Execute(ctx context.Context, txtID string, req domain.LogoutRequest) (*domain.LogoutResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("logout validated", "txtID", txtID)
	return &domain.LogoutResult{}, nil
}

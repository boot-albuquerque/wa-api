package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// LogoutUseCase DESVINCULA o aparelho: a sessão some de "Aparelhos
// conectados" no celular e um novo pareamento por QR passa a ser necessário.
// É irreversível sem o telefone à mão — distinto de DisconnectUseCase, que
// só derruba o transporte.
//
// A porta é SessionController, e não SessionGuard, pelo mesmo motivo da
// F79: até então este use case só conseguia validar, e devolvia 200 sem
// desvincular coisa alguma.
type LogoutUseCase struct {
	sessions appport.SessionController
	logger   appport.Logger
}

// NewLogoutUseCase cria uma nova instância do usecase.
func NewLogoutUseCase(sc appport.SessionController, l appport.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		sessions: sc,
		logger:   l,
	}
}

// Execute encerra a autenticação da sessão de txtID no WhatsApp.
func (uc *LogoutUseCase) Execute(ctx context.Context, txtID string, req domain.LogoutRequest) (*domain.LogoutResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if err := uc.sessions.Logout(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "logout failed", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "logged out", "txtID", txtID)
	return &domain.LogoutResult{}, nil
}

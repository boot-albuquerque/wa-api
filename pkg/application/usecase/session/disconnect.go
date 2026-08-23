package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DisconnectUseCase derruba o transporte de uma sessão, mantendo o
// pareamento: a sessão reconecta depois sem exigir QR novo. É a diferença
// para LogoutUseCase, que desvincula o aparelho.
//
// A porta é SessionController, e não SessionGuard, porque este use case
// precisa AGIR sobre a sessão e não apenas verificar que ela existe. Até a
// F79 ele consumia SessionGuard e por isso só conseguia validar — devolvia
// 200 sem desconectar nada.
type DisconnectUseCase struct {
	sessions appport.SessionDisconnector
	logger   appport.Logger
}

// NewDisconnectUseCase cria uma nova instância do usecase.
func NewDisconnectUseCase(sc appport.SessionDisconnector, l appport.Logger) *DisconnectUseCase {
	return &DisconnectUseCase{
		sessions: sc,
		logger:   l,
	}
}

// Execute derruba o transporte da sessão de txtID.
func (uc *DisconnectUseCase) Execute(ctx context.Context, txtID string, req domain.DisconnectRequest) (*domain.DisconnectResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if err := uc.sessions.Disconnect(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "disconnect failed", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "disconnected", "txtID", txtID)
	return &domain.DisconnectResult{}, nil
}

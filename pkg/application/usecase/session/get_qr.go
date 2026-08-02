package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetQRUseCase encapsula a validação e leitura do QR code de pareamento.
type GetQRUseCase struct {
	sessions appport.SessionGuard
	users    appport.UserRepository
	logger   appport.Logger
}

// NewGetQRUseCase cria uma nova instância do usecase.
func NewGetQRUseCase(sg appport.SessionGuard, users appport.UserRepository, l appport.Logger) *GetQRUseCase {
	return &GetQRUseCase{
		sessions: sg,
		users:    users,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível e devolve o QR code persistido
// pelo listener de eventos do whatsmeow (bootstrap/lifecycle.go grava em
// users.qrcode a cada evento "code" do canal de QR).
//
// Antes, Execute só validava a sessão e devolvia GetQRResult{} vazio — o
// endpoint respondia 200 sem nunca ler o QR de volta, mesmo com ele já
// persistido no banco por outro caminho. ListUsers(ctx, txtID) lê o valor
// atual direto do banco (mesmo padrão de list_users.go), sem depender do
// cache de 10min da auth middleware, que ficaria obsoleto durante o
// intervalo em que o QR gira.
func (uc *GetQRUseCase) Execute(ctx context.Context, txtID string) (*domain.GetQRResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	entries, err := uc.users.ListUsers(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to read QR code", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}
	if len(entries) == 0 {
		uc.logger.Error(ctx, "no user record for session", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "get QR validated", "txtID", txtID, "hasQR", entries[0].QRCode != "")
	return &domain.GetQRResult{QRCode: entries[0].QRCode}, nil
}

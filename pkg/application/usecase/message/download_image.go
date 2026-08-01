package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadImageUseCase encapsula a validação de download de imagem.
type DownloadImageUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDownloadImageUseCase cria uma nova instância do usecase.
func NewDownloadImageUseCase(sg appport.SessionGuard, l appport.Logger) *DownloadImageUseCase {
	return &DownloadImageUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadImageUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("missing Url in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "download image validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

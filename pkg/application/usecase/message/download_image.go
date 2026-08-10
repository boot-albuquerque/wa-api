package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

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
		return nil, apperr.New("missing_url", apperr.CategoryValidation, "missing Url in payload", false, nil)
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "download image validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

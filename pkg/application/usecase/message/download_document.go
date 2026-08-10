package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadDocumentUseCase encapsula a validação de download de documento.
type DownloadDocumentUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDownloadDocumentUseCase cria uma nova instância do usecase.
func NewDownloadDocumentUseCase(sg appport.SessionGuard, l appport.Logger) *DownloadDocumentUseCase {
	return &DownloadDocumentUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadDocumentUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" {
		return nil, apperr.New("missing_url", apperr.CategoryValidation, "missing Url in payload", false, nil)
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "download document validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

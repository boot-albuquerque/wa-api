package message

import (
	"context"
	"fmt"

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
		return nil, fmt.Errorf("missing Url in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "download document validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

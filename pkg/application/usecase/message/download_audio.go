package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadAudioUseCase encapsula a validação de download de áudio.
type DownloadAudioUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDownloadAudioUseCase cria uma nova instância do usecase.
func NewDownloadAudioUseCase(sg appport.SessionGuard, l appport.Logger) *DownloadAudioUseCase {
	return &DownloadAudioUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadAudioUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("missing Url in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "download audio validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

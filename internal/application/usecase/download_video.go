package usecase

import (
	"context"
	"fmt"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// DownloadVideoUseCase encapsula a validação de download de vídeo.
type DownloadVideoUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDownloadVideoUseCase cria uma nova instância do usecase.
func NewDownloadVideoUseCase(cp port.ClientProvider, l port.Logger) *DownloadVideoUseCase {
	return &DownloadVideoUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadVideoUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("missing Url in payload")
	}

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("download video validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

// GetVideoMessage builds the protobuf VideoMessage from DownloadRequest.
func (uc *DownloadVideoUseCase) GetVideoMessage(req domain.DownloadRequest) *waE2E.VideoMessage {
	return &waE2E.VideoMessage{
		URL:           proto.String(req.URL),
		DirectPath:    proto.String(req.DirectPath),
		MediaKey:      req.MediaKey,
		Mimetype:      proto.String(req.Mimetype),
		FileEncSHA256: req.FileEncSHA256,
		FileSHA256:    req.FileSHA256,
		FileLength:    &req.FileLength,
	}
}

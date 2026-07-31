package usecase

import (
	"context"
	"fmt"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
)

// DownloadAudioUseCase encapsula a validação de download de áudio.
type DownloadAudioUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewDownloadAudioUseCase cria uma nova instância do usecase.
func NewDownloadAudioUseCase(cp appport.ClientProvider, l appport.Logger) *DownloadAudioUseCase {
	return &DownloadAudioUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadAudioUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
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

	uc.logger.Info("download audio validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

// GetAudioMessage builds the protobuf AudioMessage from DownloadRequest.
func (uc *DownloadAudioUseCase) GetAudioMessage(req domain.DownloadRequest) *waE2E.AudioMessage {
	return &waE2E.AudioMessage{
		URL:           proto.String(req.URL),
		DirectPath:    proto.String(req.DirectPath),
		MediaKey:      req.MediaKey,
		Mimetype:      proto.String(req.Mimetype),
		FileEncSHA256: req.FileEncSHA256,
		FileSHA256:    req.FileSHA256,
		FileLength:    &req.FileLength,
	}
}

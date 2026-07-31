package usecase

import (
	"context"
	"fmt"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// DownloadStickerUseCase encapsula a validação de download de sticker.
type DownloadStickerUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewDownloadStickerUseCase cria uma nova instância do usecase.
func NewDownloadStickerUseCase(cp appport.ClientProvider, l appport.Logger) *DownloadStickerUseCase {
	return &DownloadStickerUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadStickerUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
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

	uc.logger.Info("download sticker validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

// GetStickerMessage builds the protobuf StickerMessage from DownloadRequest.
func (uc *DownloadStickerUseCase) GetStickerMessage(req domain.DownloadRequest) *waE2E.StickerMessage {
	return &waE2E.StickerMessage{
		URL:           proto.String(req.URL),
		DirectPath:    proto.String(req.DirectPath),
		MediaKey:      req.MediaKey,
		Mimetype:      proto.String(req.Mimetype),
		FileEncSHA256: req.FileEncSHA256,
		FileSHA256:    req.FileSHA256,
		FileLength:    &req.FileLength,
	}
}

package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// DownloadImageUseCase encapsula a validação de download de imagem.
type DownloadImageUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewDownloadImageUseCase cria uma nova instância do usecase.
func NewDownloadImageUseCase(cp appport.ClientProvider, l appport.Logger) *DownloadImageUseCase {
	return &DownloadImageUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadImageUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("missing Url in payload")
	}

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "download image validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

// GetImageMessage builds the protobuf ImageMessage from DownloadRequest.
func (uc *DownloadImageUseCase) GetImageMessage(req domain.DownloadRequest) *waE2E.ImageMessage {
	return &waE2E.ImageMessage{
		URL:           proto.String(req.URL),
		DirectPath:    proto.String(req.DirectPath),
		MediaKey:      req.MediaKey,
		Mimetype:      proto.String(req.Mimetype),
		FileEncSHA256: req.FileEncSHA256,
		FileSHA256:    req.FileSHA256,
		FileLength:    &req.FileLength,
	}
}

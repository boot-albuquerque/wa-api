package usecase

import (
	"context"
	"fmt"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// DownloadDocumentUseCase encapsula a validação de download de documento.
type DownloadDocumentUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDownloadDocumentUseCase cria uma nova instância do usecase.
func NewDownloadDocumentUseCase(cp port.ClientProvider, l port.Logger) *DownloadDocumentUseCase {
	return &DownloadDocumentUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DownloadDocumentUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
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

	uc.logger.Info("download document validated", "txtID", txtID)
	return &domain.DownloadResult{}, nil
}

// GetDocumentMessage builds the protobuf DocumentMessage from DownloadRequest.
func (uc *DownloadDocumentUseCase) GetDocumentMessage(req domain.DownloadRequest) *waE2E.DocumentMessage {
	return &waE2E.DocumentMessage{
		URL:           proto.String(req.URL),
		DirectPath:    proto.String(req.DirectPath),
		MediaKey:      req.MediaKey,
		Mimetype:      proto.String(req.Mimetype),
		FileEncSHA256: req.FileEncSHA256,
		FileSHA256:    req.FileSHA256,
		FileLength:    &req.FileLength,
	}
}

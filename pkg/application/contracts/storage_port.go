// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em internal/infrastructure/.
package port

import (
	"context"

	"wa-api/pkg/domain"
)

// StoragePort abstrai operações de armazenamento (S3, mídia).
type StoragePort interface {
	// UploadFile faz upload de um arquivo para storage e retorna a URL.
	UploadFile(ctx context.Context, jid domain.JID, fileName string, data []byte, mimeType string) (string, error)

	// ProcessMedia processa mídia recebida (download, thumbnail, upload).
	ProcessMedia(ctx context.Context, msg *domain.Message) error

	// GetS3Config retorna a configuração S3 para um usuário.
	GetS3Config(userID string) *S3Config
}

// S3Config representa a configuração de conexão S3.
type S3Config struct {
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
}

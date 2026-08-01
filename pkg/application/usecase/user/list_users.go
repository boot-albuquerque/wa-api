package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ListUsersUseCase lista usuários do banco de dados
type ListUsersUseCase struct {
	users    appport.UserRepository
	sessions appport.SessionStatusReader
	logger   appport.Logger
}

// NewListUsersUseCase cria uma nova instância
func NewListUsersUseCase(users appport.UserRepository, logger appport.Logger, sessions appport.SessionStatusReader) *ListUsersUseCase {
	return &ListUsersUseCase{users: users, sessions: sessions, logger: logger}
}

// Execute lista um ou todos os usuários
func (uc *ListUsersUseCase) Execute(ctx context.Context, req domain.ListUsersRequest) ([]domain.UserResponse, error) {
	entries, err := uc.users.ListUsers(ctx, req.UserID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to list users", "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}

	var users []domain.UserResponse
	for _, entry := range entries {
		isConnected, isLoggedIn := uc.sessions.SessionStatus(ctx, entry.ID)

		s3Config := map[string]interface{}{
			"enabled":        entry.S3.Enabled,
			"endpoint":       entry.S3.Endpoint,
			"region":         entry.S3.Region,
			"bucket":         entry.S3.Bucket,
			"access_key":     "***",
			"path_style":     entry.S3.PathStyle,
			"public_url":     entry.S3.PublicURL,
			"media_delivery": entry.S3.MediaDelivery,
			"retention_days": entry.S3.RetentionDays,
		}

		proxyConfig := map[string]interface{}{
			"enabled":         entry.HasProxyURL,
			"proxyUrl":        entry.ProxyURL,
			"webhookUseProxy": entry.WebhookUseProxy,
		}

		// Token fica deliberadamente vazio: GET /admin/users devolvia o token em
		// texto claro de todos os usuários (sec/F20), o que transformava uma
		// leitura de listagem no vazamento de todas as credenciais da
		// instalação. A listagem não é o lugar de recuperar credencial.
		users = append(users, domain.UserResponse{
			ID:          entry.ID,
			Name:        entry.Name,
			Webhook:     entry.Webhook,
			JID:         entry.JID,
			QRCode:      entry.QRCode,
			Connected:   isConnected,
			LoggedIn:    isLoggedIn,
			Expiration:  entry.Expiration,
			ProxyConfig: proxyConfig,
			S3Config:    s3Config,
			Events:      entry.Events,
		})
	}

	return users, nil
}

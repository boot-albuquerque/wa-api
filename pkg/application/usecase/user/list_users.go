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

// Execute lista um ou todos os usuários.
//
// The empty listing is an empty slice and never nil: the presenter turns it
// into `[]` on the wire, and `null` there is a different value to every
// client — only one of the two can be iterated without a check.
func (uc *ListUsersUseCase) Execute(ctx context.Context, req domain.ListUsersInput) ([]domain.UserAccount, error) {
	entries, err := uc.users.ListUsers(ctx, req.UserID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to list users", "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}

	users := make([]domain.UserAccount, 0, len(entries))
	for _, entry := range entries {
		isConnected, isLoggedIn := uc.sessions.SessionStatus(ctx, entry.ID)

		// Token fica deliberadamente vazio: GET /admin/users devolvia o token em
		// texto claro de todos os usuários (sec/F20), o que transformava uma
		// leitura de listagem no vazamento de todas as credenciais da
		// instalação. A listagem não é o lugar de recuperar credencial.
		users = append(users, domain.UserAccount{
			ID:         entry.ID,
			Name:       entry.Name,
			Webhook:    entry.Webhook,
			JID:        entry.JID,
			QRCode:     entry.QRCode,
			Connected:  isConnected,
			LoggedIn:   isLoggedIn,
			Expiration: entry.Expiration,
			Events:     entry.Events,
			Engine:     entry.Engine,
			Proxy: domain.UserProxySettings{
				Enabled:         entry.HasProxyURL,
				URL:             entry.ProxyURL,
				WebhookUseProxy: entry.WebhookUseProxy,
			},
			S3: domain.UserS3Settings{
				Enabled:             entry.S3.Enabled,
				Endpoint:            entry.S3.Endpoint,
				Region:              entry.S3.Region,
				Bucket:              entry.S3.Bucket,
				PathStyle:           entry.S3.PathStyle,
				PublicURL:           entry.S3.PublicURL,
				MediaDelivery:       entry.S3.MediaDelivery,
				RetentionDays:       entry.S3.RetentionDays,
				AccessKeyConfigured: entry.S3.AccessKeyConfigured,
			},
		})
	}

	return users, nil
}

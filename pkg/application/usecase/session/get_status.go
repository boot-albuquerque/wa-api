package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// GetStatusUseCase encapsula a validação e leitura do status de sessão.
type GetStatusUseCase struct {
	sessions appport.SessionGuard
	status   appport.SessionStatusReader
	users    appport.UserRepository
	logger   appport.Logger
}

// NewGetStatusUseCase cria uma nova instância do usecase.
func NewGetStatusUseCase(sg appport.SessionGuard, status appport.SessionStatusReader, users appport.UserRepository, l appport.Logger) *GetStatusUseCase {
	return &GetStatusUseCase{
		sessions: sg,
		status:   status,
		users:    users,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível e devolve o status ao vivo da
// sessão (connected/loggedIn, via whatsmeow) somado ao registro persistido
// (jid, webhook, qrcode, ...). Antes, Execute só validava a sessão e devolvia
// GetStatusResult{} vazio — todo caller via connected=false/loggedIn=false
// sempre, mesmo com a sessão pareada; um adapter cliente que dependa deste
// endpoint para detectar a transição QR→autenticado nunca via a mudança.
func (uc *GetStatusUseCase) Execute(ctx context.Context, txtID string) (*domain.GetStatusResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	connected, loggedIn := uc.status.SessionStatus(ctx, txtID)

	entries, err := uc.users.ListUsers(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to read session record", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}
	if len(entries) == 0 {
		uc.logger.Error(ctx, "no user record for session", "txtID", txtID)
		return nil, apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
	}
	entry := entries[0]

	uc.logger.Info(ctx, "get status validated", "txtID", txtID, "connected", connected, "loggedIn", loggedIn)
	return &domain.GetStatusResult{
		ID:        entry.ID,
		Name:      entry.Name,
		Connected: connected,
		LoggedIn:  loggedIn,
		Jid:       entry.JID,
		Webhook:   entry.Webhook,
		Events:    entry.Events,
		ProxyURL:  entry.ProxyURL,
		Qrcode:    entry.QRCode,
		History:   "0",
		ProxyConfig: map[string]interface{}{
			"enabled":  entry.HasProxyURL,
			"proxyUrl": entry.ProxyURL,
		},
		S3Config: map[string]interface{}{
			"enabled":        entry.S3.Enabled,
			"endpoint":       entry.S3.Endpoint,
			"region":         entry.S3.Region,
			"bucket":         entry.S3.Bucket,
			"path_style":     entry.S3.PathStyle,
			"public_url":     entry.S3.PublicURL,
			"media_delivery": entry.S3.MediaDelivery,
			"retention_days": entry.S3.RetentionDays,
		},
	}, nil
}

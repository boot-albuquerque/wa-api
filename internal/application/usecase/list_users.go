package usecase

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"wuzapi/internal/domain"
)

// ClientManagerAdapter interface para acessar whatsmeow clients
// Note: GetWhatsmeowClient returns *whatsmeow.Client (imported as interface{} to avoid circular deps)
type ClientManagerAdapter interface {
	GetWhatsmeowClient(id string) interface{}
	IsConnected(id string) bool
	IsLoggedIn(id string) bool
}

// ListUsersUseCase lista usuários do banco de dados
type ListUsersUseCase struct {
	db     *sqlx.DB
	logger zerolog.Logger
	cm     ClientManagerAdapter
}

// NewListUsersUseCase cria uma nova instância
func NewListUsersUseCase(db *sqlx.DB, logger zerolog.Logger, cm ClientManagerAdapter) *ListUsersUseCase {
	return &ListUsersUseCase{db: db, logger: logger, cm: cm}
}

// Execute lista um ou todos os usuários
func (uc *ListUsersUseCase) Execute(ctx context.Context, req domain.ListUsersRequest) ([]domain.UserResponse, error) {
	var query string
	var args []interface{}

	if req.UserID != "" {
		query = `SELECT id, name, token, webhook, jid, qrcode, connected, expiration, proxy_url,
				 COALESCE(webhook_use_proxy, true) AS webhook_use_proxy, events, history
				 FROM users WHERE id = $1`
		args = append(args, req.UserID)
	} else {
		query = `SELECT id, name, token, webhook, jid, qrcode, connected, expiration, proxy_url,
				 COALESCE(webhook_use_proxy, true) AS webhook_use_proxy, events, history
				 FROM users`
	}

	rows, err := uc.db.QueryxContext(ctx, query, args...)
	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to list users")
		return nil, fmt.Errorf("database error: %w", err)
	}
	defer rows.Close()

	var users []domain.UserResponse
	for rows.Next() {
		var user struct {
			ID              string         `db:"id"`
			Name            string         `db:"name"`
			Token           string         `db:"token"`
			Webhook         string         `db:"webhook"`
			JID             string         `db:"jid"`
			QRCode          string         `db:"qrcode"`
			Connected       sql.NullBool   `db:"connected"`
			Expiration      sql.NullInt64  `db:"expiration"`
			ProxyURL        sql.NullString `db:"proxy_url"`
			WebhookUseProxy bool           `db:"webhook_use_proxy"`
			Events          string         `db:"events"`
			History         sql.NullInt64  `db:"history"`
		}

		if err := rows.StructScan(&user); err != nil {
			uc.logger.Error().Err(err).Msg("Failed to scan user")
			return nil, fmt.Errorf("scan error: %w", err)
		}

		// Check live WhatsApp client status
		isConnected := false
		isLoggedIn := false
		if client := uc.cm.GetWhatsmeowClient(user.ID); client != nil {
			isConnected = uc.cm.IsConnected(user.ID)
			isLoggedIn = uc.cm.IsLoggedIn(user.ID)
		}

		// Query S3 config
		var s3Enabled bool
		var s3Endpoint, s3Region, s3Bucket, s3PublicURL, s3MediaDelivery string
		var s3PathStyle bool
		var s3RetentionDays int

		s3Config := map[string]interface{}{
			"enabled":        false,
			"endpoint":       "",
			"region":         "",
			"bucket":         "",
			"access_key":     "***",
			"path_style":     false,
			"public_url":     "",
			"media_delivery": "",
			"retention_days": 0,
		}

		err = uc.db.QueryRowContext(ctx,
			`SELECT COALESCE(s3_enabled, false), COALESCE(s3_endpoint, ''), COALESCE(s3_region, ''),
			 COALESCE(s3_bucket, ''), COALESCE(s3_path_style, false), COALESCE(s3_public_url, ''),
			 COALESCE(media_delivery, ''), COALESCE(s3_retention_days, 0) FROM users WHERE id = $1`,
			user.ID).Scan(&s3Enabled, &s3Endpoint, &s3Region, &s3Bucket, &s3PathStyle, &s3PublicURL, &s3MediaDelivery, &s3RetentionDays)

		if err == nil {
			s3Config["enabled"] = s3Enabled
			s3Config["endpoint"] = s3Endpoint
			s3Config["region"] = s3Region
			s3Config["bucket"] = s3Bucket
			s3Config["path_style"] = s3PathStyle
			s3Config["public_url"] = s3PublicURL
			s3Config["media_delivery"] = s3MediaDelivery
			s3Config["retention_days"] = s3RetentionDays
		} else if err != sql.ErrNoRows {
			uc.logger.Warn().Err(err).Str("user_id", user.ID).Msg("Failed to query S3 config")
		}

		// Build proxy config response
		proxyConfig := map[string]interface{}{
			"enabled":         user.ProxyURL.Valid && user.ProxyURL.String != "",
			"proxyUrl":        user.ProxyURL.String,
			"webhookUseProxy": user.WebhookUseProxy,
		}

		userResp := domain.UserResponse{
			ID:          user.ID,
			Name:        user.Name,
			Token:       user.Token,
			Webhook:     user.Webhook,
			JID:         user.JID,
			QRCode:      user.QRCode,
			Connected:   isConnected,
			LoggedIn:    isLoggedIn,
			Expiration:  user.Expiration.Int64,
			ProxyConfig: proxyConfig,
			S3Config:    s3Config,
			Events:      user.Events,
		}

		users = append(users, userResp)
	}

	if err := rows.Err(); err != nil {
		uc.logger.Error().Err(err).Msg("Error iterating users")
		return nil, fmt.Errorf("iteration error: %w", err)
	}

	return users, nil
}
